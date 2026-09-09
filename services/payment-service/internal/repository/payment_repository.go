// Package repository persists payment-service's domain entities.
package repository

import (
	"context"
	"errors"

	"github.com/google/uuid"

	"github.com/Afari-Richmond/payflow/services/payment-service/internal/domain"
)

// ErrNotFound is returned when a payment does not exist.
var ErrNotFound = errors.New("payment not found")

// PaymentRepository persists and retrieves payments. Defined as an
// interface so the application layer never depends on a specific
// persistence framework.
type PaymentRepository interface {
	Create(ctx context.Context, payment *domain.Payment) error
	GetByID(ctx context.Context, id uuid.UUID) (*domain.Payment, error)
}
