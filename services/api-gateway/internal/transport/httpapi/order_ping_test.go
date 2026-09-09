package httpapi_test

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Afari-Richmond/payflow/services/api-gateway/internal/transport/httpapi"
)

func TestOrderPingEndpoint_Success(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	pinger := fakeOrderPinger{message: "order-service received: ping from api-gateway"}
	router := httpapi.NewRouter(logger, pinger)

	req := httptest.NewRequest(http.MethodGet, "/internal/order-ping", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rec.Code)
	}

	var body httpapi.OrderPingResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("failed to decode response body: %v", err)
	}

	if body.Message != pinger.message {
		t.Errorf("expected message %q, got %q", pinger.message, body.Message)
	}
}

func TestOrderPingEndpoint_OrderServiceUnreachable(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	pinger := fakeOrderPinger{err: errors.New("connection refused")}
	router := httpapi.NewRouter(logger, pinger)

	req := httptest.NewRequest(http.MethodGet, "/internal/order-ping", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadGateway {
		t.Fatalf("expected status %d, got %d", http.StatusBadGateway, rec.Code)
	}
}
