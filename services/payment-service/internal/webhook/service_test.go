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
	"github.com/Afari-Richmond/payflow/services/payment-service/internal/outbox"
	"github.com/Afari-Richmond/payflow/services/payment-service/internal/provider"
	"github.com/Afari-Richmond/payflow/services/payment-service/internal/repository"
	"github.com/Afari-Richmond/payflow/services/payment-service/internal/webhook"
)

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
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
// the real unique constraint provides. It also records outbox events
// exactly as MarkProcessedAndUpdate would durably persist them, so
// tests can assert on what would have been enqueued.
type fakeRepo struct {
	mu               sync.Mutex
	payment          *domain.Payment
	getErr           error
	updated          *domain.Payment
	updateErr        error
	markProcessedErr error
	processed        map[string]bool
	markCallCount    int
	enqueued         []*outbox.Event
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

func (f *fakeRepo) MarkProcessedAndUpdate(_ context.Context, payment *domain.Payment, eventType string, outboxEvent *outbox.Event) (bool, error) {
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
	if outboxEvent != nil {
		f.enqueued = append(f.enqueued, outboxEvent)
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
	svc := webhook.NewService(repo, prov, testSecretKey, testLogger())

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

	if len(repo.enqueued) != 1 {
		t.Fatalf("expected 1 outbox event enqueued, got %d", len(repo.enqueued))
	}
	enqueued := repo.enqueued[0]
	if enqueued.RoutingKey != events.PaymentSucceeded {
		t.Errorf("expected routing key %q, got %q", events.PaymentSucceeded, enqueued.RoutingKey)
	}
	if enqueued.AggregateID != payment.ID {
		t.Errorf("expected aggregate id %v, got %v", payment.ID, enqueued.AggregateID)
	}

	var envelope events.Envelope
	if err := json.Unmarshal(enqueued.Payload, &envelope); err != nil {
		t.Fatalf("failed to decode enqueued envelope: %v", err)
	}
	var payload events.PaymentSucceededPayload
	if err := json.Unmarshal(envelope.Payload, &payload); err != nil {
		t.Fatalf("failed to decode enqueued payload: %v", err)
	}
	if payload.PaymentID != payment.ID.String() {
		t.Errorf("expected payment id %q in payload, got %q", payment.ID, payload.PaymentID)
	}
}

func TestHandleWebhook_RepositoryFailure_OutboxWriteNeverSucceedsPartially(t *testing.T) {
	// Proves the point of writing payment + outbox in one transaction:
	// if MarkProcessedAndUpdate fails, the caller sees a single error —
	// there's no way to observe "payment updated but outbox row
	// missing" or vice versa, because the fake (like the real
	// implementation) only ever returns success after both succeed
	// together.
	payment := newTestPayment()
	repo := &fakeRepo{payment: payment, markProcessedErr: errors.New("db connection lost")}
	prov := fakeProvider{verifyResult: provider.VerifyTransactionResult{
		Status:      provider.TransactionStatusSuccess,
		AmountMinor: payment.AmountMinor,
		Currency:    payment.Currency,
	}}
	svc := webhook.NewService(repo, prov, testSecretKey, testLogger())

	body := chargeSuccessBody(payment.ID.String())
	err := svc.HandleWebhook(context.Background(), body, sign(body))
	if err == nil {
		t.Fatal("expected an error when the repository transaction fails")
	}
	if repo.updated != nil {
		t.Error("expected no state change when the transaction fails")
	}
	if len(repo.enqueued) != 0 {
		t.Error("expected no outbox event enqueued when the transaction fails")
	}
}

func TestHandleWebhook_InvalidSignature(t *testing.T) {
	repo := &fakeRepo{}
	svc := webhook.NewService(repo, fakeProvider{}, testSecretKey, testLogger())

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
	svc := webhook.NewService(repo, fakeProvider{}, testSecretKey, testLogger())

	body := []byte(`not valid json`)
	err := svc.HandleWebhook(context.Background(), body, sign(body))

	if !errors.Is(err, webhook.ErrMalformedPayload) {
		t.Errorf("expected ErrMalformedPayload, got %v", err)
	}
}

func TestHandleWebhook_UnsupportedEvent(t *testing.T) {
	repo := &fakeRepo{}
	svc := webhook.NewService(repo, fakeProvider{}, testSecretKey, testLogger())

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
	svc := webhook.NewService(repo, prov, testSecretKey, testLogger())

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
	if len(repo.enqueued) != 1 || repo.enqueued[0].RoutingKey != events.PaymentFailed {
		t.Errorf("expected 1 payment.failed outbox event, got %+v", repo.enqueued)
	}
}

func TestHandleWebhook_DuplicateDelivery_AlreadySuccess(t *testing.T) {
	payment := newTestPayment()
	payment.Status = domain.StatusSuccess
	repo := &fakeRepo{payment: payment}
	prov := fakeProvider{verifyResult: provider.VerifyTransactionResult{Status: provider.TransactionStatusSuccess}}
	svc := webhook.NewService(repo, prov, testSecretKey, testLogger())

	body := chargeSuccessBody(payment.ID.String())
	err := svc.HandleWebhook(context.Background(), body, sign(body))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if repo.updated != nil {
		t.Error("expected a duplicate delivery for an already-SUCCESS payment to be a no-op")
	}
	if len(repo.enqueued) != 0 {
		t.Error("expected no outbox event enqueued for a duplicate delivery")
	}
}

func TestHandleWebhook_UnknownReference_SafelyIgnored(t *testing.T) {
	repo := &fakeRepo{} // no payment stored
	svc := webhook.NewService(repo, fakeProvider{}, testSecretKey, testLogger())

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
	svc := webhook.NewService(repo, prov, testSecretKey, testLogger())

	body := chargeSuccessBody(payment.ID.String())
	err := svc.HandleWebhook(context.Background(), body, sign(body))

	if err == nil {
		t.Fatal("expected an error on amount mismatch between webhook and verified transaction")
	}
	if repo.updated != nil {
		t.Error("expected no state change on amount mismatch")
	}
	if len(repo.enqueued) != 0 {
		t.Error("expected no outbox event enqueued on amount mismatch")
	}
}

// TestHandleWebhook_ConcurrentDuplicateDeliveries is the concurrency
// test Milestone 10's task list asked for, still valid here: N
// goroutines deliver the *same* webhook simultaneously. Exactly one
// must "win" — the point of MarkProcessedAndUpdate's atomic marker
// insert, not the earlier fast-path status check. Now also asserts
// exactly one outbox event is enqueued, not just one DB update.
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
	svc := webhook.NewService(repo, prov, testSecretKey, testLogger())

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
	enqueuedCount := len(repo.enqueued)
	repo.mu.Unlock()

	if markCalls != concurrency {
		t.Errorf("expected MarkProcessedAndUpdate called %d times (once per delivery), got %d", concurrency, markCalls)
	}
	if enqueuedCount != 1 {
		t.Errorf("expected exactly 1 outbox event enqueued despite %d concurrent deliveries, got %d", concurrency, enqueuedCount)
	}
}
