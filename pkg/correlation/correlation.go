// Package correlation carries a correlation ID across every hop a
// single request can touch: HTTP header at the edge, gRPC metadata
// between services, and the event envelope across RabbitMQ. It holds
// no business logic — just the shared key names and a context
// carrier, the same role pkg/events and proto/ play for their own
// concerns.
package correlation

import (
	"context"

	"github.com/google/uuid"
)

// HeaderName is the HTTP header api-gateway reads an inbound
// correlation ID from, and sets on its own responses.
const HeaderName = "X-Correlation-ID"

// MetadataKey is the gRPC metadata key the ID travels under between
// services. gRPC lowercases metadata keys, so this is already lower
// case to match what's actually received.
const MetadataKey = "x-correlation-id"

type contextKey struct{}

// New generates a fresh correlation ID.
func New() string {
	return uuid.NewString()
}

// WithID returns a context carrying id.
func WithID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, contextKey{}, id)
}

// FromContext returns the correlation ID carried by ctx, or "" if
// none was set.
func FromContext(ctx context.Context) string {
	id, _ := ctx.Value(contextKey{}).(string)
	return id
}
