package events_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/Afari-Richmond/payflow/pkg/correlation"
	"github.com/Afari-Richmond/payflow/pkg/events"
)

func TestNewEnvelope_RoundTrip(t *testing.T) {
	payload := events.PaymentSucceededPayload{
		PaymentID:   "payment-123",
		OrderID:     "order-456",
		AmountMinor: 25000,
		Currency:    "GHS",
	}

	envelope, err := events.NewEnvelope(context.Background(), events.PaymentSucceeded, payload.PaymentID, payload)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if envelope.EventID == "" {
		t.Error("expected a generated event ID")
	}
	if envelope.EventType != events.PaymentSucceeded {
		t.Errorf("expected event type %q, got %q", events.PaymentSucceeded, envelope.EventType)
	}
	if envelope.AggregateID != payload.PaymentID {
		t.Errorf("expected aggregate id %q, got %q", payload.PaymentID, envelope.AggregateID)
	}
	if envelope.OccurredAt.IsZero() {
		t.Error("expected a non-zero OccurredAt")
	}
	if envelope.Version != 1 {
		t.Errorf("expected version 1, got %d", envelope.Version)
	}

	var decoded events.PaymentSucceededPayload
	if err := json.Unmarshal(envelope.Payload, &decoded); err != nil {
		t.Fatalf("failed to decode payload: %v", err)
	}
	if decoded != payload {
		t.Errorf("expected payload %+v, got %+v", payload, decoded)
	}
}

func TestEnvelope_JSONRoundTrip(t *testing.T) {
	envelope, err := events.NewEnvelope(context.Background(), events.PaymentFailed, "payment-123", events.PaymentFailedPayload{
		PaymentID: "payment-123",
		OrderID:   "order-456",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	wireBytes, err := json.Marshal(envelope)
	if err != nil {
		t.Fatalf("failed to marshal envelope: %v", err)
	}

	var decoded events.Envelope
	if err := json.Unmarshal(wireBytes, &decoded); err != nil {
		t.Fatalf("failed to unmarshal envelope: %v", err)
	}

	if decoded.EventID != envelope.EventID {
		t.Errorf("expected event id %q, got %q", envelope.EventID, decoded.EventID)
	}
	if decoded.EventType != events.PaymentFailed {
		t.Errorf("expected event type %q, got %q", events.PaymentFailed, decoded.EventType)
	}
}

func TestNewEnvelope_CarriesCorrelationIDFromContext(t *testing.T) {
	id := correlation.New()
	ctx := correlation.WithID(context.Background(), id)

	envelope, err := events.NewEnvelope(ctx, events.PaymentSucceeded, "payment-123", events.PaymentSucceededPayload{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if envelope.CorrelationID != id {
		t.Errorf("expected correlation id %q, got %q", id, envelope.CorrelationID)
	}
}

func TestNewEnvelope_NoCorrelationIDInContext(t *testing.T) {
	envelope, err := events.NewEnvelope(context.Background(), events.PaymentSucceeded, "payment-123", events.PaymentSucceededPayload{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if envelope.CorrelationID != "" {
		t.Errorf("expected empty correlation id when none was set, got %q", envelope.CorrelationID)
	}
}
