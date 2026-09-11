package correlation_test

import (
	"context"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"

	"github.com/Afari-Richmond/payflow/pkg/correlation"
)

func TestUnaryServerInterceptor_ExtractsIDFromMetadata(t *testing.T) {
	id := correlation.New()
	ctx := metadata.NewIncomingContext(context.Background(), metadata.Pairs(correlation.MetadataKey, id))

	var gotID string
	handler := func(ctx context.Context, req any) (any, error) {
		gotID = correlation.FromContext(ctx)
		return nil, nil
	}

	if _, err := correlation.UnaryServerInterceptor()(ctx, nil, &grpc.UnaryServerInfo{}, handler); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotID != id {
		t.Errorf("expected correlation ID %q in handler context, got %q", id, gotID)
	}
}

func TestUnaryServerInterceptor_GeneratesIDWhenMissing(t *testing.T) {
	var gotID string
	handler := func(ctx context.Context, req any) (any, error) {
		gotID = correlation.FromContext(ctx)
		return nil, nil
	}

	if _, err := correlation.UnaryServerInterceptor()(context.Background(), nil, &grpc.UnaryServerInfo{}, handler); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotID == "" {
		t.Error("expected a generated correlation ID, got empty string")
	}
}
