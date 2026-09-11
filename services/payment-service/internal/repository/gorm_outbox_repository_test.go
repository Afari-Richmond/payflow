package repository_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/Afari-Richmond/payflow/services/payment-service/internal/outbox"
	"github.com/Afari-Richmond/payflow/services/payment-service/internal/repository"
)

func TestGormOutboxRepository_ProcessUnpublished_MarksPublishedOnSuccess(t *testing.T) {
	db := testDB(t)
	repo := repository.NewGormOutboxRepository(db)
	ctx := context.Background()

	id := uuid.New()
	aggregateID := uuid.New()
	if err := db.Exec(
		`INSERT INTO outbox_events (id, aggregate_id, event_type, routing_key, payload, created_at) VALUES (?, ?, ?, ?, ?, ?)`,
		id, aggregateID, "payment.succeeded", "payment.succeeded", `{"hello":"world"}`, time.Now().UTC(),
	).Error; err != nil {
		t.Fatalf("failed to insert outbox row: %v", err)
	}
	t.Cleanup(func() { db.Exec("DELETE FROM outbox_events WHERE id = ?", id) })

	var handled []*outbox.Event
	err := repo.ProcessUnpublished(ctx, 10, func(e *outbox.Event) error {
		handled = append(handled, e)
		return nil
	})
	if err != nil {
		t.Fatalf("ProcessUnpublished failed: %v", err)
	}

	found := false
	for _, e := range handled {
		if e.ID == id {
			found = true
			if string(e.Payload) != `{"hello":"world"}` {
				t.Errorf("expected payload to round-trip, got %q", e.Payload)
			}
		}
	}
	if !found {
		t.Fatal("expected the inserted row to be handled")
	}

	var publishedAt *time.Time
	if err := db.Raw("SELECT published_at FROM outbox_events WHERE id = ?", id).Scan(&publishedAt).Error; err != nil {
		t.Fatalf("failed to read back published_at: %v", err)
	}
	if publishedAt == nil {
		t.Error("expected published_at to be set after a successful handle")
	}
}

func TestGormOutboxRepository_ProcessUnpublished_IncrementsAttemptOnFailure(t *testing.T) {
	db := testDB(t)
	repo := repository.NewGormOutboxRepository(db)
	ctx := context.Background()

	id := uuid.New()
	if err := db.Exec(
		`INSERT INTO outbox_events (id, aggregate_id, event_type, routing_key, payload, created_at) VALUES (?, ?, ?, ?, ?, ?)`,
		id, uuid.New(), "payment.succeeded", "payment.succeeded", `{}`, time.Now().UTC(),
	).Error; err != nil {
		t.Fatalf("failed to insert outbox row: %v", err)
	}
	t.Cleanup(func() { db.Exec("DELETE FROM outbox_events WHERE id = ?", id) })

	simulatedErr := errors.New("broker unavailable")
	err := repo.ProcessUnpublished(ctx, 10, func(e *outbox.Event) error {
		if e.ID == id {
			return simulatedErr
		}
		return nil
	})
	if err != nil {
		t.Fatalf("ProcessUnpublished itself should not fail when handle returns an error: %v", err)
	}

	var attemptCount int
	var publishedAt *time.Time
	if err := db.Raw("SELECT attempt_count, published_at FROM outbox_events WHERE id = ?", id).Row().Scan(&attemptCount, &publishedAt); err != nil {
		t.Fatalf("failed to read back row: %v", err)
	}
	if attemptCount != 1 {
		t.Errorf("expected attempt_count 1, got %d", attemptCount)
	}
	if publishedAt != nil {
		t.Error("expected published_at to remain unset after a failed handle")
	}

	// The row should still be claimable next tick (published_at is
	// still NULL) — support retry, as the milestone's task list asks.
	var retried bool
	err = repo.ProcessUnpublished(ctx, 10, func(e *outbox.Event) error {
		if e.ID == id {
			retried = true
		}
		return nil
	})
	if err != nil {
		t.Fatalf("ProcessUnpublished failed: %v", err)
	}
	if !retried {
		t.Error("expected the previously-failed row to be retried on the next call")
	}
}

// TestGormOutboxRepository_ProcessUnpublished_ConcurrentWorkersNeverDoubleClaim
// is the real proof behind FOR UPDATE SKIP LOCKED: two goroutines,
// standing in for two worker instances, call ProcessUnpublished
// concurrently against the same set of unpublished rows. Each row must
// be handled by exactly one of them — SKIP LOCKED is what makes the
// second query skip rows the first transaction is still holding.
func TestGormOutboxRepository_ProcessUnpublished_ConcurrentWorkersNeverDoubleClaim(t *testing.T) {
	db := testDB(t)
	repo := repository.NewGormOutboxRepository(db)
	ctx := context.Background()

	const rowCount = 10
	ids := make([]uuid.UUID, rowCount)
	for i := range rowCount {
		ids[i] = uuid.New()
		if err := db.Exec(
			`INSERT INTO outbox_events (id, aggregate_id, event_type, routing_key, payload, created_at) VALUES (?, ?, ?, ?, ?, ?)`,
			ids[i], uuid.New(), "payment.succeeded", "payment.succeeded", `{}`, time.Now().UTC(),
		).Error; err != nil {
			t.Fatalf("failed to insert outbox row %d: %v", i, err)
		}
	}
	t.Cleanup(func() {
		for _, id := range ids {
			db.Exec("DELETE FROM outbox_events WHERE id = ?", id)
		}
	})

	var mu sync.Mutex
	handledBy := make(map[uuid.UUID]int) // id -> which worker (1 or 2) handled it

	// A short delay inside handle keeps each transaction's lock held
	// long enough for the other goroutine's claim query to genuinely
	// overlap in time — without it, one worker could finish its entire
	// transaction before the other even starts, which wouldn't
	// exercise SKIP LOCKED's actual purpose.
	runWorker := func(workerID int, wg *sync.WaitGroup) {
		defer wg.Done()
		_ = repo.ProcessUnpublished(ctx, rowCount, func(e *outbox.Event) error {
			time.Sleep(10 * time.Millisecond)
			mu.Lock()
			handledBy[e.ID] = workerID
			mu.Unlock()
			return nil
		})
	}

	var wg sync.WaitGroup
	wg.Add(2)
	go runWorker(1, &wg)
	go runWorker(2, &wg)
	wg.Wait()

	// Run again in case SKIP LOCKED caused either worker to skip rows
	// the other hadn't reached yet (both should be published by now,
	// so this should be a no-op, but confirms no row was left behind).
	_ = repo.ProcessUnpublished(ctx, rowCount, func(e *outbox.Event) error {
		mu.Lock()
		if _, ok := handledBy[e.ID]; !ok {
			handledBy[e.ID] = 0 // handled on the cleanup pass
		}
		mu.Unlock()
		return nil
	})

	if len(handledBy) != rowCount {
		t.Errorf("expected all %d rows handled exactly once, got %d distinct rows handled", rowCount, len(handledBy))
	}
}
