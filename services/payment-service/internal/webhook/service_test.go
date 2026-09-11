package webhook_test

import (
	"context"
	"crypto/hmac"
	"crypto/sha512"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/Afari-Richmond/payflow/pkg/events"
	"github.com/Afari-Richmond/payflow/services/payment-service/internal/domain"
	"github.com/Afari-Richmond/payflow/services/payment-service/internal/provider"
	"github.com/Afari-Richmond/payflow/services/payment-service/internal/repository"
	"github.com/Afari-Richmond/payflow/services/payment-service/internal/webhook"
)

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

type fakePublisher struct {
	mu         sync.Mutex
	published  []events.Envelope
	routingKey []string
	err        error
}

func (f *fakePublisher) Publish(_ context.Context, routingKey string, envelope events.Envelope) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return f.err
	}
	f.published = append(f.published, envelope)
	f.routingKey = append(f.routingKey, routingKey)
	return nil
}

func (f *fakePublisher) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.published)
}

const testSecretKey = "sk_test_fake_secret"

func sign(body []byte) string {
	mac := hmac.New(sha512.New, []byte(testSecretKey))
	mac.Write(body)
	return hex.EncodeToString(mac.Sum(nil))
}

// fakeRepo emulates the real GormPaymentRepository's atomicity
// guarantee — a mutex-guarded map standing in for the database's
// (payment_id, event_type) unique constraint — so concurrency tests
// against this fake exercise the same "only one caller wins" property
// the real unique constraint provides.
type fakeRepo struct {
	mu               sync.Mutex
	payment          *domain.Payment
	getErr           error
	updated          *domain.Payment
	updateErr        error
	markProcessedErr error
	processed        map[string]bool
	markCallCount    int
}

func (f *fakeRepo) Create(_ context.Context, _ *domain.Payment) error { return nil }

func (f *fakeRepo) GetByID(_ context.Context, id uuid.UUID) (*domain.Payment, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
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
	f.mu.Lock()
	defer f.mu.Unlock()
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

func (f *fakeRepo) MarkProcessedAndUpdate(_ context.Context, payment *domain.Payment, eventType string) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.markCallCount++
	if f.markProcessedErr != nil {
		return false, f.markProcessedErr
	}

	if f.processed == nil {
		f.processed = make(map[string]bool)
	}
	key := payment.ID.String() + ":" + eventType
	if f.processed[key] {
		return true, nil // emulates the unique-constraint violation
	}
	f.processed[key] = true

	snapshot := *payment
	f.updated = &snapshot
	if f.payment != nil && f.payment.ID == payment.ID {
		f.payment = &snapshot
	}
	return false, nil
}

type fakeProvider struct {
	verifyResult provider.VerifyTransactionResult
	verifyErr    error
	delay        time.Duration
}

func (f fakeProvider) InitializeTransaction(_ context.Context, _ provider.InitializeTransactionInput) (provider.InitializeTransactionResult, error) {
	return provider.InitializeTransactionResult{}, errors.New("not implemented")
}

func (f fakeProvider) VerifyTransaction(_ context.Context, _ string) (provider.VerifyTransactionResult, error) {
	// A real VerifyTransaction is a network call — real latency, which
	// is exactly what turns "check status, then act" into a race
	// between concurrent deliveries. This fake has near-zero latency
	// otherwise, which would let the fast-path status check alone
	// (accidentally) serialize every call. delay widens the window so
	// the concurrency test actually exercises MarkProcessedAndUpdate's
	// atomicity, not just the optimization in front of it.
	time.Sleep(f.delay)
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
	pub := &fakePublisher{}
	svc := webhook.NewService(repo, prov, pub, testSecretKey, testLogger())

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

	if len(pub.published) != 1 {
		t.Fatalf("expected 1 published event, got %d", len(pub.published))
	}
	if pub.routingKey[0] != events.PaymentSucceeded {
		t.Errorf("expected routing key %q, got %q", events.PaymentSucceeded, pub.routingKey[0])
	}
	var payload events.PaymentSucceededPayload
	if err := json.Unmarshal(pub.published[0].Payload, &payload); err != nil {
		t.Fatalf("failed to decode published payload: %v", err)
	}
	if payload.PaymentID != payment.ID.String() {
		t.Errorf("expected payment id %q in payload, got %q", payment.ID, payload.PaymentID)
	}
}

func TestHandleWebhook_PublishFailure_DoesNotFailWebhook(t *testing.T) {
	// No Outbox yet (Milestone 11) — a publish failure after the DB is
	// already updated is swallowed, not propagated. This is the known
	// dual-write gap, not a bug; documented in ADR 003.
	payment := newTestPayment()
	repo := &fakeRepo{payment: payment}
	prov := fakeProvider{verifyResult: provider.VerifyTransactionResult{
		Status:      provider.TransactionStatusSuccess,
		AmountMinor: payment.AmountMinor,
		Currency:    payment.Currency,
	}}
	pub := &fakePublisher{err: errors.New("broker unavailable")}
	svc := webhook.NewService(repo, prov, pub, testSecretKey, testLogger())

	body := chargeSuccessBody(payment.ID.String())
	err := svc.HandleWebhook(context.Background(), body, sign(body))
	if err != nil {
		t.Fatalf("expected the webhook to still succeed despite the publish failure, got: %v", err)
	}
	if repo.updated == nil || repo.updated.Status != domain.StatusSuccess {
		t.Error("expected the payment's DB state to still be updated to SUCCESS")
	}
}

func TestHandleWebhook_InvalidSignature(t *testing.T) {
	repo := &fakeRepo{}
	svc := webhook.NewService(repo, fakeProvider{}, &fakePublisher{}, testSecretKey, testLogger())

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
	svc := webhook.NewService(repo, fakeProvider{}, &fakePublisher{}, testSecretKey, testLogger())

	body := []byte(`not valid json`)
	err := svc.HandleWebhook(context.Background(), body, sign(body))

	if !errors.Is(err, webhook.ErrMalformedPayload) {
		t.Errorf("expected ErrMalformedPayload, got %v", err)
	}
}

func TestHandleWebhook_UnsupportedEvent(t *testing.T) {
	repo := &fakeRepo{}
	svc := webhook.NewService(repo, fakeProvider{}, &fakePublisher{}, testSecretKey, testLogger())

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
	svc := webhook.NewService(repo, prov, &fakePublisher{}, testSecretKey, testLogger())

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
	svc := webhook.NewService(repo, prov, &fakePublisher{}, testSecretKey, testLogger())

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
	svc := webhook.NewService(repo, fakeProvider{}, &fakePublisher{}, testSecretKey, testLogger())

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
	svc := webhook.NewService(repo, prov, &fakePublisher{}, testSecretKey, testLogger())

	body := chargeSuccessBody(payment.ID.String())
	err := svc.HandleWebhook(context.Background(), body, sign(body))

	if err == nil {
		t.Fatal("expected an error on amount mismatch between webhook and verified transaction")
	}
	if repo.updated != nil {
		t.Error("expected no state change on amount mismatch")
	}
}

// TestHandleWebhook_ConcurrentDuplicateDeliveries is the concurrency
// test the milestone's own task list asks for: N goroutines deliver
// the *same* webhook simultaneously (simulating Paystack redelivering
// while the first attempt is still in flight — the exact race the
// Milestone 8 status-check-only guard could not close). Exactly one
// must "win" — the point of MarkProcessedAndUpdate's atomic marker
// insert, not the earlier fast-path status check.
func TestHandleWebhook_ConcurrentDuplicateDeliveries(t *testing.T) {
	payment := newTestPayment()
	repo := &fakeRepo{payment: payment}
	prov := fakeProvider{
		verifyResult: provider.VerifyTransactionResult{
			Status:      provider.TransactionStatusSuccess,
			AmountMinor: payment.AmountMinor,
			Currency:    payment.Currency,
		},
		delay: 20 * time.Millisecond,
	}
	pub := &fakePublisher{}
	svc := webhook.NewService(repo, prov, pub, testSecretKey, testLogger())

	body := chargeSuccessBody(payment.ID.String())
	signature := sign(body)

	const concurrency = 20
	var wg sync.WaitGroup
	errs := make([]error, concurrency)
	for i := range concurrency {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			errs[i] = svc.HandleWebhook(context.Background(), body, signature)
		}(i)
	}
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Errorf("call %d: unexpected error: %v", i, err)
		}
	}

	repo.mu.Lock()
	markCalls := repo.markCallCount
	repo.mu.Unlock()
	if markCalls != concurrency {
		t.Errorf("expected MarkProcessedAndUpdate called %d times (once per delivery), got %d", concurrency, markCalls)
	}

	if got := pub.count(); got != 1 {
		t.Errorf("expected exactly 1 published event despite %d concurrent deliveries, got %d", concurrency, got)
	}
}
