// Package events is the shared contract between event producers and
// consumers across PayFlow services — the message-queue equivalent of
// proto/ for gRPC. It defines the envelope shape and known event
// payloads; it holds no business logic and no service-specific code.
package events

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"

	"github.com/Afari-Richmond/payflow/pkg/correlation"
)

// Envelope wraps every event published to RabbitMQ. AggregateID is the
// ID of the entity the event is about (e.g. a payment ID) — not
// necessarily the same as any field inside Payload. CorrelationID
// carries the originating request's correlation ID (see pkg/correlation)
// across the async hop, so a consumer's logs can be tied back to the
// HTTP request that ultimately caused this event.
type Envelope struct {
	EventID       string          `json:"event_id"`
	EventType     string          `json:"event_type"`
	AggregateID   string          `json:"aggregate_id"`
	CorrelationID string          `json:"correlation_id,omitempty"`
	OccurredAt    time.Time       `json:"occurred_at"`
	Version       int             `json:"version"`
	Payload       json.RawMessage `json:"payload"`
}

// NewEnvelope builds an Envelope, marshaling payload into it. EventID
// and OccurredAt are always generated here — a producer never supplies
// them directly, so every event has a genuinely unique ID and an
// accurate timestamp. CorrelationID is read from ctx (via
// pkg/correlation) automatically — the caller doesn't have to thread
// it through as a separate parameter, and an event built outside any
// request context simply has an empty one.
func NewEnvelope(ctx context.Context, eventType, aggregateID string, payload any) (Envelope, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return Envelope{}, err
	}
	return Envelope{
		EventID:       uuid.NewString(),
		EventType:     eventType,
		AggregateID:   aggregateID,
		CorrelationID: correlation.FromContext(ctx),
		OccurredAt:    time.Now().UTC(),
		Version:       1,
		Payload:       body,
	}, nil
}
