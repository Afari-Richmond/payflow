package messaging_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"os"
	"testing"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"

	"github.com/Afari-Richmond/payflow/pkg/events"
	"github.com/Afari-Richmond/payflow/services/order-service/internal/messaging"
)

// dialTestBroker connects to the real RabbitMQ used for local
// development (docker-compose). Skips the test if it's unreachable —
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

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// publishRaw publishes directly via a raw AMQP channel, bypassing
// payment-service's Publisher entirely — this test verifies the
// Consumer's own behavior against the real broker, independent of the
// other service's code.
func publishRaw(t *testing.T, conn *amqp.Connection, routingKey string, envelope events.Envelope) {
	t.Helper()
	ch, err := conn.Channel()
	if err != nil {
		t.Fatalf("failed to open publish channel: %v", err)
	}
	defer ch.Close()

	if err := ch.ExchangeDeclare(messaging.ExchangeName, "topic", true, false, false, false, nil); err != nil {
		t.Fatalf("failed to declare exchange: %v", err)
	}

	body, err := json.Marshal(envelope)
	if err != nil {
		t.Fatalf("failed to marshal envelope: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := ch.PublishWithContext(ctx, messaging.ExchangeName, routingKey, false, false, amqp.Publishing{
		ContentType: "application/json",
		Body:        body,
	}); err != nil {
		t.Fatalf("failed to publish: %v", err)
	}
}

func TestConsumer_Run_ProcessesAndAcksSuccessfully(t *testing.T) {
	conn := dialTestBroker(t)

	consumer, err := messaging.NewConsumer(conn, testLogger())
	if err != nil {
		t.Fatalf("failed to create consumer: %v", err)
	}
	defer consumer.Close()

	envelope, err := events.NewEnvelope(context.Background(), events.PaymentSucceeded, "payment-consumer-test", events.PaymentSucceededPayload{
		PaymentID: "payment-consumer-test",
		OrderID:   "order-consumer-test",
	})
	if err != nil {
		t.Fatalf("failed to build envelope: %v", err)
	}
	publishRaw(t, conn, events.PaymentSucceeded, envelope)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	received := make(chan events.Envelope, 1)
	go consumer.Run(ctx, func(_ context.Context, e events.Envelope) error {
		received <- e
		cancel() // stop the consumer once we've got what we need
		return nil
	})

	select {
	case got := <-received:
		if got.EventID != envelope.EventID {
			t.Errorf("expected event id %q, got %q", envelope.EventID, got.EventID)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for the consumer to process the message")
	}
}

func TestConsumer_Run_PermanentErrorDeadLetters(t *testing.T) {
	conn := dialTestBroker(t)

	// Drain the DLQ from any prior test runs first, so this test's
	// assertion isn't polluted by leftovers.
	drainCh, err := conn.Channel()
	if err != nil {
		t.Fatalf("failed to open drain channel: %v", err)
	}
	for {
		_, ok, derr := drainCh.Get("order-service.payment-events.dlq", true)
		if derr != nil || !ok {
			break
		}
	}
	drainCh.Close()

	consumer, err := messaging.NewConsumer(conn, testLogger())
	if err != nil {
		t.Fatalf("failed to create consumer: %v", err)
	}
	defer consumer.Close()

	envelope, err := events.NewEnvelope(context.Background(), events.PaymentSucceeded, "payment-dlq-test", events.PaymentSucceededPayload{
		PaymentID: "payment-dlq-test",
	})
	if err != nil {
		t.Fatalf("failed to build envelope: %v", err)
	}
	publishRaw(t, conn, events.PaymentSucceeded, envelope)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	go consumer.Run(ctx, func(_ context.Context, _ events.Envelope) error {
		return events.NewPermanentError(errors.New("simulated unrecoverable failure"))
	})

	dlqCh, err := conn.Channel()
	if err != nil {
		t.Fatalf("failed to open DLQ channel: %v", err)
	}
	defer dlqCh.Close()

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		msg, ok, err := dlqCh.Get("order-service.payment-events.dlq", true)
		if err == nil && ok {
			var dead events.Envelope
			if err := json.Unmarshal(msg.Body, &dead); err != nil {
				t.Fatalf("failed to decode dead-lettered message: %v", err)
			}
			if dead.EventID != envelope.EventID {
				t.Errorf("expected event id %q in DLQ, got %q", envelope.EventID, dead.EventID)
			}
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatal("timed out waiting for the message to be dead-lettered")
}
