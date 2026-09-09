package httpapi_test

import "context"

// fakeOrderPinger is a test double for httpapi.OrderPinger — lets tests
// exercise the HTTP <-> gRPC translation without a real network call.
type fakeOrderPinger struct {
	message string
	err     error
}

func (f fakeOrderPinger) Ping(_ context.Context, _ string) (string, error) {
	return f.message, f.err
}
