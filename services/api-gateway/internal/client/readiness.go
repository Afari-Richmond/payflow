package client

import "context"

// HealthChecker is satisfied by both OrderClient and PaymentClient.
type HealthChecker interface {
	HealthCheck(ctx context.Context) error
}

// DependencyStatus is one dependency's readiness result. Defined here
// (not in httpapi) because httpapi already imports client for the
// Order DTO — httpapi.ReadinessChecker references this type instead of
// defining its own, to avoid an import cycle.
type DependencyStatus struct {
	Name  string `json:"name"`
	Ready bool   `json:"ready"`
	Error string `json:"error,omitempty"`
}

// Readiness aggregates health checks across api-gateway's downstream
// gRPC dependencies into a single httpapi.ReadinessChecker.
type Readiness struct {
	deps map[string]HealthChecker
}

// NewReadiness builds a Readiness checker for orderClient and
// paymentClient.
func NewReadiness(orderClient, paymentClient HealthChecker) *Readiness {
	return &Readiness{deps: map[string]HealthChecker{
		"order-service":   orderClient,
		"payment-service": paymentClient,
	}}
}

// CheckReadiness implements httpapi.ReadinessChecker.
func (r *Readiness) CheckReadiness(ctx context.Context) []DependencyStatus {
	// Fixed order so the response is stable across calls — map
	// iteration order isn't, and a flapping field order in a JSON
	// response is a needless annoyance for anything polling it.
	names := []string{"order-service", "payment-service"}

	statuses := make([]DependencyStatus, 0, len(names))
	for _, name := range names {
		dep, ok := r.deps[name]
		if !ok {
			continue
		}
		status := DependencyStatus{Name: name, Ready: true}
		if err := dep.HealthCheck(ctx); err != nil {
			status.Ready = false
			status.Error = err.Error()
		}
		statuses = append(statuses, status)
	}
	return statuses
}
