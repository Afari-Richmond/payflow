package httpapi_test

import (
	"context"

	"github.com/Afari-Richmond/payflow/services/api-gateway/internal/client"
)

// fakeReadinessChecker is a test double for httpapi.ReadinessChecker.
type fakeReadinessChecker struct {
	statuses []client.DependencyStatus
}

func (f fakeReadinessChecker) CheckReadiness(_ context.Context) []client.DependencyStatus {
	return f.statuses
}
