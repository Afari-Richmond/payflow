// Package domain holds order-service's business entities, independent
// of any transport or persistence framework.
package domain

import (
	"time"

	"github.com/google/uuid"
)

// Status is an order's lifecycle state.
type Status string

const (
	// StatusPendingPayment is an order's initial state: created, but
	// payment has not yet been confirmed.
	StatusPendingPayment Status = "PENDING_PAYMENT"
	// StatusPaid means payment has been confirmed server-side (never
	// set merely because a client reached a success page).
	StatusPaid Status = "PAID"
)

// Order is PayFlow's core order entity. AmountMinor is always an
// integer minor-currency-unit amount (e.g. pesewas for GHS) — never a
// float.
type Order struct {
	ID          uuid.UUID
	Email       string
	AmountMinor int64
	Currency    string
	Status      Status
	CreatedAt   time.Time
	UpdatedAt   time.Time
}
