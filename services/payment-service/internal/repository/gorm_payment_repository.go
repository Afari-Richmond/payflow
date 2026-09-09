package repository

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/Afari-Richmond/payflow/services/payment-service/internal/domain"
)

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
