// Package repository persists payment-service's domain entities.
package repository

import (
	"context"
	"errors"

	"github.com/google/uuid"

	"github.com/Afari-Richmond/payflow/services/payment-service/internal/domain"
	"github.com/Afari-Richmond/payflow/services/payment-service/internal/outbox"
)

// ErrNotFound is returned when a payment does not exist.
var ErrNotFound = errors.New("payment not found")

// PaymentRepository persists and retrieves payments. Defined as an
// interface so the application layer never depends on a specific
// persistence framework.
type PaymentRepository interface {
	Create(ctx context.Context, payment *domain.Payment) error
	GetByID(ctx context.Context, id uuid.UUID) (*domain.Payment, error)
	Update(ctx context.Context, payment *domain.Payment) error

	// MarkProcessedAndUpdate atomically records that eventType has been
	// processed for payment, persists payment's current state, and
	// enqueues outboxEvent (if non-nil) for reliable delivery — all in
	// one transaction. Returns (true, nil) if eventType was already
	// processed for this payment — a duplicate delivery — in which
	// case no changes were made and no event was enqueued. Returns
	// (false, nil) the first time.
	MarkProcessedAndUpdate(ctx context.Context, payment *domain.Payment, eventType string, outboxEvent *outbox.Event) (alreadyProcessed bool, err error)
}
