package httpapi_test

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/Afari-Richmond/payflow/services/api-gateway/internal/client"
	"github.com/Afari-Richmond/payflow/services/api-gateway/internal/transport/httpapi"
)

func postOrder(t *testing.T, router http.Handler, body any) *httptest.ResponseRecorder {
	t.Helper()
	b, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("failed to marshal request body: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/orders", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func TestCreateOrderEndpoint_Success(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	order := &client.Order{
		ID:          "11111111-1111-1111-1111-111111111111",
		Email:       "customer@example.com",
		AmountMinor: 25000,
		Currency:    "GHS",
		Status:      "PENDING_PAYMENT",
		CreatedAt:   "2026-09-09T00:00:00Z",
		UpdatedAt:   "2026-09-09T00:00:00Z",
	}
	router := httpapi.NewRouter(logger, fakeOrderCreator{order: order}, fakeWebhookForwarder{}, fakeReadinessChecker{})

	rec := postOrder(t, router, map[string]any{
		"email":    "customer@example.com",
		"amount":   25000,
		"currency": "GHS",
	})

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected status %d, got %d: %s", http.StatusCreated, rec.Code, rec.Body.String())
	}

	var resp httpapi.OrderResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp.Status != "PENDING_PAYMENT" {
		t.Errorf("expected status PENDING_PAYMENT, got %q", resp.Status)
	}
	if resp.Amount != 25000 {
		t.Errorf("expected amount 25000, got %d", resp.Amount)
	}
}

func TestCreateOrderEndpoint_ValidationRejectedByGateway(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	router := httpapi.NewRouter(logger, fakeOrderCreator{}, fakeWebhookForwarder{}, fakeReadinessChecker{})

	tests := []map[string]any{
		{"email": "not-an-email", "amount": 25000, "currency": "GHS"},
		{"email": "customer@example.com", "amount": -5, "currency": "GHS"},
		{"email": "customer@example.com", "amount": 25000, "currency": "GH"},
		{"email": "", "amount": 25000, "currency": "GHS"},
	}

	for _, body := range tests {
		rec := postOrder(t, router, body)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("body %v: expected status %d, got %d", body, http.StatusBadRequest, rec.Code)
		}
	}
}

func TestCreateOrderEndpoint_OrderServiceValidationError(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	router := httpapi.NewRouter(logger, fakeOrderCreator{
		err: status.Error(codes.InvalidArgument, "unsupported currency"),
	}, fakeWebhookForwarder{}, fakeReadinessChecker{})

	rec := postOrder(t, router, map[string]any{
		"email":    "customer@example.com",
		"amount":   25000,
		"currency": "ZZZ",
	})

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d", http.StatusBadRequest, rec.Code)
	}
}

func TestCreateOrderEndpoint_OrderServiceUnreachable(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	router := httpapi.NewRouter(logger, fakeOrderCreator{
		err: status.Error(codes.Unavailable, "connection refused"),
	}, fakeWebhookForwarder{}, fakeReadinessChecker{})

	rec := postOrder(t, router, map[string]any{
		"email":    "customer@example.com",
		"amount":   25000,
		"currency": "GHS",
	})

	if rec.Code != http.StatusBadGateway {
		t.Fatalf("expected status %d, got %d", http.StatusBadGateway, rec.Code)
	}
}
