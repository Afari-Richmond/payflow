// Package grpcapi implements order-service's gRPC transport: the
// OrderService contract defined in proto/order/v1.
package grpcapi

import (
	"context"

	orderv1 "github.com/Afari-Richmond/payflow/proto/order/v1"
)

// OrderServer implements orderv1.OrderServiceServer.
type OrderServer struct {
	orderv1.UnimplementedOrderServiceServer
}

// NewOrderServer builds an OrderServer.
func NewOrderServer() *OrderServer {
	return &OrderServer{}
}

// Ping is a temporary connectivity-proof RPC (see order.proto) — it will
// be removed once CreateOrder (Milestone 5) lands.
func (s *OrderServer) Ping(_ context.Context, req *orderv1.PingRequest) (*orderv1.PingResponse, error) {
	return &orderv1.PingResponse{
		Message: "order-service received: " + req.GetMessage(),
	}, nil
}
