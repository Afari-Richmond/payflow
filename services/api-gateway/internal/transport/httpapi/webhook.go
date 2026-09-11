package httpapi

import (
	"context"
	"io"
	"net/http"

	"github.com/gin-gonic/gin"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// WebhookForwarder is the gateway's dependency for relaying a webhook
// delivery to the service that owns it. A narrow interface (not the
// concrete gRPC client) so this handler can be tested without a real
// network call.
type WebhookForwarder interface {
	HandleWebhook(ctx context.Context, rawBody []byte, signature string) error
}

// PaystackWebhookHandler receives Paystack webhook deliveries and
// forwards them, byte-for-byte, to payment-service — see ADR 002. This
// handler deliberately does not use c.ShouldBindJSON or any other body
// parsing: the raw bytes must reach payment-service exactly as
// received, since signature verification depends on them.
func PaystackWebhookHandler(webhooks WebhookForwarder) gin.HandlerFunc {
	return func(c *gin.Context) {
		rawBody, err := io.ReadAll(c.Request.Body)
		if err != nil {
			c.Status(http.StatusBadRequest)
			return
		}

		signature := c.GetHeader("X-Paystack-Signature")

		err = webhooks.HandleWebhook(c.Request.Context(), rawBody, signature)
		if err != nil {
			if st, ok := status.FromError(err); ok {
				switch st.Code() {
				case codes.Unauthenticated:
					c.Status(http.StatusUnauthorized)
					return
				case codes.InvalidArgument:
					c.Status(http.StatusBadRequest)
					return
				}
			}
			c.Status(http.StatusInternalServerError)
			return
		}

		c.Status(http.StatusOK)
	}
}
