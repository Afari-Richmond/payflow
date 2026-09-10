// Package paystack implements provider.PaymentProvider against the
// real Paystack API (https://paystack.com/docs/api/transaction/).
package paystack

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/Afari-Richmond/payflow/services/payment-service/internal/provider"
)

const defaultBaseURL = "https://api.paystack.co"

// Client calls the real Paystack API. It implements
// provider.PaymentProvider.
type Client struct {
	secretKey  string
	baseURL    string
	httpClient *http.Client
}

// Option configures a Client.
type Option func(*Client)

// WithBaseURL overrides the API base URL. Intended for pointing at a
// local fake server in tests — never used in production, where the
// default (the real Paystack API) always applies.
func WithBaseURL(url string) Option {
	return func(c *Client) { c.baseURL = url }
}

// NewClient builds a Paystack client. secretKey must not be empty —
// callers are expected to fail fast at startup rather than construct
// a client that can never authenticate.
func NewClient(secretKey string, opts ...Option) *Client {
	c := &Client{
		secretKey: secretKey,
		baseURL:   defaultBaseURL,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

type initializeRequestBody struct {
	Email       string `json:"email"`
	Amount      int64  `json:"amount"`
	Currency    string `json:"currency,omitempty"`
	Reference   string `json:"reference,omitempty"`
	CallbackURL string `json:"callback_url,omitempty"`
}

type initializeResponseBody struct {
	Status  bool   `json:"status"`
	Message string `json:"message"`
	Data    struct {
		AuthorizationURL string `json:"authorization_url"`
		AccessCode       string `json:"access_code"`
		Reference        string `json:"reference"`
	} `json:"data"`
}

// InitializeTransaction starts a transaction via Paystack's
// POST /transaction/initialize.
func (c *Client) InitializeTransaction(ctx context.Context, input provider.InitializeTransactionInput) (provider.InitializeTransactionResult, error) {
	reqBody := initializeRequestBody{
		Email:       input.Email,
		Amount:      input.AmountMinor,
		Currency:    input.Currency,
		Reference:   input.Reference,
		CallbackURL: input.CallbackURL,
	}

	var respBody initializeResponseBody
	if err := c.doJSON(ctx, http.MethodPost, "/transaction/initialize", reqBody, &respBody); err != nil {
		return provider.InitializeTransactionResult{}, err
	}
	if !respBody.Status {
		return provider.InitializeTransactionResult{}, fmt.Errorf("paystack: initialize transaction failed: %s", respBody.Message)
	}

	return provider.InitializeTransactionResult{
		AuthorizationURL: respBody.Data.AuthorizationURL,
		AccessCode:       respBody.Data.AccessCode,
		Reference:        respBody.Data.Reference,
	}, nil
}

type verifyResponseBody struct {
	Status  bool   `json:"status"`
	Message string `json:"message"`
	Data    struct {
		Reference string `json:"reference"`
		Status    string `json:"status"`
		Amount    int64  `json:"amount"`
		Currency  string `json:"currency"`
		PaidAt    string `json:"paid_at"`
	} `json:"data"`
}

// VerifyTransaction checks a transaction's status via Paystack's
// GET /transaction/verify/:reference.
func (c *Client) VerifyTransaction(ctx context.Context, reference string) (provider.VerifyTransactionResult, error) {
	var respBody verifyResponseBody
	path := "/transaction/verify/" + reference
	if err := c.doJSON(ctx, http.MethodGet, path, nil, &respBody); err != nil {
		return provider.VerifyTransactionResult{}, err
	}
	if !respBody.Status {
		return provider.VerifyTransactionResult{}, fmt.Errorf("paystack: verify transaction failed: %s", respBody.Message)
	}

	result := provider.VerifyTransactionResult{
		Reference:   respBody.Data.Reference,
		Status:      translateStatus(respBody.Data.Status),
		AmountMinor: respBody.Data.Amount,
		Currency:    respBody.Data.Currency,
	}
	if respBody.Data.PaidAt != "" {
		if t, err := time.Parse(time.RFC3339, respBody.Data.PaidAt); err == nil {
			result.PaidAt = t
		}
	}
	return result, nil
}

// translateStatus maps Paystack's own status vocabulary into PayFlow's
// provider.TransactionStatus — never mirrored blindly.
func translateStatus(paystackStatus string) provider.TransactionStatus {
	switch paystackStatus {
	case "success":
		return provider.TransactionStatusSuccess
	case "abandoned":
		return provider.TransactionStatusAbandoned
	default:
		return provider.TransactionStatusFailed
	}
}

func (c *Client) doJSON(ctx context.Context, method, path string, reqBody, respBody any) error {
	var bodyReader io.Reader
	if reqBody != nil {
		b, err := json.Marshal(reqBody)
		if err != nil {
			return fmt.Errorf("paystack: encode request: %w", err)
		}
		bodyReader = bytes.NewReader(b)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, bodyReader)
	if err != nil {
		return fmt.Errorf("paystack: build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.secretKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("paystack: request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 500 {
		return fmt.Errorf("paystack: server error (status %d)", resp.StatusCode)
	}

	if err := json.NewDecoder(resp.Body).Decode(respBody); err != nil {
		return fmt.Errorf("paystack: decode response: %w", err)
	}

	if resp.StatusCode >= 400 {
		return fmt.Errorf("paystack: request rejected (status %d)", resp.StatusCode)
	}

	return nil
}
