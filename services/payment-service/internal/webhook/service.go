// Package webhook processes incoming Paystack webhook deliveries:
// signature verification, event parsing, provider-side reconciliation,
// and the resulting payment state transition.
package webhook

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

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

// Service verifies and processes Paystack webhook deliveries.
type Service struct {
	repo      repository.PaymentRepository
	provider  provider.PaymentProvider
	secretKey string
}

// NewService builds a Service.
func NewService(repo repository.PaymentRepository, paymentProvider provider.PaymentProvider, secretKey string) *Service {
	return &Service{repo: repo, provider: paymentProvider, secretKey: secretKey}
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
		// Already processed — this is a duplicate delivery. Basic
		// idempotency guard via current state; full processed-event
		// tracking (protecting against a race between two concurrent
		// deliveries) is Milestone 10.
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
		return s.repo.Update(ctx, payment)
	}

	if result.AmountMinor != payment.AmountMinor || result.Currency != payment.Currency {
		return fmt.Errorf("webhook: verified amount/currency mismatch for payment %s", payment.ID)
	}

	payment.Status = domain.StatusSuccess
	payment.UpdatedAt = time.Now().UTC()
	return s.repo.Update(ctx, payment)
}
