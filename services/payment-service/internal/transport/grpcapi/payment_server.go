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
	"github.com/Afari-Richmond/payflow/services/payment-service/internal/webhook"
)

// PaymentCreator is the application-layer dependency this server
// needs. A narrow interface (not the concrete *application.PaymentService)
// so the handler can be tested without real persistence or a real
// payment provider.
type PaymentCreator interface {
	CreatePayment(ctx context.Context, orderID, email string, amountMinor int64, currency string) (*domain.Payment, string, error)
}

// WebhookHandler is the webhook-processing dependency this server
// needs. A narrow interface (not the concrete *webhook.Service) so the
// handler can be tested without real signature verification.
type WebhookHandler interface {
	HandleWebhook(ctx context.Context, rawBody []byte, signature string) error
}

// PaymentServer implements paymentv1.PaymentServiceServer.
type PaymentServer struct {
	paymentv1.UnimplementedPaymentServiceServer
	payments PaymentCreator
	webhooks WebhookHandler
}

// NewPaymentServer builds a PaymentServer.
func NewPaymentServer(payments PaymentCreator, webhooks WebhookHandler) *PaymentServer {
	return &PaymentServer{payments: payments, webhooks: webhooks}
}

// CreatePayment handles the CreatePayment RPC.
func (s *PaymentServer) CreatePayment(ctx context.Context, req *paymentv1.CreatePaymentRequest) (*paymentv1.CreatePaymentResponse, error) {
	payment, authorizationURL, err := s.payments.CreatePayment(ctx, req.GetOrderId(), req.GetEmail(), req.GetAmountMinor(), req.GetCurrency())
	if err != nil {
		switch {
		case errors.Is(err, application.ErrInvalidOrderID),
			errors.Is(err, application.ErrInvalidEmail),
			errors.Is(err, application.ErrInvalidAmount),
			errors.Is(err, application.ErrUnsupportedCurrency):
			return nil, status.Error(codes.InvalidArgument, err.Error())
		default:
			return nil, status.Error(codes.Internal, "failed to create payment")
		}
	}

	return &paymentv1.CreatePaymentResponse{
		Payment:          toProto(payment),
		AuthorizationUrl: authorizationURL,
	}, nil
}

// HandleWebhook handles the HandleWebhook RPC. See ADR 002 for why the
// gateway forwards raw bytes here rather than verifying itself.
func (s *PaymentServer) HandleWebhook(ctx context.Context, req *paymentv1.HandleWebhookRequest) (*paymentv1.HandleWebhookResponse, error) {
	err := s.webhooks.HandleWebhook(ctx, req.GetRawBody(), req.GetSignature())
	switch {
	case err == nil:
		return &paymentv1.HandleWebhookResponse{}, nil
	case errors.Is(err, webhook.ErrInvalidSignature):
		return nil, status.Error(codes.Unauthenticated, err.Error())
	case errors.Is(err, webhook.ErrMalformedPayload):
		return nil, status.Error(codes.InvalidArgument, err.Error())
	default:
		return nil, status.Error(codes.Internal, "failed to process webhook")
	}
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
