package httpapi_test

import (
	"context"

	"github.com/Afari-Richmond/payflow/services/api-gateway/internal/client"
)

// fakeOrderCreator is a test double for httpapi.OrderCreator — lets
// tests exercise the HTTP <-> gRPC translation without a real network
// call.
type fakeOrderCreator struct {
	order *client.Order
	err   error
}

func (f fakeOrderCreator) CreateOrder(_ context.Context, _ string, _ int64, _ string) (*client.Order, error) {
	return f.order, f.err
}
