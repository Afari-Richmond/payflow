package application_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/Afari-Richmond/payflow/services/payment-service/internal/application"
	"github.com/Afari-Richmond/payflow/services/payment-service/internal/domain"
	"github.com/Afari-Richmond/payflow/services/payment-service/internal/provider"
)

type fakePaymentRepository struct {
	created   *domain.Payment
	updated   *domain.Payment
	createErr error
	updateErr error
}

func (f *fakePaymentRepository) Create(_ context.Context, payment *domain.Payment) error {
	if f.createErr != nil {
		return f.createErr
	}
	snapshot := *payment // capture state at call time — the caller mutates payment afterward
	f.created = &snapshot
	return nil
}

func (f *fakePaymentRepository) GetByID(_ context.Context, _ uuid.UUID) (*domain.Payment, error) {
	return nil, errors.New("not implemented")
}

func (f *fakePaymentRepository) Update(_ context.Context, payment *domain.Payment) error {
	if f.updateErr != nil {
		return f.updateErr
	}
	snapshot := *payment
	f.updated = &snapshot
	return nil
}

// MarkProcessedAndUpdate is not exercised by these tests (webhook
// idempotency is covered in the webhook package's own tests) — this
// stub exists only to satisfy repository.PaymentRepository.
func (f *fakePaymentRepository) MarkProcessedAndUpdate(ctx context.Context, payment *domain.Payment, _ string) (bool, error) {
	return false, f.Update(ctx, payment)
}

type fakePaymentProvider struct {
	result provider.InitializeTransactionResult
	err    error
}

func (f fakePaymentProvider) InitializeTransaction(_ context.Context, _ provider.InitializeTransactionInput) (provider.InitializeTransactionResult, error) {
	return f.result, f.err
}

func (f fakePaymentProvider) VerifyTransaction(_ context.Context, _ string) (provider.VerifyTransactionResult, error) {
	return provider.VerifyTransactionResult{}, errors.New("not implemented")
}

func TestPaymentService_CreatePayment_Success(t *testing.T) {
	repo := &fakePaymentRepository{}
	paymentProvider := fakePaymentProvider{result: provider.InitializeTransactionResult{
		AuthorizationURL: "https://checkout.paystack.com/abc123",
		AccessCode:       "abc123",
		Reference:        "will-be-overwritten-by-payment-id-in-real-flow",
	}}
	svc := application.NewPaymentService(repo, paymentProvider)
	orderID := uuid.New()

	payment, authURL, err := svc.CreatePayment(context.Background(), orderID.String(), "customer@example.com", 25000, "GHS")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if payment.ID == uuid.Nil {
		t.Error("expected a server-generated ID, got nil UUID")
	}
	if payment.OrderID != orderID {
		t.Errorf("expected order id %v, got %v", orderID, payment.OrderID)
	}
	if payment.Status != domain.StatusInitialized {
		t.Errorf("expected status %q, got %q", domain.StatusInitialized, payment.Status)
	}
	if payment.ProviderReference != paymentProvider.result.Reference {
		t.Errorf("expected provider reference %q, got %q", paymentProvider.result.Reference, payment.ProviderReference)
	}
	if authURL != paymentProvider.result.AuthorizationURL {
		t.Errorf("expected authorization url %q, got %q", paymentProvider.result.AuthorizationURL, authURL)
	}
	if repo.created == nil {
		t.Fatal("expected repository.Create to be called with the initial PENDING attempt")
	}
	if repo.created.Status != domain.StatusPending {
		t.Errorf("expected the created record to start PENDING, got %q", repo.created.Status)
	}
	if repo.updated == nil {
		t.Fatal("expected repository.Update to be called after provider initialization")
	}
}

func TestPaymentService_CreatePayment_ProviderFailure(t *testing.T) {
	repo := &fakePaymentRepository{}
	paymentProvider := fakePaymentProvider{err: errors.New("paystack unreachable")}
	svc := application.NewPaymentService(repo, paymentProvider)

	_, _, err := svc.CreatePayment(context.Background(), uuid.New().String(), "customer@example.com", 25000, "GHS")
	if err == nil {
		t.Fatal("expected an error when the provider fails")
	}
	if repo.created == nil {
		t.Fatal("expected the PENDING attempt to still be recorded")
	}
	if repo.updated == nil {
		t.Fatal("expected the payment to be marked FAILED via Update")
	}
	if repo.updated.Status != domain.StatusFailed {
		t.Errorf("expected status %q after provider failure, got %q", domain.StatusFailed, repo.updated.Status)
	}
}

func TestPaymentService_CreatePayment_Validation(t *testing.T) {
	validOrderID := uuid.New().String()
	validEmail := "customer@example.com"

	tests := []struct {
		name        string
		orderID     string
		email       string
		amountMinor int64
		currency    string
		wantErr     error
	}{
		{"invalid order id", "not-a-uuid", validEmail, 25000, "GHS", application.ErrInvalidOrderID},
		{"empty order id", "", validEmail, 25000, "GHS", application.ErrInvalidOrderID},
		{"invalid email", validOrderID, "not-an-email", 25000, "GHS", application.ErrInvalidEmail},
		{"zero amount", validOrderID, validEmail, 0, "GHS", application.ErrInvalidAmount},
		{"negative amount", validOrderID, validEmail, -100, "GHS", application.ErrInvalidAmount},
		{"unsupported currency", validOrderID, validEmail, 25000, "XYZ", application.ErrUnsupportedCurrency},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &fakePaymentRepository{}
			svc := application.NewPaymentService(repo, fakePaymentProvider{})

			_, _, err := svc.CreatePayment(context.Background(), tt.orderID, tt.email, tt.amountMinor, tt.currency)
			if !errors.Is(err, tt.wantErr) {
				t.Errorf("expected error %v, got %v", tt.wantErr, err)
			}
			if repo.created != nil {
				t.Error("expected repository.Create not to be called on validation failure")
			}
		})
	}
}

func TestPaymentService_CreatePayment_RepositoryCreateError(t *testing.T) {
	repo := &fakePaymentRepository{createErr: errors.New("connection lost")}
	svc := application.NewPaymentService(repo, fakePaymentProvider{})

	_, _, err := svc.CreatePayment(context.Background(), uuid.New().String(), "customer@example.com", 25000, "GHS")
	if err == nil {
		t.Fatal("expected an error when the repository fails to create")
	}
}
