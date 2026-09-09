// Package application holds payment-service's use-case orchestration —
// the business rules for what makes a valid payment, independent of
// whether the caller is gRPC, a test, or (later) something else.
package application

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"

	"github.com/Afari-Richmond/payflow/services/payment-service/internal/domain"
	"github.com/Afari-Richmond/payflow/services/payment-service/internal/repository"
)

var (
	// ErrInvalidOrderID means the provided order ID was not a valid UUID.
	ErrInvalidOrderID = errors.New("invalid order id")
	// ErrInvalidAmount means the amount was not a positive integer.
	ErrInvalidAmount = errors.New("amount must be a positive integer")
	// ErrUnsupportedCurrency means the currency isn't one PayFlow
	// accepts.
	ErrUnsupportedCurrency = errors.New("unsupported currency")
)

// supportedCurrencies mirrors order-service's allowlist — kept in sync
// deliberately, not shared as a package, since the two services must
// never import each other's internals.
var supportedCurrencies = map[string]bool{
	"GHS": true,
	"NGN": true,
	"USD": true,
}

// PaymentService orchestrates payment creation: validates business
// rules, generates the server-side ID, and persists via the
// repository. Does not talk to Paystack — see PaymentProvider,
// introduced in Milestone 7.
type PaymentService struct {
	repo repository.PaymentRepository
}

// NewPaymentService builds a PaymentService.
func NewPaymentService(repo repository.PaymentRepository) *PaymentService {
	return &PaymentService{repo: repo}
}

// CreatePayment validates the request and creates a new payment in
// StatusPending. IDs and timestamps are always generated server-side —
// never trusted from the caller.
func (s *PaymentService) CreatePayment(ctx context.Context, orderID string, amountMinor int64, currency string) (*domain.Payment, error) {
	parsedOrderID, err := uuid.Parse(orderID)
	if err != nil {
		return nil, ErrInvalidOrderID
	}
	if amountMinor <= 0 {
		return nil, ErrInvalidAmount
	}
	if !supportedCurrencies[currency] {
		return nil, ErrUnsupportedCurrency
	}

	now := time.Now().UTC()
	payment := &domain.Payment{
		ID:          uuid.New(),
		OrderID:     parsedOrderID,
		AmountMinor: amountMinor,
		Currency:    currency,
		Status:      domain.StatusPending,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	if err := s.repo.Create(ctx, payment); err != nil {
		return nil, err
	}

	return payment, nil
}
