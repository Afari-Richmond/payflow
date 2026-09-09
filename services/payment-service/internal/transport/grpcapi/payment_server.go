// Package grpcapi implements payment-service's gRPC transport: the
// PaymentService contract defined in proto/payment/v1. Handlers here
// only validate, translate, and call the application layer — no
// business logic lives here.
package grpcapi

import (
	"context"
	"errors"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	paymentv1 "github.com/Afari-Richmond/payflow/proto/payment/v1"
	"github.com/Afari-Richmond/payflow/services/payment-service/internal/application"
	"github.com/Afari-Richmond/payflow/services/payment-service/internal/domain"
)

// PaymentCreator is the application-layer dependency this server
// needs. A narrow interface (not the concrete *application.PaymentService)
// so the handler can be tested without real persistence.
type PaymentCreator interface {
	CreatePayment(ctx context.Context, orderID string, amountMinor int64, currency string) (*domain.Payment, error)
}

// PaymentServer implements paymentv1.PaymentServiceServer.
type PaymentServer struct {
	paymentv1.UnimplementedPaymentServiceServer
	payments PaymentCreator
}

// NewPaymentServer builds a PaymentServer.
func NewPaymentServer(payments PaymentCreator) *PaymentServer {
	return &PaymentServer{payments: payments}
}

// CreatePayment handles the CreatePayment RPC.
func (s *PaymentServer) CreatePayment(ctx context.Context, req *paymentv1.CreatePaymentRequest) (*paymentv1.CreatePaymentResponse, error) {
	payment, err := s.payments.CreatePayment(ctx, req.GetOrderId(), req.GetAmountMinor(), req.GetCurrency())
	if err != nil {
		switch {
		case errors.Is(err, application.ErrInvalidOrderID),
			errors.Is(err, application.ErrInvalidAmount),
			errors.Is(err, application.ErrUnsupportedCurrency):
			return nil, status.Error(codes.InvalidArgument, err.Error())
		default:
			return nil, status.Error(codes.Internal, "failed to create payment")
		}
	}

	return &paymentv1.CreatePaymentResponse{
		Payment: toProto(payment),
	}, nil
}

func toProto(p *domain.Payment) *paymentv1.Payment {
	return &paymentv1.Payment{
		Id:                p.ID.String(),
		OrderId:           p.OrderID.String(),
		AmountMinor:       p.AmountMinor,
		Currency:          p.Currency,
		Status:            string(p.Status),
		ProviderReference: p.ProviderReference,
		CreatedAt:         p.CreatedAt.Format(time.RFC3339Nano),
		UpdatedAt:         p.UpdatedAt.Format(time.RFC3339Nano),
	}
}
