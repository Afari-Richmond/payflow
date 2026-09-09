package grpcapi_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	orderv1 "github.com/Afari-Richmond/payflow/proto/order/v1"
	"github.com/Afari-Richmond/payflow/services/order-service/internal/application"
	"github.com/Afari-Richmond/payflow/services/order-service/internal/domain"
	"github.com/Afari-Richmond/payflow/services/order-service/internal/transport/grpcapi"
)

type fakeOrderCreator struct {
	order *domain.Order
	err   error
}

func (f fakeOrderCreator) CreateOrder(_ context.Context, _ string, _ int64, _ string) (*domain.Order, error) {
	return f.order, f.err
}

func TestOrderServer_CreateOrder_Success(t *testing.T) {
	now := time.Now().UTC()
	order := &domain.Order{
		ID:          uuid.New(),
		Email:       "customer@example.com",
		AmountMinor: 25000,
		Currency:    "GHS",
		Status:      domain.StatusPendingPayment,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	server := grpcapi.NewOrderServer(fakeOrderCreator{order: order})

	resp, err := server.CreateOrder(context.Background(), &orderv1.CreateOrderRequest{
		Email:       "customer@example.com",
		AmountMinor: 25000,
		Currency:    "GHS",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.GetOrder().GetId() != order.ID.String() {
		t.Errorf("expected id %q, got %q", order.ID.String(), resp.GetOrder().GetId())
	}
	if resp.GetOrder().GetStatus() != string(domain.StatusPendingPayment) {
		t.Errorf("expected status %q, got %q", domain.StatusPendingPayment, resp.GetOrder().GetStatus())
	}
}

func TestOrderServer_CreateOrder_ValidationError(t *testing.T) {
	server := grpcapi.NewOrderServer(fakeOrderCreator{err: application.ErrInvalidAmount})

	_, err := server.CreateOrder(context.Background(), &orderv1.CreateOrderRequest{
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

func TestOrderServer_CreateOrder_InternalError(t *testing.T) {
	server := grpcapi.NewOrderServer(fakeOrderCreator{err: errors.New("db connection lost")})

	_, err := server.CreateOrder(context.Background(), &orderv1.CreateOrderRequest{
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
