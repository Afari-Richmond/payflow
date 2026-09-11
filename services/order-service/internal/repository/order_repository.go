// Package repository persists order-service's domain entities.
package repository

import (
	"context"
	"errors"

	"github.com/google/uuid"

	"github.com/Afari-Richmond/payflow/services/order-service/internal/domain"
)

// ErrNotFound is returned when an order does not exist.
var ErrNotFound = errors.New("order not found")

// OrderRepository persists and retrieves orders. Defined as an
// interface so the application layer never depends on a specific
// persistence framework.
type OrderRepository interface {
	Create(ctx context.Context, order *domain.Order) error
	GetByID(ctx context.Context, id uuid.UUID) (*domain.Order, error)
	Update(ctx context.Context, order *domain.Order) error
}
