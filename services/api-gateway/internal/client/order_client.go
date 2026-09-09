// Package client holds api-gateway's outbound gRPC clients to internal
// services.
package client

import (
	"context"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	orderv1 "github.com/Afari-Richmond/payflow/proto/order/v1"
)

// OrderClient wraps the gRPC connection to order-service.
type OrderClient struct {
	conn   *grpc.ClientConn
	client orderv1.OrderServiceClient
}

// NewOrderClient dials order-service at addr. grpc.NewClient does not
// block or fail on an unreachable target — connection establishment
// happens lazily on the first RPC.
func NewOrderClient(addr string) (*OrderClient, error) {
	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, err
	}
	return &OrderClient{
		conn:   conn,
		client: orderv1.NewOrderServiceClient(conn),
	}, nil
}

// Ping calls order-service's temporary Ping RPC, proving the gateway ->
// order-service gRPC round trip. Removed once CreateOrder (Milestone 5)
// replaces it with a real order-service client method.
func (c *OrderClient) Ping(ctx context.Context, message string) (string, error) {
	resp, err := c.client.Ping(ctx, &orderv1.PingRequest{Message: message})
	if err != nil {
		return "", err
	}
	return resp.GetMessage(), nil
}

// Close closes the underlying gRPC connection.
func (c *OrderClient) Close() error {
	return c.conn.Close()
}
