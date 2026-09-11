// Package webhook processes incoming Paystack webhook deliveries:
// signature verification, event parsing, provider-side reconciliation,
// and the resulting payment state transition.
package webhook

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"github.com/Afari-Richmond/payflow/pkg/events"
	"github.com/Afari-Richmond/payflow/services/payment-service/internal/domain"
	"github.com/Afari-Richmond/payflow/services/payment-service/internal/provider"
	"github.com/Afari-Richmond/payflow/services/payment-service/internal/repository"
)

// ErrInvalidSignature means the request's signature did not match —
// the request is rejected before its body is even parsed.
var ErrInvalidSignature = errors.New("invalid webhook signature")

// ErrMalformedPayload means the (signature-verified) body wasn't
// valid JSON or was missing required fields.
var ErrMalformedPayload = errors.New("malformed webhook payload")

// EventPublisher is the messaging dependency this service needs. A
// narrow interface (not the concrete *messaging.Publisher) so this
// package can be tested without a real broker.
type EventPublisher interface {
	Publish(ctx context.Context, routingKey string, envelope events.Envelope) error
}

// Service verifies and processes Paystack webhook deliveries.
type Service struct {
	repo      repository.PaymentRepository
	provider  provider.PaymentProvider
	publisher EventPublisher
	secretKey string
	logger    *slog.Logger
}

// NewService builds a Service.
func NewService(repo repository.PaymentRepository, paymentProvider provider.PaymentProvider, publisher EventPublisher, secretKey string, logger *slog.Logger) *Service {
	return &Service{repo: repo, provider: paymentProvider, publisher: publisher, secretKey: secretKey, logger: logger}
}

// HandleWebhook verifies rawBody against signature, then processes the
// event. Returns nil for both "successfully processed" and "safely
// ignored" (unsupported event type, already-processed duplicate) —
// both cases should be acknowledged (HTTP 200) to Paystack. Returns
// ErrInvalidSignature or ErrMalformedPayload for genuine rejections.
func (s *Service) HandleWebhook(ctx context.Context, rawBody []byte, signature string) error {
	if !verifySignature(s.secretKey, rawBody, signature) {
		return ErrInvalidSignature
	}

	var evt event
	if err := json.Unmarshal(rawBody, &evt); err != nil {
		return fmt.Errorf("%w: %v", ErrMalformedPayload, err)
	}
	if evt.Event == "" {
		return fmt.Errorf("%w: missing event field", ErrMalformedPayload)
	}

	if evt.Event != chargeSuccessEvent {
		// Unsupported event type — safely ignored, not an error.
		return nil
	}
	if evt.Data.Reference == "" {
		return fmt.Errorf("%w: missing data.reference", ErrMalformedPayload)
	}

	return s.handleChargeSuccess(ctx, evt.Data.Reference)
}

func (s *Service) handleChargeSuccess(ctx context.Context, reference string) error {
	paymentID, err := uuid.Parse(reference)
	if err != nil {
		// Reference isn't one of our payment IDs — nothing we can
		// safely act on. Not our event; ignore rather than error, so
		// an unrelated/malformed reference can't be used to make this
		// endpoint return errors that trigger Paystack retries.
		return nil
	}

	payment, err := s.repo.GetByID(ctx, paymentID)
	if errors.Is(err, repository.ErrNotFound) {
		return nil // unknown payment — safely ignored, same reasoning as above.
	}
	if err != nil {
		return err
	}

	if payment.Status == domain.StatusSuccess {
		// Fast path only: cheap enough to skip a VerifyTransaction call
		// and a DB transaction for the common case (redelivered after
		// processing is fully done and visible). The actual correctness
		// guarantee against concurrent duplicate deliveries is the
		// processed_webhook_events unique constraint below, via
		// MarkProcessedAndUpdate — this check alone has a real race
		// (two concurrent calls could both pass it before either
		// writes), which is exactly why that constraint exists.
		return nil
	}

	// Never trust the webhook body's own status/amount claims alone —
	// independently reconcile with Paystack before changing state
	// (existing invariant: confirmation is always server-side).
	result, err := s.provider.VerifyTransaction(ctx, reference)
	if err != nil {
		return err
	}

	if result.Status != provider.TransactionStatusSuccess {
		payment.Status = domain.StatusFailed
		payment.UpdatedAt = time.Now().UTC()
		alreadyProcessed, err := s.repo.MarkProcessedAndUpdate(ctx, payment, chargeSuccessEvent)
		if err != nil {
			return err
		}
		if !alreadyProcessed {
			s.publishPaymentFailed(ctx, payment)
		}
		return nil
	}

	if result.AmountMinor != payment.AmountMinor || result.Currency != payment.Currency {
		return fmt.Errorf("webhook: verified amount/currency mismatch for payment %s", payment.ID)
	}

	payment.Status = domain.StatusSuccess
	payment.UpdatedAt = time.Now().UTC()
	alreadyProcessed, err := s.repo.MarkProcessedAndUpdate(ctx, payment, chargeSuccessEvent)
	if err != nil {
		return err
	}
	if !alreadyProcessed {
		s.publishPaymentSucceeded(ctx, payment)
	}
	return nil
}

// publishPaymentSucceeded and publishPaymentFailed publish best-effort:
// the payment's own state is already durably persisted by the time
// these are called, so a publish failure is logged, not propagated.
// No Outbox yet (Milestone 11) — this is exactly the dual-write gap
// that milestone exists to close. See ADR 003.
func (s *Service) publishPaymentSucceeded(ctx context.Context, payment *domain.Payment) {
	payload := events.PaymentSucceededPayload{
		PaymentID:   payment.ID.String(),
		OrderID:     payment.OrderID.String(),
		AmountMinor: payment.AmountMinor,
		Currency:    payment.Currency,
	}
	envelope, err := events.NewEnvelope(events.PaymentSucceeded, payment.ID.String(), payload)
	if err != nil {
		s.logger.Error("failed to build payment.succeeded envelope", "payment_id", payment.ID, "error", err)
		return
	}
	if err := s.publisher.Publish(ctx, events.PaymentSucceeded, envelope); err != nil {
		s.logger.Error("failed to publish payment.succeeded", "payment_id", payment.ID, "error", err)
	}
}

func (s *Service) publishPaymentFailed(ctx context.Context, payment *domain.Payment) {
	payload := events.PaymentFailedPayload{
		PaymentID: payment.ID.String(),
		OrderID:   payment.OrderID.String(),
	}
	envelope, err := events.NewEnvelope(events.PaymentFailed, payment.ID.String(), payload)
	if err != nil {
		s.logger.Error("failed to build payment.failed envelope", "payment_id", payment.ID, "error", err)
		return
	}
	if err := s.publisher.Publish(ctx, events.PaymentFailed, envelope); err != nil {
		s.logger.Error("failed to publish payment.failed", "payment_id", payment.ID, "error", err)
	}
}
