// Package grpcapi implements order-service's gRPC transport: the
// OrderService contract defined in proto/order/v1. Handlers here only
// validate, translate, and call the application layer — no business
// logic lives here.
package grpcapi

import (
	"context"
	"errors"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	orderv1 "github.com/Afari-Richmond/payflow/proto/order/v1"
	"github.com/Afari-Richmond/payflow/services/order-service/internal/application"
	"github.com/Afari-Richmond/payflow/services/order-service/internal/domain"
)

// OrderCreator is the application-layer dependency this server needs.
// A narrow interface (not the concrete *application.OrderService) so
// the handler can be tested without real persistence.
type OrderCreator interface {
	CreateOrder(ctx context.Context, email string, amountMinor int64, currency string) (*domain.Order, error)
}

// OrderServer implements orderv1.OrderServiceServer.
type OrderServer struct {
	orderv1.UnimplementedOrderServiceServer
	orders OrderCreator
}

// NewOrderServer builds an OrderServer.
func NewOrderServer(orders OrderCreator) *OrderServer {
	return &OrderServer{orders: orders}
}

// CreateOrder handles the CreateOrder RPC.
func (s *OrderServer) CreateOrder(ctx context.Context, req *orderv1.CreateOrderRequest) (*orderv1.CreateOrderResponse, error) {
	order, err := s.orders.CreateOrder(ctx, req.GetEmail(), req.GetAmountMinor(), req.GetCurrency())
	if err != nil {
		switch {
		case errors.Is(err, application.ErrInvalidEmail),
			errors.Is(err, application.ErrInvalidAmount),
			errors.Is(err, application.ErrUnsupportedCurrency):
			return nil, status.Error(codes.InvalidArgument, err.Error())
		default:
			return nil, status.Error(codes.Internal, "failed to create order")
		}
	}

	return &orderv1.CreateOrderResponse{
		Order: toProto(order),
	}, nil
}

func toProto(o *domain.Order) *orderv1.Order {
	return &orderv1.Order{
		Id:          o.ID.String(),
		Email:       o.Email,
		AmountMinor: o.AmountMinor,
		Currency:    o.Currency,
		Status:      string(o.Status),
		CreatedAt:   o.CreatedAt.Format(time.RFC3339Nano),
		UpdatedAt:   o.UpdatedAt.Format(time.RFC3339Nano),
	}
}
