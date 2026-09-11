// Package application holds order-service's use-case orchestration —
// the business rules for what makes a valid order, independent of
// whether the caller is gRPC, a test, or (later) something else.
package application

import (
	"context"
	"encoding/json"
	"errors"
	"net/mail"
	"time"

	"github.com/google/uuid"

	"github.com/Afari-Richmond/payflow/pkg/events"
	"github.com/Afari-Richmond/payflow/services/order-service/internal/domain"
	"github.com/Afari-Richmond/payflow/services/order-service/internal/repository"
)

var (
	// ErrInvalidEmail means the provided email address failed
	// validation.
	ErrInvalidEmail = errors.New("invalid email address")
	// ErrInvalidAmount means the amount was not a positive integer.
	ErrInvalidAmount = errors.New("amount must be a positive integer")
	// ErrUnsupportedCurrency means the currency isn't one PayFlow
	// accepts.
	ErrUnsupportedCurrency = errors.New("unsupported currency")
)

// supportedCurrencies are the currencies PayFlow accepts, matching
// what Paystack supports for the markets this project targets.
// Extended deliberately, not defensively — no currency is added here
// until there's a real reason to accept it.
var supportedCurrencies = map[string]bool{
	"GHS": true,
	"NGN": true,
	"USD": true,
}

// OrderService orchestrates order creation: validates business rules,
// generates the server-side ID, and persists via the repository.
type OrderService struct {
	repo repository.OrderRepository
}

// NewOrderService builds an OrderService.
func NewOrderService(repo repository.OrderRepository) *OrderService {
	return &OrderService{repo: repo}
}

// CreateOrder validates the request and creates a new order in
// StatusPendingPayment. IDs and timestamps are always generated
// server-side — never trusted from the caller.
func (s *OrderService) CreateOrder(ctx context.Context, email string, amountMinor int64, currency string) (*domain.Order, error) {
	if _, err := mail.ParseAddress(email); err != nil {
		return nil, ErrInvalidEmail
	}
	if amountMinor <= 0 {
		return nil, ErrInvalidAmount
	}
	if !supportedCurrencies[currency] {
		return nil, ErrUnsupportedCurrency
	}

	now := time.Now().UTC()
	order := &domain.Order{
		ID:          uuid.New(),
		Email:       email,
		AmountMinor: amountMinor,
		Currency:    currency,
		Status:      domain.StatusPendingPayment,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	if err := s.repo.Create(ctx, order); err != nil {
		return nil, err
	}

	return order, nil
}

// HandlePaymentEvent processes a payment domain event consumed from
// RabbitMQ. The returned error's type tells the consumer how to
// acknowledge: an *events.PermanentError means dead-letter (retrying
// will never help — malformed data, unknown order); any other error
// means requeue (transient — e.g. a DB blip); nil means ack.
func (s *OrderService) HandlePaymentEvent(ctx context.Context, envelope events.Envelope) error {
	switch envelope.EventType {
	case events.PaymentSucceeded:
		return s.handlePaymentSucceeded(ctx, envelope)
	case events.PaymentFailed:
		// No order state change currently modeled for a failed payment
		// attempt — the order stays PENDING_PAYMENT so the customer can
		// retry. Still explicitly handled (not falling into default) so
		// it's clear this event type is recognized, not merely ignored
		// by omission.
		return nil
	default:
		// Unsupported event type — safely ignored, not an error.
		return nil
	}
}

func (s *OrderService) handlePaymentSucceeded(ctx context.Context, envelope events.Envelope) error {
	var payload events.PaymentSucceededPayload
	if err := json.Unmarshal(envelope.Payload, &payload); err != nil {
		return events.NewPermanentError(err)
	}

	orderID, err := uuid.Parse(payload.OrderID)
	if err != nil {
		return events.NewPermanentError(err)
	}

	eventID, err := uuid.Parse(envelope.EventID)
	if err != nil {
		return events.NewPermanentError(err)
	}

	order, err := s.repo.GetByID(ctx, orderID)
	if errors.Is(err, repository.ErrNotFound) {
		return events.NewPermanentError(err)
	}
	if err != nil {
		return err // transient — DB issue, worth retrying
	}

	if order.Status == domain.StatusPaid {
		// Fast path only: cheap enough to skip a DB transaction for the
		// common case (redelivered after processing is fully done and
		// visible). The actual correctness guarantee against a
		// redelivered/duplicate message is the processed_events unique
		// constraint below, via MarkProcessedAndUpdate — this check
		// alone has a real race (two near-simultaneous deliveries could
		// both pass it before either writes), which is exactly why that
		// constraint exists.
		return nil
	}

	order.Status = domain.StatusPaid
	order.UpdatedAt = time.Now().UTC()
	_, err = s.repo.MarkProcessedAndUpdate(ctx, eventID, envelope.EventType, order)
	return err
}
