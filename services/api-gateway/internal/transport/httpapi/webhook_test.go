package httpapi_test

import (
	"bytes"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/Afari-Richmond/payflow/services/api-gateway/internal/transport/httpapi"
)

func postWebhook(t *testing.T, router http.Handler, body []byte, signature string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/webhooks/paystack", bytes.NewReader(body))
	if signature != "" {
		req.Header.Set("X-Paystack-Signature", signature)
	}
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func TestPaystackWebhookEndpoint_Success(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	router := httpapi.NewRouter(logger, fakeOrderCreator{}, fakeWebhookForwarder{})

	rec := postWebhook(t, router, []byte(`{"event":"charge.success","data":{"reference":"abc"}}`), "some-signature")

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rec.Code)
	}
}

func TestPaystackWebhookEndpoint_InvalidSignature(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	router := httpapi.NewRouter(logger, fakeOrderCreator{}, fakeWebhookForwarder{
		err: status.Error(codes.Unauthenticated, "invalid webhook signature"),
	})

	rec := postWebhook(t, router, []byte(`{}`), "bad-signature")

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected status %d, got %d", http.StatusUnauthorized, rec.Code)
	}
}

func TestPaystackWebhookEndpoint_MalformedPayload(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	router := httpapi.NewRouter(logger, fakeOrderCreator{}, fakeWebhookForwarder{
		err: status.Error(codes.InvalidArgument, "malformed webhook payload"),
	})

	rec := postWebhook(t, router, []byte(`not json`), "some-signature")

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d", http.StatusBadRequest, rec.Code)
	}
}

func TestPaystackWebhookEndpoint_PaymentServiceUnreachable(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	router := httpapi.NewRouter(logger, fakeOrderCreator{}, fakeWebhookForwarder{
		err: status.Error(codes.Unavailable, "connection refused"),
	})

	rec := postWebhook(t, router, []byte(`{"event":"charge.success","data":{"reference":"abc"}}`), "some-signature")

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected status %d, got %d", http.StatusInternalServerError, rec.Code)
	}
}
