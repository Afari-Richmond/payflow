package application_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/Afari-Richmond/payflow/services/payment-service/internal/application"
	"github.com/Afari-Richmond/payflow/services/payment-service/internal/domain"
)

type fakePaymentRepository struct {
	created *domain.Payment
	err     error
}

func (f *fakePaymentRepository) Create(_ context.Context, payment *domain.Payment) error {
	if f.err != nil {
		return f.err
	}
	f.created = payment
	return nil
}

func (f *fakePaymentRepository) GetByID(_ context.Context, _ uuid.UUID) (*domain.Payment, error) {
	return nil, errors.New("not implemented")
}

func TestPaymentService_CreatePayment_Success(t *testing.T) {
	repo := &fakePaymentRepository{}
	svc := application.NewPaymentService(repo)
	orderID := uuid.New()

	payment, err := svc.CreatePayment(context.Background(), orderID.String(), 25000, "GHS")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if payment.ID == uuid.Nil {
		t.Error("expected a server-generated ID, got nil UUID")
	}
	if payment.OrderID != orderID {
		t.Errorf("expected order id %v, got %v", orderID, payment.OrderID)
	}
	if payment.Status != domain.StatusPending {
		t.Errorf("expected status %q, got %q", domain.StatusPending, payment.Status)
	}
	if payment.ProviderReference != "" {
		t.Errorf("expected empty provider reference before Milestone 7, got %q", payment.ProviderReference)
	}
	if repo.created == nil {
		t.Fatal("expected repository.Create to be called")
	}
}

func TestPaymentService_CreatePayment_Validation(t *testing.T) {
	validOrderID := uuid.New().String()

	tests := []struct {
		name        string
		orderID     string
		amountMinor int64
		currency    string
		wantErr     error
	}{
		{"invalid order id", "not-a-uuid", 25000, "GHS", application.ErrInvalidOrderID},
		{"empty order id", "", 25000, "GHS", application.ErrInvalidOrderID},
		{"zero amount", validOrderID, 0, "GHS", application.ErrInvalidAmount},
		{"negative amount", validOrderID, -100, "GHS", application.ErrInvalidAmount},
		{"unsupported currency", validOrderID, 25000, "XYZ", application.ErrUnsupportedCurrency},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &fakePaymentRepository{}
			svc := application.NewPaymentService(repo)

			_, err := svc.CreatePayment(context.Background(), tt.orderID, tt.amountMinor, tt.currency)
			if !errors.Is(err, tt.wantErr) {
				t.Errorf("expected error %v, got %v", tt.wantErr, err)
			}
			if repo.created != nil {
				t.Error("expected repository.Create not to be called on validation failure")
			}
		})
	}
}

func TestPaymentService_CreatePayment_RepositoryError(t *testing.T) {
	repo := &fakePaymentRepository{err: errors.New("connection lost")}
	svc := application.NewPaymentService(repo)

	_, err := svc.CreatePayment(context.Background(), uuid.New().String(), 25000, "GHS")
	if err == nil {
		t.Fatal("expected an error when the repository fails")
	}
}
