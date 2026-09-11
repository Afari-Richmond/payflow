// Package outbox implements the transactional outbox pattern:
// payload written durably alongside the business state change it
// accompanies, published to the broker by a separate worker that
// tolerates the broker being temporarily unavailable. See
// docs/adr/005-transactional-outbox.md.
package outbox

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// Event is a durably-stored event awaiting publication. Payload is
// already the fully-encoded wire envelope (see pkg/events.Envelope) —
// the outbox doesn't know or care about event schemas, only that it
// must deliver these bytes to routing_key eventually.
type Event struct {
	ID           uuid.UUID
	AggregateID  uuid.UUID
	EventType    string
	RoutingKey   string
	Payload      []byte
	CreatedAt    time.Time
	PublishedAt  *time.Time
	AttemptCount int
}

// Repository claims and processes unpublished outbox events.
type Repository interface {
	// ProcessUnpublished claims up to limit unpublished events — via
	// SELECT ... FOR UPDATE SKIP LOCKED, so multiple worker instances
	// never claim the same row — and calls handle for each one within
	// the same transaction. handle returning nil marks the event
	// published; a non-nil error increments its attempt count instead.
	// The transaction stays open for the duration of handle (including
	// the network call to the broker) so the claim is real: no other
	// worker can see these rows until this transaction commits.
	ProcessUnpublished(ctx context.Context, limit int, handle func(*Event) error) error
}
