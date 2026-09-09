// Package application holds order-service's use-case orchestration —
// the business rules for what makes a valid order, independent of
// whether the caller is gRPC, a test, or (later) something else.
package application

import (
	"context"
	"errors"
	"net/mail"
	"time"

	"github.com/google/uuid"

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
