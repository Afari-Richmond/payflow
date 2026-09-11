package outbox

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"github.com/Afari-Richmond/payflow/pkg/events"
)

// EventPublisher is the messaging dependency the worker needs. A
// narrow interface (not the concrete *messaging.Publisher) so this
// package can be tested without a real broker.
type EventPublisher interface {
	Publish(ctx context.Context, routingKey string, envelope events.Envelope) error
}

// Worker polls Repository for unpublished events and publishes them.
// Retries are unlimited and un-delayed — acceptable for this project's
// scale and the point being taught (durability, not backoff policy);
// a production system would eventually cap attempts and alert. See
// ADR 005.
type Worker struct {
	repo      Repository
	publisher EventPublisher
	logger    *slog.Logger
	interval  time.Duration
	batchSize int
}

// NewWorker builds a Worker with sensible defaults (2s poll interval,
// 20-event batches).
func NewWorker(repo Repository, publisher EventPublisher, logger *slog.Logger) *Worker {
	return &Worker{
		repo:      repo,
		publisher: publisher,
		logger:    logger,
		interval:  2 * time.Second,
		batchSize: 20,
	}
}

// Run polls until ctx is cancelled.
func (w *Worker) Run(ctx context.Context) {
	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			w.tick(ctx)
		}
	}
}

func (w *Worker) tick(ctx context.Context) {
	err := w.repo.ProcessUnpublished(ctx, w.batchSize, func(e *Event) error {
		var envelope events.Envelope
		if err := json.Unmarshal(e.Payload, &envelope); err != nil {
			// Will never succeed on retry, but the outbox has no
			// dead-letter concept of its own (unlike the RabbitMQ
			// consumer side) — log loudly and mark it published anyway
			// so a malformed row doesn't retry forever. This should
			// never actually happen in practice: the outbox row's
			// payload is written by this same service, from the same
			// envelope-encoding path, in the same transaction as the
			// row itself.
			w.logger.Error("outbox: stored envelope is malformed, discarding",
				"outbox_id", e.ID, "aggregate_id", e.AggregateID, "error", err)
			return nil
		}

		if err := w.publisher.Publish(ctx, e.RoutingKey, envelope); err != nil {
			w.logger.Warn("outbox: publish failed, will retry next tick",
				"outbox_id", e.ID, "event_type", e.EventType, "attempt", e.AttemptCount+1, "error", err)
			return err
		}

		w.logger.Info("outbox: published", "outbox_id", e.ID, "event_type", e.EventType)
		return nil
	})
	if err != nil {
		w.logger.Error("outbox: batch processing failed", "error", err)
	}
}
