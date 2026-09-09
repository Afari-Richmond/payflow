// Package domain holds payment-service's business entities, independent
// of any transport or persistence framework.
package domain

import (
	"time"

	"github.com/google/uuid"
)

// Status is a payment's lifecycle state. These are PayFlow's own
// internal business states — external provider (Paystack) statuses are
// translated into these, never mirrored blindly.
type Status string

const (
	StatusPending     Status = "PENDING"
	StatusInitialized Status = "INITIALIZED"
	StatusProcessing  Status = "PROCESSING"
	StatusSuccess     Status = "SUCCESS"
	StatusFailed      Status = "FAILED"
	StatusRefunded    Status = "REFUNDED"
)

// Payment is PayFlow's core payment entity. OrderID references an
// order owned by order-service — a different service's database, so
// this is a logical reference only, never a database foreign key.
// AmountMinor is always an integer minor-currency-unit amount — never
// a float.
type Payment struct {
	ID                uuid.UUID
	OrderID           uuid.UUID
	AmountMinor       int64
	Currency          string
	Status            Status
	ProviderReference string // empty until Milestone 7 (Paystack)
	CreatedAt         time.Time
	UpdatedAt         time.Time
}
