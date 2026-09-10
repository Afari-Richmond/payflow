package paystack_test

import (
	"context"
	"os"
	"testing"

	"github.com/Afari-Richmond/payflow/services/payment-service/internal/provider"
	"github.com/Afari-Richmond/payflow/services/payment-service/internal/provider/paystack"
)

// TestClient_InitializeTransaction_RealPaystack is a selected
// integration test against the real Paystack API, per the project's
// testing philosophy: normal tests use a fake server (see
// client_test.go); real test-mode credentials are reserved for tests
// like this one, run explicitly and skipped otherwise.
//
// Run with: PAYSTACK_SECRET_KEY=sk_test_... go test -run RealPaystack ./...
func TestClient_InitializeTransaction_RealPaystack(t *testing.T) {
	secretKey := os.Getenv("PAYSTACK_SECRET_KEY")
	if secretKey == "" {
		t.Skip("skipping: PAYSTACK_SECRET_KEY not set (this test only runs against a real Paystack test-mode account)")
	}

	client := paystack.NewClient(secretKey)

	result, err := client.InitializeTransaction(context.Background(), provider.InitializeTransactionInput{
		Email:       "customer@example.com",
		AmountMinor: 25000,
		Currency:    "GHS",
		Reference:   "payflow-integration-test-" + t.Name(),
	})
	if err != nil {
		t.Fatalf("InitializeTransaction failed against the real Paystack API: %v", err)
	}
	if result.AuthorizationURL == "" {
		t.Error("expected a real authorization URL from Paystack")
	}
}
