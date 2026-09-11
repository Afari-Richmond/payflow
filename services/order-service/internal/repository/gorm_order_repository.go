package repository

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/Afari-Richmond/payflow/services/order-service/internal/domain"
)

// orderModel is the GORM-mapped persistence shape for an order. Kept
// separate from domain.Order so the domain package stays free of
// persistence-framework tags.
type orderModel struct {
	ID          uuid.UUID `gorm:"column:id;primaryKey"`
	Email       string    `gorm:"column:email"`
	AmountMinor int64     `gorm:"column:amount_minor"`
	Currency    string    `gorm:"column:currency"`
	Status      string    `gorm:"column:status"`
	CreatedAt   time.Time `gorm:"column:created_at"`
	UpdatedAt   time.Time `gorm:"column:updated_at"`
}

func (orderModel) TableName() string { return "orders" }

func toModel(o *domain.Order) orderModel {
	return orderModel{
		ID:          o.ID,
		Email:       o.Email,
		AmountMinor: o.AmountMinor,
		Currency:    o.Currency,
		Status:      string(o.Status),
		CreatedAt:   o.CreatedAt,
		UpdatedAt:   o.UpdatedAt,
	}
}

func toDomain(m orderModel) *domain.Order {
	return &domain.Order{
		ID:          m.ID,
		Email:       m.Email,
		AmountMinor: m.AmountMinor,
		Currency:    m.Currency,
		Status:      domain.Status(m.Status),
		CreatedAt:   m.CreatedAt,
		UpdatedAt:   m.UpdatedAt,
	}
}

// GormOrderRepository is a PostgreSQL-backed OrderRepository using GORM.
type GormOrderRepository struct {
	db *gorm.DB
}

// NewGormOrderRepository builds a GormOrderRepository.
func NewGormOrderRepository(db *gorm.DB) *GormOrderRepository {
	return &GormOrderRepository{db: db}
}

// Create inserts a new order.
func (r *GormOrderRepository) Create(ctx context.Context, order *domain.Order) error {
	model := toModel(order)
	return r.db.WithContext(ctx).Create(&model).Error
}

// GetByID retrieves an order by ID, or ErrNotFound if it doesn't exist.
func (r *GormOrderRepository) GetByID(ctx context.Context, id uuid.UUID) (*domain.Order, error) {
	var model orderModel
	err := r.db.WithContext(ctx).First(&model, "id = ?", id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return toDomain(model), nil
}

// Update persists changes to an existing order (status transitions,
// etc). Uses Save (full-row overwrite by primary key), not Updates —
// GORM's Updates silently skips zero-value struct fields.
func (r *GormOrderRepository) Update(ctx context.Context, order *domain.Order) error {
	model := toModel(order)
	result := r.db.WithContext(ctx).Save(&model)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}
