package application_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/Afari-Richmond/payflow/pkg/events"
	"github.com/Afari-Richmond/payflow/services/order-service/internal/application"
	"github.com/Afari-Richmond/payflow/services/order-service/internal/domain"
	"github.com/Afari-Richmond/payflow/services/order-service/internal/repository"
)

// fakeOrderRepository emulates the real GormOrderRepository's
// atomicity guarantee — a mutex-guarded set standing in for the
// database's event_id unique constraint — so concurrency tests against
// this fake exercise the same "only one caller wins" property the real
// unique constraint provides.
type fakeOrderRepository struct {
	mu               sync.Mutex
	order            *domain.Order
	created          *domain.Order
	getErr           error
	err              error
	updated          *domain.Order
	markProcessedErr error
	processedEventID map[uuid.UUID]bool
	markCallCount    int
	firstTimeCount   int
	// getDelay simulates real network/DB latency between the read and
	// the write, widening the window a concurrency test needs to
	// actually exercise — a real GetByID has this latency for free;
	// this fake doesn't, without it. Slept outside the lock so
	// concurrent callers overlap instead of serializing on it.
	getDelay time.Duration
}

func (f *fakeOrderRepository) Create(_ context.Context, order *domain.Order) error {
	if f.err != nil {
		return f.err
	}
	f.created = order
	return nil
}

func (f *fakeOrderRepository) GetByID(_ context.Context, id uuid.UUID) (*domain.Order, error) {
	if f.getDelay > 0 {
		time.Sleep(f.getDelay)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.getErr != nil {
		return nil, f.getErr
	}
	if f.order == nil || f.order.ID != id {
		return nil, repository.ErrNotFound
	}
	cp := *f.order
	return &cp, nil
}

func (f *fakeOrderRepository) Update(_ context.Context, order *domain.Order) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	snapshot := *order
	f.updated = &snapshot
	if f.order != nil && f.order.ID == order.ID {
		f.order = &snapshot
	}
	return nil
}

func (f *fakeOrderRepository) MarkProcessedAndUpdate(_ context.Context, eventID uuid.UUID, _ string, order *domain.Order) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.markCallCount++
	if f.markProcessedErr != nil {
		return false, f.markProcessedErr
	}

	if f.processedEventID == nil {
		f.processedEventID = make(map[uuid.UUID]bool)
	}
	if f.processedEventID[eventID] {
		return true, nil // emulates the unique-constraint violation
	}
	f.processedEventID[eventID] = true
	f.firstTimeCount++

	snapshot := *order
	f.updated = &snapshot
	if f.order != nil && f.order.ID == order.ID {
		f.order = &snapshot
	}
	return false, nil
}

func TestOrderService_CreateOrder_Success(t *testing.T) {
	repo := &fakeOrderRepository{}
	svc := application.NewOrderService(repo)

	order, err := svc.CreateOrder(context.Background(), "customer@example.com", 25000, "GHS")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if order.ID == uuid.Nil {
		t.Error("expected a server-generated ID, got nil UUID")
	}
	if order.Status != domain.StatusPendingPayment {
		t.Errorf("expected status %q, got %q", domain.StatusPendingPayment, order.Status)
	}
	if repo.created == nil {
		t.Fatal("expected repository.Create to be called")
	}
}

func TestOrderService_CreateOrder_Validation(t *testing.T) {
	tests := []struct {
		name        string
		email       string
		amountMinor int64
		currency    string
		wantErr     error
	}{
		{"invalid email", "not-an-email", 25000, "GHS", application.ErrInvalidEmail},
		{"empty email", "", 25000, "GHS", application.ErrInvalidEmail},
		{"zero amount", "customer@example.com", 0, "GHS", application.ErrInvalidAmount},
		{"negative amount", "customer@example.com", -100, "GHS", application.ErrInvalidAmount},
		{"unsupported currency", "customer@example.com", 25000, "XYZ", application.ErrUnsupportedCurrency},
		{"empty currency", "customer@example.com", 25000, "", application.ErrUnsupportedCurrency},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &fakeOrderRepository{}
			svc := application.NewOrderService(repo)

			_, err := svc.CreateOrder(context.Background(), tt.email, tt.amountMinor, tt.currency)
			if !errors.Is(err, tt.wantErr) {
				t.Errorf("expected error %v, got %v", tt.wantErr, err)
			}
			if repo.created != nil {
				t.Error("expected repository.Create not to be called on validation failure")
			}
		})
	}
}

func TestOrderService_CreateOrder_RepositoryError(t *testing.T) {
	repo := &fakeOrderRepository{err: errors.New("connection lost")}
	svc := application.NewOrderService(repo)

	_, err := svc.CreateOrder(context.Background(), "customer@example.com", 25000, "GHS")
	if err == nil {
		t.Fatal("expected an error when the repository fails")
	}
}

func newTestOrder() *domain.Order {
	now := time.Now().UTC()
	return &domain.Order{
		ID:          uuid.New(),
		Email:       "customer@example.com",
		AmountMinor: 25000,
		Currency:    "GHS",
		Status:      domain.StatusPendingPayment,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
}

func paymentSucceededEnvelope(t *testing.T, orderID string) events.Envelope {
	t.Helper()
	envelope, err := events.NewEnvelope(context.Background(), events.PaymentSucceeded, "payment-1", events.PaymentSucceededPayload{
		PaymentID:   "payment-1",
		OrderID:     orderID,
		AmountMinor: 25000,
		Currency:    "GHS",
	})
	if err != nil {
		t.Fatalf("failed to build envelope: %v", err)
	}
	return envelope
}

func TestHandlePaymentEvent_PaymentSucceeded_MarksOrderPaid(t *testing.T) {
	order := newTestOrder()
	repo := &fakeOrderRepository{order: order}
	svc := application.NewOrderService(repo)

	err := svc.HandlePaymentEvent(context.Background(), paymentSucceededEnvelope(t, order.ID.String()))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if repo.updated == nil {
		t.Fatal("expected the order to be updated")
	}
	if repo.updated.Status != domain.StatusPaid {
		t.Errorf("expected status %q, got %q", domain.StatusPaid, repo.updated.Status)
	}
}

func TestHandlePaymentEvent_PaymentSucceeded_DuplicateDelivery_AlreadyPaid(t *testing.T) {
	order := newTestOrder()
	order.Status = domain.StatusPaid
	repo := &fakeOrderRepository{order: order}
	svc := application.NewOrderService(repo)

	err := svc.HandlePaymentEvent(context.Background(), paymentSucceededEnvelope(t, order.ID.String()))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if repo.updated != nil {
		t.Error("expected a duplicate delivery for an already-PAID order to be a no-op")
	}
}

func TestHandlePaymentEvent_PaymentSucceeded_UnknownOrder_PermanentError(t *testing.T) {
	repo := &fakeOrderRepository{} // no order stored
	svc := application.NewOrderService(repo)

	err := svc.HandlePaymentEvent(context.Background(), paymentSucceededEnvelope(t, uuid.New().String()))

	if _, ok := errors.AsType[*events.PermanentError](err); !ok {
		t.Errorf("expected a *events.PermanentError, got %v (%T)", err, err)
	}
}

func TestHandlePaymentEvent_PaymentSucceeded_MalformedPayload_PermanentError(t *testing.T) {
	repo := &fakeOrderRepository{}
	svc := application.NewOrderService(repo)

	envelope := events.Envelope{EventType: events.PaymentSucceeded, Payload: []byte(`not json`)}
	err := svc.HandlePaymentEvent(context.Background(), envelope)

	if _, ok := errors.AsType[*events.PermanentError](err); !ok {
		t.Errorf("expected a *events.PermanentError, got %v (%T)", err, err)
	}
}

func TestHandlePaymentEvent_PaymentSucceeded_RepositoryError_Transient(t *testing.T) {
	order := newTestOrder()
	repo := &fakeOrderRepository{order: order, getErr: errors.New("connection lost")}
	svc := application.NewOrderService(repo)

	err := svc.HandlePaymentEvent(context.Background(), paymentSucceededEnvelope(t, order.ID.String()))

	if _, ok := errors.AsType[*events.PermanentError](err); ok {
		t.Error("expected a transient (non-permanent) error for a DB failure, got a PermanentError")
	}
	if err == nil {
		t.Error("expected an error when the repository fails")
	}
}

func TestHandlePaymentEvent_PaymentFailed_NoOrderStateChange(t *testing.T) {
	order := newTestOrder()
	repo := &fakeOrderRepository{order: order}
	svc := application.NewOrderService(repo)

	envelope, err := events.NewEnvelope(context.Background(), events.PaymentFailed, "payment-1", events.PaymentFailedPayload{
		PaymentID: "payment-1",
		OrderID:   order.ID.String(),
	})
	if err != nil {
		t.Fatalf("failed to build envelope: %v", err)
	}

	if err := svc.HandlePaymentEvent(context.Background(), envelope); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if repo.updated != nil {
		t.Error("expected no order state change for a payment.failed event")
	}
}

func TestHandlePaymentEvent_UnsupportedEventType_SafelyIgnored(t *testing.T) {
	repo := &fakeOrderRepository{}
	svc := application.NewOrderService(repo)

	envelope := events.Envelope{EventType: "transfer.success"}
	if err := svc.HandlePaymentEvent(context.Background(), envelope); err != nil {
		t.Errorf("expected an unsupported event type to be safely ignored, got: %v", err)
	}
}

// TestHandlePaymentEvent_ConcurrentRedeliveries is the concurrency test
// the milestone's own task list asks for: N goroutines process the
// *same* event (same event_id, as a genuine RabbitMQ redelivery would
// be) simultaneously. Exactly one must actually apply the update.
//
// Note on markCallCount: it is deliberately *not* asserted to equal
// concurrency. The fast-path status check (cheap, no transaction) and
// the atomic MarkProcessedAndUpdate call (the real guarantee) are two
// layers of the same defense — once the fastest goroutine finishes its
// (near-instant) write, later-waking goroutines legitimately catch the
// fast path and never reach MarkProcessedAndUpdate at all. That's
// working as designed, not a gap in the test.
func TestHandlePaymentEvent_ConcurrentRedeliveries(t *testing.T) {
	order := newTestOrder()
	repo := &fakeOrderRepository{order: order, getDelay: 20 * time.Millisecond}
	svc := application.NewOrderService(repo)

	envelope := paymentSucceededEnvelope(t, order.ID.String())

	const concurrency = 20
	var wg sync.WaitGroup
	errs := make([]error, concurrency)
	for i := range concurrency {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			errs[i] = svc.HandlePaymentEvent(context.Background(), envelope)
		}(i)
	}
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Errorf("call %d: unexpected error: %v", i, err)
		}
	}

	repo.mu.Lock()
	wins := repo.firstTimeCount
	finalStatus := repo.order.Status
	repo.mu.Unlock()

	if wins != 1 {
		t.Errorf("expected exactly 1 of %d concurrent redeliveries to actually apply the update, got %d", concurrency, wins)
	}
	if finalStatus != domain.StatusPaid {
		t.Errorf("expected final status %q, got %q", domain.StatusPaid, finalStatus)
	}
}
