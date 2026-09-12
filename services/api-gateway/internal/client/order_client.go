// Package client holds api-gateway's outbound gRPC clients to internal
// services.
package client

import (
	"context"
	"fmt"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"

	orderv1 "github.com/Afari-Richmond/payflow/proto/order/v1"
)

// Order is the gateway's own DTO for an order, decoupled from the
// generated protobuf type — the HTTP layer never depends on gRPC
// message shapes directly.
type Order struct {
	ID          string
	Email       string
	AmountMinor int64
	Currency    string
	Status      string
	CreatedAt   string
	UpdatedAt   string
}

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

// CreateOrder calls order-service's CreateOrder RPC.
func (c *OrderClient) CreateOrder(ctx context.Context, email string, amountMinor int64, currency string) (*Order, error) {
	ctx = withCorrelationID(ctx)
	resp, err := c.client.CreateOrder(ctx, &orderv1.CreateOrderRequest{
		Email:       email,
		AmountMinor: amountMinor,
		Currency:    currency,
	})
	if err != nil {
		return nil, err
	}

	o := resp.GetOrder()
	return &Order{
		ID:          o.GetId(),
		Email:       o.GetEmail(),
		AmountMinor: o.GetAmountMinor(),
		Currency:    o.GetCurrency(),
		Status:      o.GetStatus(),
		CreatedAt:   o.GetCreatedAt(),
		UpdatedAt:   o.GetUpdatedAt(),
	}, nil
}

// HealthCheck calls order-service's standard gRPC health check. The
// Check RPC itself succeeding only means the server answered — the
// actual verdict is in the response body, which must be inspected
// separately, since a NOT_SERVING status is a normal (non-error)
// response.
func (c *OrderClient) HealthCheck(ctx context.Context) error {
	resp, err := healthpb.NewHealthClient(c.conn).Check(ctx, &healthpb.HealthCheckRequest{})
	if err != nil {
		return err
	}
	if resp.GetStatus() != healthpb.HealthCheckResponse_SERVING {
		return fmt.Errorf("order-service reported status %s", resp.GetStatus())
	}
	return nil
}

// Close closes the underlying gRPC connection.
func (c *OrderClient) Close() error {
	return c.conn.Close()
}
