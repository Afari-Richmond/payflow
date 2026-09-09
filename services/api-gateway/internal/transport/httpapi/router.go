// Package httpapi wires api-gateway's HTTP transport: routing and the
// handlers exposed to external clients. Handlers here validate and
// translate only — no business logic lives in this package.
package httpapi

import (
	"log/slog"
	"time"

	"github.com/gin-gonic/gin"
)

// NewRouter builds the api-gateway's HTTP router.
func NewRouter(logger *slog.Logger, orders OrderCreator) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)

	router := gin.New()
	router.Use(gin.Recovery())
	router.Use(requestLogger(logger))

	router.GET("/health", HealthHandler)
	router.GET("/docs", DocsHandler)
	router.GET("/docs/swagger.json", SwaggerSpecHandler)
	router.POST("/api/v1/orders", CreateOrderHandler(orders))

	return router
}

func requestLogger(logger *slog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()
		logger.Info("request",
			"method", c.Request.Method,
			"path", c.Request.URL.Path,
			"status", c.Writer.Status(),
			"duration_ms", time.Since(start).Milliseconds(),
		)
	}
}
