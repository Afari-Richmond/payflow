// Package application holds payment-service's use-case orchestration —
// the business rules for what makes a valid payment, independent of
// whether the caller is gRPC, a test, or (later) something else.
package application

import (
	"context"
	"errors"
	"net/mail"
	"time"

	"github.com/google/uuid"

	"github.com/Afari-Richmond/payflow/services/payment-service/internal/domain"
	"github.com/Afari-Richmond/payflow/services/payment-service/internal/provider"
	"github.com/Afari-Richmond/payflow/services/payment-service/internal/repository"
)

var (
	// ErrInvalidOrderID means the provided order ID was not a valid UUID.
	ErrInvalidOrderID = errors.New("invalid order id")
	// ErrInvalidEmail means the provided email address failed
	// validation.
	ErrInvalidEmail = errors.New("invalid email address")
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
// rules, generates the server-side ID, persists the initial attempt,
// initializes the transaction with the payment provider, and persists
// the resulting provider reference.
type PaymentService struct {
	repo     repository.PaymentRepository
	provider provider.PaymentProvider
}

// NewPaymentService builds a PaymentService.
func NewPaymentService(repo repository.PaymentRepository, paymentProvider provider.PaymentProvider) *PaymentService {
	return &PaymentService{repo: repo, provider: paymentProvider}
}

// CreatePayment validates the request, persists a PENDING payment
// attempt, then initializes the transaction with the payment provider.
// On success, the payment is updated to INITIALIZED with the
// provider's reference persisted; the authorization URL is returned to
// the caller but never persisted — it's a one-time client action URL,
// not a fact about the payment. On provider failure, the payment is
// marked FAILED and the attempt still exists for audit purposes.
//
// IDs and timestamps are always generated server-side — never trusted
// from the caller.
func (s *PaymentService) CreatePayment(ctx context.Context, orderID, email string, amountMinor int64, currency string) (*domain.Payment, string, error) {
	parsedOrderID, err := uuid.Parse(orderID)
	if err != nil {
		return nil, "", ErrInvalidOrderID
	}
	if _, err := mail.ParseAddress(email); err != nil {
		return nil, "", ErrInvalidEmail
	}
	if amountMinor <= 0 {
		return nil, "", ErrInvalidAmount
	}
	if !supportedCurrencies[currency] {
		return nil, "", ErrUnsupportedCurrency
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
		return nil, "", err
	}

	result, err := s.provider.InitializeTransaction(ctx, provider.InitializeTransactionInput{
		Email:       email,
		AmountMinor: amountMinor,
		Currency:    currency,
		Reference:   payment.ID.String(),
	})
	if err != nil {
		payment.Status = domain.StatusFailed
		payment.UpdatedAt = time.Now().UTC()
		_ = s.repo.Update(ctx, payment) // best-effort: the attempt is already recorded as PENDING even if this fails
		return nil, "", err
	}

	payment.Status = domain.StatusInitialized
	payment.ProviderReference = result.Reference
	payment.UpdatedAt = time.Now().UTC()
	if err := s.repo.Update(ctx, payment); err != nil {
		return nil, "", err
	}

	return payment, result.AuthorizationURL, nil
}
