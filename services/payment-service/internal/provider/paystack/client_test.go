package paystack_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Afari-Richmond/payflow/services/payment-service/internal/provider"
	"github.com/Afari-Richmond/payflow/services/payment-service/internal/provider/paystack"
)

func TestClient_InitializeTransaction_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/transaction/initialize" {
			t.Errorf("expected path /transaction/initialize, got %s", r.URL.Path)
		}
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer sk_test_fake" {
			t.Errorf("expected Authorization header 'Bearer sk_test_fake', got %q", got)
		}

		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("failed to decode request body: %v", err)
		}
		if body["email"] != "customer@example.com" {
			t.Errorf("expected email in request body, got %v", body["email"])
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status":  true,
			"message": "Authorization URL created",
			"data": map[string]any{
				"authorization_url": "https://checkout.paystack.com/abc123",
				"access_code":       "abc123",
				"reference":         "ref-123",
			},
		})
	}))
	defer server.Close()

	client := paystack.NewClient("sk_test_fake", paystack.WithBaseURL(server.URL))

	result, err := client.InitializeTransaction(context.Background(), provider.InitializeTransactionInput{
		Email:       "customer@example.com",
		AmountMinor: 25000,
		Currency:    "GHS",
		Reference:   "ref-123",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.AuthorizationURL != "https://checkout.paystack.com/abc123" {
		t.Errorf("unexpected authorization url: %q", result.AuthorizationURL)
	}
	if result.Reference != "ref-123" {
		t.Errorf("unexpected reference: %q", result.Reference)
	}
}

func TestClient_InitializeTransaction_ProviderRejection(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status":  false,
			"message": "Invalid currency",
		})
	}))
	defer server.Close()

	client := paystack.NewClient("sk_test_fake", paystack.WithBaseURL(server.URL))

	_, err := client.InitializeTransaction(context.Background(), provider.InitializeTransactionInput{
		Email:       "customer@example.com",
		AmountMinor: 25000,
		Currency:    "ZZZ",
		Reference:   "ref-123",
	})
	if err == nil {
		t.Fatal("expected an error when Paystack rejects the request")
	}
}

func TestClient_InitializeTransaction_ServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	client := paystack.NewClient("sk_test_fake", paystack.WithBaseURL(server.URL))

	_, err := client.InitializeTransaction(context.Background(), provider.InitializeTransactionInput{
		Email:       "customer@example.com",
		AmountMinor: 25000,
		Currency:    "GHS",
		Reference:   "ref-123",
	})
	if err == nil {
		t.Fatal("expected an error on a provider-side server error")
	}
}

func TestClient_VerifyTransaction_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/transaction/verify/ref-123" {
			t.Errorf("expected path /transaction/verify/ref-123, got %s", r.URL.Path)
		}
		if r.Method != http.MethodGet {
			t.Errorf("expected GET, got %s", r.Method)
		}

		_ = json.NewEncoder(w).Encode(map[string]any{
			"status":  true,
			"message": "Verification successful",
			"data": map[string]any{
				"reference": "ref-123",
				"status":    "success",
				"amount":    25000,
				"currency":  "GHS",
				"paid_at":   "2026-09-09T12:00:00.000Z",
			},
		})
	}))
	defer server.Close()

	client := paystack.NewClient("sk_test_fake", paystack.WithBaseURL(server.URL))

	result, err := client.VerifyTransaction(context.Background(), "ref-123")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Status != provider.TransactionStatusSuccess {
		t.Errorf("expected status success, got %q", result.Status)
	}
	if result.AmountMinor != 25000 {
		t.Errorf("expected amount 25000, got %d", result.AmountMinor)
	}
	if result.PaidAt.IsZero() {
		t.Error("expected a non-zero PaidAt")
	}
}

func TestClient_VerifyTransaction_StatusTranslation(t *testing.T) {
	tests := []struct {
		paystackStatus string
		want           provider.TransactionStatus
	}{
		{"success", provider.TransactionStatusSuccess},
		{"abandoned", provider.TransactionStatusAbandoned},
		{"failed", provider.TransactionStatusFailed},
		{"some-unknown-future-status", provider.TransactionStatusFailed},
	}

	for _, tt := range tests {
		t.Run(tt.paystackStatus, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_ = json.NewEncoder(w).Encode(map[string]any{
					"status": true,
					"data": map[string]any{
						"reference": "ref-123",
						"status":    tt.paystackStatus,
						"amount":    25000,
						"currency":  "GHS",
					},
				})
			}))
			defer server.Close()

			client := paystack.NewClient("sk_test_fake", paystack.WithBaseURL(server.URL))
			result, err := client.VerifyTransaction(context.Background(), "ref-123")
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if result.Status != tt.want {
				t.Errorf("expected status %q, got %q", tt.want, result.Status)
			}
		})
	}
}
