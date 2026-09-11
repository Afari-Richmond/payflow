package correlation_test

import (
	"context"
	"testing"

	"github.com/Afari-Richmond/payflow/pkg/correlation"
)

func TestWithID_FromContext_RoundTrip(t *testing.T) {
	id := correlation.New()
	ctx := correlation.WithID(context.Background(), id)

	if got := correlation.FromContext(ctx); got != id {
		t.Errorf("expected %q, got %q", id, got)
	}
}

func TestFromContext_NoIDSet(t *testing.T) {
	if got := correlation.FromContext(context.Background()); got != "" {
		t.Errorf("expected empty string, got %q", got)
	}
}

func TestNew_GeneratesDistinctIDs(t *testing.T) {
	first := correlation.New()
	second := correlation.New()
	if first == second {
		t.Error("expected distinct IDs from successive calls")
	}
}
