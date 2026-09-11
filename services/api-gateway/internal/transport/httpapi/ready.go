package httpapi

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/Afari-Richmond/payflow/services/api-gateway/internal/client"
)

// ReadinessChecker reports whether the gateway's downstream
// dependencies are currently reachable — the distinction from
// liveness (/health, "is this process alive") being that a process
// can be alive but unable to actually serve a request if a dependency
// it needs is down.
type ReadinessChecker interface {
	CheckReadiness(ctx context.Context) []client.DependencyStatus
}

// ReadyResponse is the body returned by GET /ready.
type ReadyResponse struct {
	Ready        bool                      `json:"ready"`
	Dependencies []client.DependencyStatus `json:"dependencies"`
}

// ReadyHandler godoc
// @Summary      Readiness check
// @Description  Reports whether downstream dependencies (order-service, payment-service) are reachable. Distinct from /health (liveness) — a process can be alive but not ready.
// @Tags         health
// @Produce      json
// @Success      200  {object}  ReadyResponse
// @Failure      503  {object}  ReadyResponse
// @Router       /ready [get]
func ReadyHandler(checker ReadinessChecker) gin.HandlerFunc {
	return func(c *gin.Context) {
		deps := checker.CheckReadiness(c.Request.Context())

		ready := true
		for _, d := range deps {
			if !d.Ready {
				ready = false
				break
			}
		}

		status := http.StatusOK
		if !ready {
			status = http.StatusServiceUnavailable
		}

		c.JSON(status, ReadyResponse{Ready: ready, Dependencies: deps})
	}
}
