package httpapi

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"
)

// OrderPinger is the gateway's dependency on order-service's temporary
// Ping RPC. A narrow interface (not the concrete client) so this
// handler can be tested without a real gRPC connection.
type OrderPinger interface {
	Ping(ctx context.Context, message string) (string, error)
}

// OrderPingResponse is the body returned by GET /internal/order-ping.
type OrderPingResponse struct {
	Message string `json:"message" example:"order-service received: ping from api-gateway"`
}

// OrderPingHandler is a TEMPORARY diagnostic endpoint proving the
// api-gateway -> order-service gRPC round trip (Milestone 3). It is
// removed once real order endpoints exist (Milestone 5) — no client
// should depend on it.
//
// OrderPingHandler godoc
// @Summary      Ping order-service (temporary)
// @Description  Proves the gateway-to-order-service gRPC round trip. Not part of the stable API — removed once real order endpoints exist.
// @Tags         internal
// @Produce      json
// @Success      200  {object}  OrderPingResponse
// @Failure      502  {object}  map[string]string
// @Router       /internal/order-ping [get]
func OrderPingHandler(pinger OrderPinger) gin.HandlerFunc {
	return func(c *gin.Context) {
		msg, err := pinger.Ping(c.Request.Context(), "ping from api-gateway")
		if err != nil {
			c.JSON(http.StatusBadGateway, gin.H{"error": "order-service unreachable"})
			return
		}
		c.JSON(http.StatusOK, OrderPingResponse{Message: msg})
	}
}
