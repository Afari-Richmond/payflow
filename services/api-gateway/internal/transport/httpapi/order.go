package httpapi

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/Afari-Richmond/payflow/services/api-gateway/internal/client"
)

// OrderCreator is the gateway's dependency on order-service's
// CreateOrder RPC. A narrow interface (not the concrete gRPC client)
// so this handler can be tested without a real network call.
type OrderCreator interface {
	CreateOrder(ctx context.Context, email string, amountMinor int64, currency string) (*client.Order, error)
}

// CreateOrderRequest is the body accepted by POST /api/v1/orders.
type CreateOrderRequest struct {
	Email    string `json:"email" binding:"required,email" example:"customer@example.com"`
	Amount   int64  `json:"amount" binding:"required,gt=0" example:"25000"`
	Currency string `json:"currency" binding:"required,len=3" example:"GHS"`
}

// OrderResponse is the order shape returned to HTTP clients.
type OrderResponse struct {
	ID        string `json:"id"`
	Email     string `json:"email"`
	Amount    int64  `json:"amount"`
	Currency  string `json:"currency"`
	Status    string `json:"status"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

// ErrorResponse is the body returned on failure.
type ErrorResponse struct {
	Error string `json:"error"`
}

// CreateOrderHandler godoc
// @Summary      Create an order
// @Description  Creates a new order in PENDING_PAYMENT status. Payment is confirmed separately, server-side — never by this endpoint alone.
// @Tags         orders
// @Accept       json
// @Produce      json
// @Param        request  body      CreateOrderRequest  true  "Order details"
// @Success      201      {object}  OrderResponse
// @Failure      400      {object}  ErrorResponse
// @Failure      502      {object}  ErrorResponse
// @Router       /api/v1/orders [post]
func CreateOrderHandler(orders OrderCreator) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req CreateOrderRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
			return
		}

		order, err := orders.CreateOrder(c.Request.Context(), req.Email, req.Amount, req.Currency)
		if err != nil {
			if st, ok := status.FromError(err); ok && st.Code() == codes.InvalidArgument {
				c.JSON(http.StatusBadRequest, ErrorResponse{Error: st.Message()})
				return
			}
			c.JSON(http.StatusBadGateway, ErrorResponse{Error: "order-service unreachable"})
			return
		}

		c.JSON(http.StatusCreated, OrderResponse{
			ID:        order.ID,
			Email:     order.Email,
			Amount:    order.AmountMinor,
			Currency:  order.Currency,
			Status:    order.Status,
			CreatedAt: order.CreatedAt,
			UpdatedAt: order.UpdatedAt,
		})
	}
}
