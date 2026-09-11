package grpcapi_test

import (
	"context"
	"errors"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	paymentv1 "github.com/Afari-Richmond/payflow/proto/payment/v1"
	"github.com/Afari-Richmond/payflow/services/payment-service/internal/transport/grpcapi"
	"github.com/Afari-Richmond/payflow/services/payment-service/internal/webhook"
)

type fakeWebhookHandler struct {
	err error
}

func (f fakeWebhookHandler) HandleWebhook(_ context.Context, _ []byte, _ string) error {
	return f.err
}

func TestPaymentServer_HandleWebhook_Success(t *testing.T) {
	server := grpcapi.NewPaymentServer(fakePaymentCreator{}, fakeWebhookHandler{})

	_, err := server.HandleWebhook(context.Background(), &paymentv1.HandleWebhookRequest{
		RawBody:   []byte(`{"event":"charge.success","data":{"reference":"abc"}}`),
		Signature: "valid-signature",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestPaymentServer_HandleWebhook_InvalidSignature(t *testing.T) {
	server := grpcapi.NewPaymentServer(fakePaymentCreator{}, fakeWebhookHandler{err: webhook.ErrInvalidSignature})

	_, err := server.HandleWebhook(context.Background(), &paymentv1.HandleWebhookRequest{
		RawBody:   []byte(`{}`),
		Signature: "bad-signature",
	})

	st, ok := status.FromError(err)
	if !ok {
		t.Fatalf("expected a gRPC status error, got %v", err)
	}
	if st.Code() != codes.Unauthenticated {
		t.Errorf("expected code %v, got %v", codes.Unauthenticated, st.Code())
	}
}

func TestPaymentServer_HandleWebhook_MalformedPayload(t *testing.T) {
	server := grpcapi.NewPaymentServer(fakePaymentCreator{}, fakeWebhookHandler{err: webhook.ErrMalformedPayload})

	_, err := server.HandleWebhook(context.Background(), &paymentv1.HandleWebhookRequest{
		RawBody:   []byte(`not json`),
		Signature: "valid-signature",
	})

	st, ok := status.FromError(err)
	if !ok {
		t.Fatalf("expected a gRPC status error, got %v", err)
	}
	if st.Code() != codes.InvalidArgument {
		t.Errorf("expected code %v, got %v", codes.InvalidArgument, st.Code())
	}
}

func TestPaymentServer_HandleWebhook_InternalError(t *testing.T) {
	server := grpcapi.NewPaymentServer(fakePaymentCreator{}, fakeWebhookHandler{err: errors.New("db connection lost")})

	_, err := server.HandleWebhook(context.Background(), &paymentv1.HandleWebhookRequest{
		RawBody:   []byte(`{"event":"charge.success","data":{"reference":"abc"}}`),
		Signature: "valid-signature",
	})

	st, ok := status.FromError(err)
	if !ok {
		t.Fatalf("expected a gRPC status error, got %v", err)
	}
	if st.Code() != codes.Internal {
		t.Errorf("expected code %v, got %v", codes.Internal, st.Code())
	}
}
