package application_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/Afari-Richmond/payflow/services/order-service/internal/application"
	"github.com/Afari-Richmond/payflow/services/order-service/internal/domain"
)

type fakeOrderRepository struct {
	created *domain.Order
	err     error
}

func (f *fakeOrderRepository) Create(_ context.Context, order *domain.Order) error {
	if f.err != nil {
		return f.err
	}
	f.created = order
	return nil
}

func (f *fakeOrderRepository) GetByID(_ context.Context, _ uuid.UUID) (*domain.Order, error) {
	return nil, errors.New("not implemented")
}

func TestOrderService_CreateOrder_Success(t *testing.T) {
	repo := &fakeOrderRepository{}
	svc := application.NewOrderService(repo)

	order, err := svc.CreateOrder(context.Background(), "customer@example.com", 25000, "GHS")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if order.ID == uuid.Nil {
		t.Error("expected a server-generated ID, got nil UUID")
	}
	if order.Status != domain.StatusPendingPayment {
		t.Errorf("expected status %q, got %q", domain.StatusPendingPayment, order.Status)
	}
	if repo.created == nil {
		t.Fatal("expected repository.Create to be called")
	}
}

func TestOrderService_CreateOrder_Validation(t *testing.T) {
	tests := []struct {
		name        string
		email       string
		amountMinor int64
		currency    string
		wantErr     error
	}{
		{"invalid email", "not-an-email", 25000, "GHS", application.ErrInvalidEmail},
		{"empty email", "", 25000, "GHS", application.ErrInvalidEmail},
		{"zero amount", "customer@example.com", 0, "GHS", application.ErrInvalidAmount},
		{"negative amount", "customer@example.com", -100, "GHS", application.ErrInvalidAmount},
		{"unsupported currency", "customer@example.com", 25000, "XYZ", application.ErrUnsupportedCurrency},
		{"empty currency", "customer@example.com", 25000, "", application.ErrUnsupportedCurrency},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &fakeOrderRepository{}
			svc := application.NewOrderService(repo)

			_, err := svc.CreateOrder(context.Background(), tt.email, tt.amountMinor, tt.currency)
			if !errors.Is(err, tt.wantErr) {
				t.Errorf("expected error %v, got %v", tt.wantErr, err)
			}
			if repo.created != nil {
				t.Error("expected repository.Create not to be called on validation failure")
			}
		})
	}
}

func TestOrderService_CreateOrder_RepositoryError(t *testing.T) {
	repo := &fakeOrderRepository{err: errors.New("connection lost")}
	svc := application.NewOrderService(repo)

	_, err := svc.CreateOrder(context.Background(), "customer@example.com", 25000, "GHS")
	if err == nil {
		t.Fatal("expected an error when the repository fails")
	}
}
