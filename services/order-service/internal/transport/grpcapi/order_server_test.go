package grpcapi_test

import (
	"context"
	"testing"

	orderv1 "github.com/Afari-Richmond/payflow/proto/order/v1"
	"github.com/Afari-Richmond/payflow/services/order-service/internal/transport/grpcapi"
)

func TestOrderServer_Ping(t *testing.T) {
	server := grpcapi.NewOrderServer()

	resp, err := server.Ping(context.Background(), &orderv1.PingRequest{Message: "hello"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := "order-service received: hello"
	if resp.GetMessage() != want {
		t.Errorf("expected message %q, got %q", want, resp.GetMessage())
	}
}
