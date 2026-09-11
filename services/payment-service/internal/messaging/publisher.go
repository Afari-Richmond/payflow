// Package messaging is payment-service's RabbitMQ publish side.
package messaging

import (
	"context"
	"encoding/json"

	amqp "github.com/rabbitmq/amqp091-go"

	"github.com/Afari-Richmond/payflow/pkg/events"
)

// ExchangeName is the topic exchange domain events are published to.
// Bound queues (order-service's, for now) subscribe with a routing-key
// pattern like "payment.*" rather than needing to know about every
// producer individually.
const ExchangeName = "payflow.events"

// Publisher publishes domain events to the payflow.events exchange.
type Publisher struct {
	channel *amqp.Channel
}

// NewPublisher declares the exchange (idempotent — safe if it already
// exists) and returns a ready-to-use Publisher.
func NewPublisher(conn *amqp.Connection) (*Publisher, error) {
	ch, err := conn.Channel()
	if err != nil {
		return nil, err
	}

	err = ch.ExchangeDeclare(
		ExchangeName,
		"topic",
		true,  // durable — survives a broker restart
		false, // auto-delete
		false, // internal
		false, // no-wait
		nil,
	)
	if err != nil {
		ch.Close()
		return nil, err
	}

	return &Publisher{channel: ch}, nil
}

// Publish sends envelope to the exchange with routingKey. No Outbox
// yet (Milestone 11) — if this call fails after the caller has already
// persisted a state change, the event is simply lost. See ADR 003.
func (p *Publisher) Publish(ctx context.Context, routingKey string, envelope events.Envelope) error {
	body, err := json.Marshal(envelope)
	if err != nil {
		return err
	}

	return p.channel.PublishWithContext(ctx, ExchangeName, routingKey, false, false, amqp.Publishing{
		ContentType: "application/json",
		MessageId:   envelope.EventID,
		Timestamp:   envelope.OccurredAt,
		Body:        body,
	})
}

// Close closes the underlying channel.
func (p *Publisher) Close() error {
	return p.channel.Close()
}
