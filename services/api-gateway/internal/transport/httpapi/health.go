package httpapi

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// HealthResponse is the body returned by GET /health.
type HealthResponse struct {
	Status string `json:"status" example:"ok"`
}

// HealthHandler godoc
// @Summary      Health check
// @Description  Reports whether the api-gateway process is up and serving requests.
// @Tags         health
// @Produce      json
// @Success      200  {object}  HealthResponse
// @Router       /health [get]
func HealthHandler(c *gin.Context) {
	c.JSON(http.StatusOK, HealthResponse{Status: "ok"})
}
