package repository

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"

	"github.com/Afari-Richmond/payflow/services/payment-service/internal/domain"
)

// postgresUniqueViolation is Postgres's SQLSTATE code for a unique
// constraint violation.
const postgresUniqueViolation = "23505"

// isUniqueViolation reports whether err is a Postgres unique
// constraint violation — used to detect "this event was already
// processed" via the processed_webhook_events table's primary key,
// rather than an application-level check-then-write race.
func isUniqueViolation(err error) bool {
	pgErr, ok := errors.AsType[*pgconn.PgError](err)
	return ok && pgErr.Code == postgresUniqueViolation
}

// paymentModel is the GORM-mapped persistence shape for a payment.
// Kept separate from domain.Payment so the domain package stays free
// of persistence-framework tags.
type paymentModel struct {
	ID                uuid.UUID `gorm:"column:id;primaryKey"`
	OrderID           uuid.UUID `gorm:"column:order_id"`
	AmountMinor       int64     `gorm:"column:amount_minor"`
	Currency          string    `gorm:"column:currency"`
	Status            string    `gorm:"column:status"`
	ProviderReference string    `gorm:"column:provider_reference"`
	CreatedAt         time.Time `gorm:"column:created_at"`
	UpdatedAt         time.Time `gorm:"column:updated_at"`
}

func (paymentModel) TableName() string { return "payments" }

// processedWebhookEventModel backs the idempotency guard — its
// composite primary key (payment_id, event_type) is what actually
// prevents double-processing under concurrent webhook deliveries; see
// isUniqueViolation and MarkProcessedAndUpdate.
type processedWebhookEventModel struct {
	PaymentID   uuid.UUID `gorm:"column:payment_id;primaryKey"`
	EventType   string    `gorm:"column:event_type;primaryKey"`
	ProcessedAt time.Time `gorm:"column:processed_at"`
}

func (processedWebhookEventModel) TableName() string { return "processed_webhook_events" }

func toModel(p *domain.Payment) paymentModel {
	return paymentModel{
		ID:                p.ID,
		OrderID:           p.OrderID,
		AmountMinor:       p.AmountMinor,
		Currency:          p.Currency,
		Status:            string(p.Status),
		ProviderReference: p.ProviderReference,
		CreatedAt:         p.CreatedAt,
		UpdatedAt:         p.UpdatedAt,
	}
}

func toDomain(m paymentModel) *domain.Payment {
	return &domain.Payment{
		ID:                m.ID,
		OrderID:           m.OrderID,
		AmountMinor:       m.AmountMinor,
		Currency:          m.Currency,
		Status:            domain.Status(m.Status),
		ProviderReference: m.ProviderReference,
		CreatedAt:         m.CreatedAt,
		UpdatedAt:         m.UpdatedAt,
	}
}

// GormPaymentRepository is a PostgreSQL-backed PaymentRepository using
// GORM.
type GormPaymentRepository struct {
	db *gorm.DB
}

// NewGormPaymentRepository builds a GormPaymentRepository.
func NewGormPaymentRepository(db *gorm.DB) *GormPaymentRepository {
	return &GormPaymentRepository{db: db}
}

// Create inserts a new payment.
func (r *GormPaymentRepository) Create(ctx context.Context, payment *domain.Payment) error {
	model := toModel(payment)
	return r.db.WithContext(ctx).Create(&model).Error
}

// GetByID retrieves a payment by ID, or ErrNotFound if it doesn't exist.
func (r *GormPaymentRepository) GetByID(ctx context.Context, id uuid.UUID) (*domain.Payment, error) {
	var model paymentModel
	err := r.db.WithContext(ctx).First(&model, "id = ?", id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return toDomain(model), nil
}

// Update persists changes to an existing payment (status transitions,
// provider reference, etc). Uses Save (full-row overwrite by primary
// key), not Updates, so a field explicitly set to its zero value is
// still written — GORM's Updates silently skips zero-value struct
// fields, which would be a real bug for this method.
func (r *GormPaymentRepository) Update(ctx context.Context, payment *domain.Payment) error {
	model := toModel(payment)
	result := r.db.WithContext(ctx).Save(&model)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

// MarkProcessedAndUpdate inserts the processed-event marker and saves
// payment's state in one transaction. If the marker insert violates
// the (payment_id, event_type) primary key, the whole transaction
// rolls back — nothing is double-applied — and this returns
// (true, nil): the caller knows, with certainty rather than a guess,
// that another call already did this work.
func (r *GormPaymentRepository) MarkProcessedAndUpdate(ctx context.Context, payment *domain.Payment, eventType string) (bool, error) {
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		marker := processedWebhookEventModel{
			PaymentID:   payment.ID,
			EventType:   eventType,
			ProcessedAt: time.Now().UTC(),
		}
		if err := tx.Create(&marker).Error; err != nil {
			return err
		}

		model := toModel(payment)
		return tx.Save(&model).Error
	})
	if err != nil {
		if isUniqueViolation(err) {
			return true, nil
		}
		return false, err
	}
	return false, nil
}
