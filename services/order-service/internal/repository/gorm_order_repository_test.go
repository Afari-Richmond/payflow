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

	"github.com/Afari-Richmond/payflow/services/order-service/internal/domain"
	"github.com/Afari-Richmond/payflow/services/order-service/internal/repository"
)

// testDB opens a connection to the real order-service database used
// for local development (docker-compose). Skips the test if it's
// unreachable, so `go test` doesn't hard-fail on a machine without
// Postgres running.
func testDB(t *testing.T) *gorm.DB {
	t.Helper()

	dbURL := os.Getenv("ORDER_DATABASE_URL")
	if dbURL == "" {
		dbURL = "postgres://payflow:payflow@localhost:5433/payflow_order?sslmode=disable"
	}

	db, err := gorm.Open(postgres.Open(dbURL), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Skipf("skipping: order-service database unreachable: %v", err)
	}

	sqlDB, err := db.DB()
	if err != nil || sqlDB.Ping() != nil {
		t.Skip("skipping: order-service database unreachable")
	}

	return db
}

func TestGormOrderRepository_CreateAndGetByID(t *testing.T) {
	db := testDB(t)
	repo := repository.NewGormOrderRepository(db)
	ctx := context.Background()

	order := &domain.Order{
		ID:          uuid.New(),
		Email:       "customer@example.com",
		AmountMinor: 25000,
		Currency:    "GHS",
		Status:      domain.StatusPendingPayment,
		CreatedAt:   time.Now().UTC(),
		UpdatedAt:   time.Now().UTC(),
	}

	t.Cleanup(func() {
		db.Exec("DELETE FROM orders WHERE id = ?", order.ID)
	})

	if err := repo.Create(ctx, order); err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	got, err := repo.GetByID(ctx, order.ID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}

	if got.Email != order.Email {
		t.Errorf("expected email %q, got %q", order.Email, got.Email)
	}
	if got.AmountMinor != order.AmountMinor {
		t.Errorf("expected amount %d, got %d", order.AmountMinor, got.AmountMinor)
	}
	if got.Currency != order.Currency {
		t.Errorf("expected currency %q, got %q", order.Currency, got.Currency)
	}
	if got.Status != order.Status {
		t.Errorf("expected status %q, got %q", order.Status, got.Status)
	}
}

func TestGormOrderRepository_GetByID_NotFound(t *testing.T) {
	db := testDB(t)
	repo := repository.NewGormOrderRepository(db)

	_, err := repo.GetByID(context.Background(), uuid.New())
	if !errors.Is(err, repository.ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

// TestGormOrderRepository_MarkProcessedAndUpdate_ConcurrentCallsOnlyOneWins
// is the real proof behind Milestone 10's idempotency claim on the
// consumer side: N genuinely concurrent database transactions attempt
// to mark the *same* event_id processed (simulating a RabbitMQ
// redelivery landing while the first delivery's processing is still in
// flight). The event_id primary key on processed_events must let
// exactly one succeed.
func TestGormOrderRepository_MarkProcessedAndUpdate_ConcurrentCallsOnlyOneWins(t *testing.T) {
	db := testDB(t)
	repo := repository.NewGormOrderRepository(db)
	ctx := context.Background()

	order := &domain.Order{
		ID:          uuid.New(),
		Email:       "customer@example.com",
		AmountMinor: 25000,
		Currency:    "GHS",
		Status:      domain.StatusPendingPayment,
		CreatedAt:   time.Now().UTC(),
		UpdatedAt:   time.Now().UTC(),
	}
	if err := repo.Create(ctx, order); err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	eventID := uuid.New()
	t.Cleanup(func() {
		db.Exec("DELETE FROM processed_events WHERE event_id = ?", eventID)
		db.Exec("DELETE FROM orders WHERE id = ?", order.ID)
	})

	const concurrency = 10
	var wg sync.WaitGroup
	var firstTimeCount atomic.Int32
	var errCount atomic.Int32

	for range concurrency {
		wg.Go(func() {
			updated := *order
			updated.Status = domain.StatusPaid
			updated.UpdatedAt = time.Now().UTC()

			alreadyProcessed, err := repo.MarkProcessedAndUpdate(ctx, eventID, "payment.succeeded", &updated)
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

	final, err := repo.GetByID(ctx, order.ID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}
	if final.Status != domain.StatusPaid {
		t.Errorf("expected final status %q, got %q", domain.StatusPaid, final.Status)
	}
}
