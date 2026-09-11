package repository_test

import (
	"context"
	"errors"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/Afari-Richmond/payflow/pkg/events"
	"github.com/Afari-Richmond/payflow/services/payment-service/internal/domain"
	"github.com/Afari-Richmond/payflow/services/payment-service/internal/outbox"
	"github.com/Afari-Richmond/payflow/services/payment-service/internal/repository"
)

// testDB opens a connection to the real payment-service database used
// for local development (docker-compose). Skips the test if it's
// unreachable, so `go test` doesn't hard-fail on a machine without
// Postgres running.
func testDB(t *testing.T) *gorm.DB {
	t.Helper()

	dbURL := os.Getenv("PAYMENT_DATABASE_URL")
	if dbURL == "" {
		dbURL = "postgres://payflow:payflow@localhost:5433/payflow_payment?sslmode=disable"
	}

	db, err := gorm.Open(postgres.Open(dbURL), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Skipf("skipping: payment-service database unreachable: %v", err)
	}

	sqlDB, err := db.DB()
	if err != nil || sqlDB.Ping() != nil {
		t.Skip("skipping: payment-service database unreachable")
	}

	return db
}

func TestGormPaymentRepository_CreateAndGetByID(t *testing.T) {
	db := testDB(t)
	repo := repository.NewGormPaymentRepository(db)
	ctx := context.Background()

	payment := &domain.Payment{
		ID:          uuid.New(),
		OrderID:     uuid.New(),
		AmountMinor: 25000,
		Currency:    "GHS",
		Status:      domain.StatusPending,
		CreatedAt:   time.Now().UTC(),
		UpdatedAt:   time.Now().UTC(),
	}

	t.Cleanup(func() {
		db.Exec("DELETE FROM payments WHERE id = ?", payment.ID)
	})

	if err := repo.Create(ctx, payment); err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	got, err := repo.GetByID(ctx, payment.ID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}

	if got.OrderID != payment.OrderID {
		t.Errorf("expected order id %v, got %v", payment.OrderID, got.OrderID)
	}
	if got.AmountMinor != payment.AmountMinor {
		t.Errorf("expected amount %d, got %d", payment.AmountMinor, got.AmountMinor)
	}
	if got.Currency != payment.Currency {
		t.Errorf("expected currency %q, got %q", payment.Currency, got.Currency)
	}
	if got.Status != payment.Status {
		t.Errorf("expected status %q, got %q", payment.Status, got.Status)
	}
}

func TestGormPaymentRepository_GetByID_NotFound(t *testing.T) {
	db := testDB(t)
	repo := repository.NewGormPaymentRepository(db)

	_, err := repo.GetByID(context.Background(), uuid.New())
	if !errors.Is(err, repository.ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

// TestGormPaymentRepository_MarkProcessedAndUpdate_ConcurrentCallsOnlyOneWins
// is the real proof behind ADR 003 / Milestone 10's idempotency claim:
// N genuinely concurrent database transactions attempt to mark the
// *same* (payment_id, event_type) processed. The composite primary key
// on processed_webhook_events must let exactly one succeed — not an
// application-level race that "usually" works, an actual constraint
// the database enforces.
func TestGormPaymentRepository_MarkProcessedAndUpdate_ConcurrentCallsOnlyOneWins(t *testing.T) {
	db := testDB(t)
	repo := repository.NewGormPaymentRepository(db)
	ctx := context.Background()

	payment := &domain.Payment{
		ID:          uuid.New(),
		OrderID:     uuid.New(),
		AmountMinor: 25000,
		Currency:    "GHS",
		Status:      domain.StatusInitialized,
		CreatedAt:   time.Now().UTC(),
		UpdatedAt:   time.Now().UTC(),
	}
	if err := repo.Create(ctx, payment); err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	t.Cleanup(func() {
		db.Exec("DELETE FROM outbox_events WHERE aggregate_id = ?", payment.ID)
		db.Exec("DELETE FROM processed_webhook_events WHERE payment_id = ?", payment.ID)
		db.Exec("DELETE FROM payments WHERE id = ?", payment.ID)
	})

	const concurrency = 10
	var wg sync.WaitGroup
	var firstTimeCount atomic.Int32
	var errCount atomic.Int32

	for range concurrency {
		wg.Go(func() {
			updated := *payment
			updated.Status = domain.StatusSuccess
			updated.UpdatedAt = time.Now().UTC()

			outboxEvent := &outbox.Event{
				ID:          uuid.New(),
				AggregateID: payment.ID,
				EventType:   events.PaymentSucceeded,
				RoutingKey:  events.PaymentSucceeded,
				Payload:     []byte(`{}`),
				CreatedAt:   time.Now().UTC(),
			}

			alreadyProcessed, err := repo.MarkProcessedAndUpdate(ctx, &updated, "charge.success", outboxEvent)
			if err != nil {
				errCount.Add(1)
				return
			}
			if !alreadyProcessed {
				firstTimeCount.Add(1)
			}
		})
	}
	wg.Wait()

	if got := errCount.Load(); got != 0 {
		t.Errorf("expected 0 unexpected errors, got %d", got)
	}
	if got := firstTimeCount.Load(); got != 1 {
		t.Errorf("expected exactly 1 of %d concurrent calls to win (alreadyProcessed=false), got %d", concurrency, got)
	}

	final, err := repo.GetByID(ctx, payment.ID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}
	if final.Status != domain.StatusSuccess {
		t.Errorf("expected final status %q, got %q", domain.StatusSuccess, final.Status)
	}

	var outboxCount int64
	if err := db.Table("outbox_events").Where("aggregate_id = ?", payment.ID).Count(&outboxCount).Error; err != nil {
		t.Fatalf("failed to count outbox rows: %v", err)
	}
	if outboxCount != 1 {
		t.Errorf("expected exactly 1 outbox row despite %d concurrent calls, got %d", concurrency, outboxCount)
	}
}
