package messaging_test

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"

	"github.com/Afari-Richmond/payflow/pkg/events"
	"github.com/Afari-Richmond/payflow/services/payment-service/internal/messaging"
)

// dialTestBroker connects to the real RabbitMQ used for local
// development (docker-compose). Skips the test if it's unreachable, so
// `go test` doesn't hard-fail on a machine without Docker running —
// same pattern as the Postgres repository integration tests.
func dialTestBroker(t *testing.T) *amqp.Connection {
	t.Helper()

	url := os.Getenv("RABBITMQ_URL")
	if url == "" {
		url = "amqp://payflow:payflow@localhost:5672/"
	}

	conn, err := amqp.Dial(url)
	if err != nil {
		t.Skipf("skipping: RabbitMQ unreachable: %v", err)
	}
	t.Cleanup(func() { conn.Close() })
	return conn
}

// TestPublisher_Publish_RealBroker publishes a real message to a real
// RabbitMQ exchange, then consumes it back via a raw AMQP consumer
// (bypassing our own Consumer type) to verify the publisher actually
// put a correctly-shaped message on the wire.
func TestPublisher_Publish_RealBroker(t *testing.T) {
	conn := dialTestBroker(t)

	publisher, err := messaging.NewPublisher(conn)
	if err != nil {
		t.Fatalf("failed to create publisher: %v", err)
	}
	defer publisher.Close()

	verifyCh, err := conn.Channel()
	if err != nil {
		t.Fatalf("failed to open verification channel: %v", err)
	}
	defer verifyCh.Close()

	q, err := verifyCh.QueueDeclare("", false, true, true, false, nil)
	if err != nil {
		t.Fatalf("failed to declare verification queue: %v", err)
	}
	if err := verifyCh.QueueBind(q.Name, "payment.succeeded", messaging.ExchangeName, false, nil); err != nil {
		t.Fatalf("failed to bind verification queue: %v", err)
	}

	msgs, err := verifyCh.Consume(q.Name, "", true, true, false, false, nil)
	if err != nil {
		t.Fatalf("failed to start verification consumer: %v", err)
	}

	payload := events.PaymentSucceededPayload{
		PaymentID:   "payment-integration-test",
		OrderID:     "order-integration-test",
		AmountMinor: 25000,
		Currency:    "GHS",
	}
	envelope, err := events.NewEnvelope(events.PaymentSucceeded, payload.PaymentID, payload)
	if err != nil {
		t.Fatalf("failed to build envelope: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := publisher.Publish(ctx, events.PaymentSucceeded, envelope); err != nil {
		t.Fatalf("Publish failed: %v", err)
	}

	select {
	case msg := <-msgs:
		var received events.Envelope
		if err := json.Unmarshal(msg.Body, &received); err != nil {
			t.Fatalf("failed to decode received message: %v", err)
		}
		if received.EventID != envelope.EventID {
			t.Errorf("expected event id %q, got %q", envelope.EventID, received.EventID)
		}
		var receivedPayload events.PaymentSucceededPayload
		if err := json.Unmarshal(received.Payload, &receivedPayload); err != nil {
			t.Fatalf("failed to decode received payload: %v", err)
		}
		if receivedPayload != payload {
			t.Errorf("expected payload %+v, got %+v", payload, receivedPayload)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for the published message to arrive")
	}
}
