package repository

import (
	"context"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/Afari-Richmond/payflow/services/payment-service/internal/outbox"
)

// GormOutboxRepository implements outbox.Repository — the read/claim
// side of the outbox, used by outbox.Worker. Shares the outbox_events
// table with GormPaymentRepository's write side, but is a distinct
// type since it's used by a different component with a different
// responsibility (publishing, not payment processing).
type GormOutboxRepository struct {
	db *gorm.DB
}

// NewGormOutboxRepository builds a GormOutboxRepository.
func NewGormOutboxRepository(db *gorm.DB) *GormOutboxRepository {
	return &GormOutboxRepository{db: db}
}

// ProcessUnpublished claims up to limit unpublished rows via
// SELECT ... FOR UPDATE SKIP LOCKED — if multiple worker instances
// ever run concurrently, each claims a disjoint set of rows, never the
// same one twice. The transaction stays open for the duration of
// handle (including the network call to the broker), which is what
// makes the claim meaningful: another worker's SKIP LOCKED query won't
// see these rows until this transaction commits or rolls back.
func (r *GormOutboxRepository) ProcessUnpublished(ctx context.Context, limit int, handle func(*outbox.Event) error) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var models []outboxEventModel
		err := tx.Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"}).
			Where("published_at IS NULL").
			Order("created_at ASC").
			Limit(limit).
			Find(&models).Error
		if err != nil {
			return err
		}

		for _, m := range models {
			event := &outbox.Event{
				ID:           m.ID,
				AggregateID:  m.AggregateID,
				EventType:    m.EventType,
				RoutingKey:   m.RoutingKey,
				Payload:      []byte(m.Payload),
				CreatedAt:    m.CreatedAt,
				PublishedAt:  m.PublishedAt,
				AttemptCount: m.AttemptCount,
			}

			if handleErr := handle(event); handleErr != nil {
				if err := tx.Model(&outboxEventModel{}).Where("id = ?", m.ID).
					Update("attempt_count", m.AttemptCount+1).Error; err != nil {
					return err
				}
				continue
			}

			now := time.Now().UTC()
			if err := tx.Model(&outboxEventModel{}).Where("id = ?", m.ID).
				Update("published_at", now).Error; err != nil {
				return err
			}
		}

		return nil
	})
}
