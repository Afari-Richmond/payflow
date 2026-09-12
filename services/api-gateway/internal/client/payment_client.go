package client

import (
	"context"
	"fmt"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"

	paymentv1 "github.com/Afari-Richmond/payflow/proto/payment/v1"
)

// PaymentClient wraps the gRPC connection to payment-service.
type PaymentClient struct {
	conn   *grpc.ClientConn
	client paymentv1.PaymentServiceClient
}

// NewPaymentClient dials payment-service at addr.
func NewPaymentClient(addr string) (*PaymentClient, error) {
	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, err
	}
	return &PaymentClient{
		conn:   conn,
		client: paymentv1.NewPaymentServiceClient(conn),
	}, nil
}

// HandleWebhook forwards a raw webhook delivery to payment-service
// untouched — see ADR 002. The gateway never inspects rawBody or
// signature; payment-service owns verification entirely.
func (c *PaymentClient) HandleWebhook(ctx context.Context, rawBody []byte, signature string) error {
	ctx = withCorrelationID(ctx)
	_, err := c.client.HandleWebhook(ctx, &paymentv1.HandleWebhookRequest{
		RawBody:   rawBody,
		Signature: signature,
	})
	return err
}

// HealthCheck calls payment-service's standard gRPC health check. The
// Check RPC itself succeeding only means the server answered — the
// actual verdict is in the response body, which must be inspected
// separately, since a NOT_SERVING status is a normal (non-error)
// response.
func (c *PaymentClient) HealthCheck(ctx context.Context) error {
	resp, err := healthpb.NewHealthClient(c.conn).Check(ctx, &healthpb.HealthCheckRequest{})
	if err != nil {
		return err
	}
	if resp.GetStatus() != healthpb.HealthCheckResponse_SERVING {
		return fmt.Errorf("payment-service reported status %s", resp.GetStatus())
	}
	return nil
}

// Close closes the underlying gRPC connection.
func (c *PaymentClient) Close() error {
	return c.conn.Close()
}
