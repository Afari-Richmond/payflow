package webhook_test

import (
	"context"
	"crypto/hmac"
	"crypto/sha512"
	"encoding/hex"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/Afari-Richmond/payflow/services/payment-service/internal/domain"
	"github.com/Afari-Richmond/payflow/services/payment-service/internal/provider"
	"github.com/Afari-Richmond/payflow/services/payment-service/internal/repository"
	"github.com/Afari-Richmond/payflow/services/payment-service/internal/webhook"
)

const testSecretKey = "sk_test_fake_secret"

func sign(body []byte) string {
	mac := hmac.New(sha512.New, []byte(testSecretKey))
	mac.Write(body)
	return hex.EncodeToString(mac.Sum(nil))
}

type fakeRepo struct {
	payment   *domain.Payment
	getErr    error
	updated   *domain.Payment
	updateErr error
}

func (f *fakeRepo) Create(_ context.Context, _ *domain.Payment) error { return nil }

func (f *fakeRepo) GetByID(_ context.Context, id uuid.UUID) (*domain.Payment, error) {
	if f.getErr != nil {
		return nil, f.getErr
	}
	if f.payment == nil || f.payment.ID != id {
		return nil, repository.ErrNotFound
	}
	cp := *f.payment
	return &cp, nil
}

func (f *fakeRepo) Update(_ context.Context, payment *domain.Payment) error {
	if f.updateErr != nil {
		return f.updateErr
	}
	snapshot := *payment
	f.updated = &snapshot
	if f.payment != nil && f.payment.ID == payment.ID {
		f.payment = &snapshot
	}
	return nil
}

type fakeProvider struct {
	verifyResult provider.VerifyTransactionResult
	verifyErr    error
}

func (f fakeProvider) InitializeTransaction(_ context.Context, _ provider.InitializeTransactionInput) (provider.InitializeTransactionResult, error) {
	return provider.InitializeTransactionResult{}, errors.New("not implemented")
}

func (f fakeProvider) VerifyTransaction(_ context.Context, _ string) (provider.VerifyTransactionResult, error) {
	return f.verifyResult, f.verifyErr
}

func newTestPayment() *domain.Payment {
	now := time.Now().UTC()
	return &domain.Payment{
		ID:                uuid.New(),
		OrderID:           uuid.New(),
		AmountMinor:       25000,
		Currency:          "GHS",
		Status:            domain.StatusInitialized,
		ProviderReference: "",
		CreatedAt:         now,
		UpdatedAt:         now,
	}
}

func chargeSuccessBody(reference string) []byte {
	body, _ := json.Marshal(map[string]any{
		"event": "charge.success",
		"data": map[string]any{
			"reference": reference,
		},
	})
	return body
}

func TestHandleWebhook_ValidSignature_SuccessfulEvent(t *testing.T) {
	payment := newTestPayment()
	payment.ProviderReference = payment.ID.String()
	repo := &fakeRepo{payment: payment}
	prov := fakeProvider{verifyResult: provider.VerifyTransactionResult{
		Status:      provider.TransactionStatusSuccess,
		AmountMinor: payment.AmountMinor,
		Currency:    payment.Currency,
	}}
	svc := webhook.NewService(repo, prov, testSecretKey)

	body := chargeSuccessBody(payment.ID.String())
	err := svc.HandleWebhook(context.Background(), body, sign(body))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if repo.updated == nil {
		t.Fatal("expected the payment to be updated")
	}
	if repo.updated.Status != domain.StatusSuccess {
		t.Errorf("expected status %q, got %q", domain.StatusSuccess, repo.updated.Status)
	}
}

func TestHandleWebhook_InvalidSignature(t *testing.T) {
	repo := &fakeRepo{}
	svc := webhook.NewService(repo, fakeProvider{}, testSecretKey)

	body := chargeSuccessBody(uuid.New().String())
	err := svc.HandleWebhook(context.Background(), body, "not-the-right-signature")

	if !errors.Is(err, webhook.ErrInvalidSignature) {
		t.Errorf("expected ErrInvalidSignature, got %v", err)
	}
	if repo.updated != nil {
		t.Error("expected no state change on invalid signature")
	}
}

func TestHandleWebhook_MalformedPayload(t *testing.T) {
	repo := &fakeRepo{}
	svc := webhook.NewService(repo, fakeProvider{}, testSecretKey)

	body := []byte(`not valid json`)
	err := svc.HandleWebhook(context.Background(), body, sign(body))

	if !errors.Is(err, webhook.ErrMalformedPayload) {
		t.Errorf("expected ErrMalformedPayload, got %v", err)
	}
}

func TestHandleWebhook_UnsupportedEvent(t *testing.T) {
	repo := &fakeRepo{}
	svc := webhook.NewService(repo, fakeProvider{}, testSecretKey)

	body, _ := json.Marshal(map[string]any{
		"event": "transfer.success",
		"data":  map[string]any{"reference": "irrelevant"},
	})
	err := svc.HandleWebhook(context.Background(), body, sign(body))

	if err != nil {
		t.Errorf("expected unsupported events to be safely ignored, got error: %v", err)
	}
	if repo.updated != nil {
		t.Error("expected no state change for an unsupported event")
	}
}

func TestHandleWebhook_VerifiedAsFailed(t *testing.T) {
	payment := newTestPayment()
	repo := &fakeRepo{payment: payment}
	prov := fakeProvider{verifyResult: provider.VerifyTransactionResult{
		Status: provider.TransactionStatusFailed,
	}}
	svc := webhook.NewService(repo, prov, testSecretKey)

	body := chargeSuccessBody(payment.ID.String())
	err := svc.HandleWebhook(context.Background(), body, sign(body))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if repo.updated == nil {
		t.Fatal("expected the payment to be updated")
	}
	if repo.updated.Status != domain.StatusFailed {
		t.Errorf("expected status %q, got %q", domain.StatusFailed, repo.updated.Status)
	}
}

func TestHandleWebhook_DuplicateDelivery_AlreadySuccess(t *testing.T) {
	payment := newTestPayment()
	payment.Status = domain.StatusSuccess
	repo := &fakeRepo{payment: payment}
	prov := fakeProvider{verifyResult: provider.VerifyTransactionResult{Status: provider.TransactionStatusSuccess}}
	svc := webhook.NewService(repo, prov, testSecretKey)

	body := chargeSuccessBody(payment.ID.String())
	err := svc.HandleWebhook(context.Background(), body, sign(body))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if repo.updated != nil {
		t.Error("expected a duplicate delivery for an already-SUCCESS payment to be a no-op")
	}
}

func TestHandleWebhook_UnknownReference_SafelyIgnored(t *testing.T) {
	repo := &fakeRepo{} // no payment stored
	svc := webhook.NewService(repo, fakeProvider{}, testSecretKey)

	body := chargeSuccessBody(uuid.New().String())
	err := svc.HandleWebhook(context.Background(), body, sign(body))

	if err != nil {
		t.Errorf("expected an unknown reference to be safely ignored, got error: %v", err)
	}
}

func TestHandleWebhook_AmountMismatch_Rejected(t *testing.T) {
	payment := newTestPayment()
	repo := &fakeRepo{payment: payment}
	prov := fakeProvider{verifyResult: provider.VerifyTransactionResult{
		Status:      provider.TransactionStatusSuccess,
		AmountMinor: payment.AmountMinor + 1, // mismatch
		Currency:    payment.Currency,
	}}
	svc := webhook.NewService(repo, prov, testSecretKey)

	body := chargeSuccessBody(payment.ID.String())
	err := svc.HandleWebhook(context.Background(), body, sign(body))

	if err == nil {
		t.Fatal("expected an error on amount mismatch between webhook and verified transaction")
	}
	if repo.updated != nil {
		t.Error("expected no state change on amount mismatch")
	}
}
