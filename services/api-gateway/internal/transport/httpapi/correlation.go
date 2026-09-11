package httpapi

import (
	"github.com/gin-gonic/gin"

	"github.com/Afari-Richmond/payflow/pkg/correlation"
)

// correlationMiddleware reads an inbound X-Correlation-ID header, or
// generates a fresh one if absent, and carries it on the request's
// context for the rest of the handler chain — including the gRPC
// clients, which forward it as outgoing metadata. Also echoes it back
// on the response header, so a caller that didn't supply one can still
// find it in logs afterward.
func correlationMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.GetHeader(correlation.HeaderName)
		if id == "" {
			id = correlation.New()
		}

		ctx := correlation.WithID(c.Request.Context(), id)
		c.Request = c.Request.WithContext(ctx)
		c.Header(correlation.HeaderName, id)

		c.Next()
	}
}
