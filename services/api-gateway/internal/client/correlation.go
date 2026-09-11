package client

import (
	"context"

	"google.golang.org/grpc/metadata"

	"github.com/Afari-Richmond/payflow/pkg/correlation"
)

// withCorrelationID attaches the request's correlation ID (set on ctx
// by httpapi's correlation middleware) as outgoing gRPC metadata, so
// order-service/payment-service's own interceptors can pick it up and
// log against the same ID. A no-op if ctx carries no correlation ID
// (e.g. a call made outside an HTTP request's context).
func withCorrelationID(ctx context.Context) context.Context {
	id := correlation.FromContext(ctx)
	if id == "" {
		return ctx
	}
	return metadata.AppendToOutgoingContext(ctx, correlation.MetadataKey, id)
}
