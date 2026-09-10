package grpcapi_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	paymentv1 "github.com/Afari-Richmond/payflow/proto/payment/v1"
	"github.com/Afari-Richmond/payflow/services/payment-service/internal/application"
	"github.com/Afari-Richmond/payflow/services/payment-service/internal/domain"
	"github.com/Afari-Richmond/payflow/services/payment-service/internal/transport/grpcapi"
)

type fakePaymentCreator struct {
	payment          *domain.Payment
	authorizationURL string
	err              error
}

func (f fakePaymentCreator) CreatePayment(_ context.Context, _, _ string, _ int64, _ string) (*domain.Payment, string, error) {
	return f.payment, f.authorizationURL, f.err
}

func TestPaymentServer_CreatePayment_Success(t *testing.T) {
	now := time.Now().UTC()
	payment := &domain.Payment{
		ID:                uuid.New(),
		OrderID:           uuid.New(),
		AmountMinor:       25000,
		Currency:          "GHS",
		Status:            domain.StatusInitialized,
		ProviderReference: "ref-123",
		CreatedAt:         now,
		UpdatedAt:         now,
	}
	server := grpcapi.NewPaymentServer(fakePaymentCreator{
		payment:          payment,
		authorizationURL: "https://checkout.paystack.com/abc123",
	})

	resp, err := server.CreatePayment(context.Background(), &paymentv1.CreatePaymentRequest{
		OrderId:     payment.OrderID.String(),
		Email:       "customer@example.com",
		AmountMinor: 25000,
		Currency:    "GHS",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.GetPayment().GetId() != payment.ID.String() {
		t.Errorf("expected id %q, got %q", payment.ID.String(), resp.GetPayment().GetId())
	}
	if resp.GetPayment().GetStatus() != string(domain.StatusInitialized) {
		t.Errorf("expected status %q, got %q", domain.StatusInitialized, resp.GetPayment().GetStatus())
	}
	if resp.GetAuthorizationUrl() != "https://checkout.paystack.com/abc123" {
		t.Errorf("expected authorization url to be returned, got %q", resp.GetAuthorizationUrl())
	}
}

func TestPaymentServer_CreatePayment_ValidationError(t *testing.T) {
	server := grpcapi.NewPaymentServer(fakePaymentCreator{err: application.ErrInvalidAmount})

	_, err := server.CreatePayment(context.Background(), &paymentv1.CreatePaymentRequest{
		OrderId:     uuid.New().String(),
		Email:       "customer@example.com",
		AmountMinor: -1,
		Currency:    "GHS",
	})

	st, ok := status.FromError(err)
	if !ok {
		t.Fatalf("expected a gRPC status error, got %v", err)
	}
	if st.Code() != codes.InvalidArgument {
		t.Errorf("expected code %v, got %v", codes.InvalidArgument, st.Code())
	}
}

func TestPaymentServer_CreatePayment_InternalError(t *testing.T) {
	server := grpcapi.NewPaymentServer(fakePaymentCreator{err: errors.New("db connection lost")})

	_, err := server.CreatePayment(context.Background(), &paymentv1.CreatePaymentRequest{
		OrderId:     uuid.New().String(),
		Email:       "customer@example.com",
		AmountMinor: 25000,
		Currency:    "GHS",
	})

	st, ok := status.FromError(err)
	if !ok {
		t.Fatalf("expected a gRPC status error, got %v", err)
	}
	if st.Code() != codes.Internal {
		t.Errorf("expected code %v, got %v", codes.Internal, st.Code())
	}
}
