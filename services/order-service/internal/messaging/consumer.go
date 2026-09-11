// Package messaging is order-service's RabbitMQ consume side.
package messaging

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"

	amqp "github.com/rabbitmq/amqp091-go"

	"github.com/Afari-Richmond/payflow/pkg/events"
)

// ExchangeName must match the producer's (payment-service) exchange —
// see payment-service/internal/messaging.ExchangeName.
const ExchangeName = "payflow.events"

// QueueName is order-service's durable queue for payment events.
const QueueName = "order-service.payment-events"

// dlxExchangeName and deadLetterQueueName hold permanently-failed
// messages for inspection, rather than losing them. See ADR 003.
const (
	dlxExchangeName     = "payflow.events.dlx"
	deadLetterQueueName = "order-service.payment-events.dlq"
)

// Handler processes one event. Returning an *events.PermanentError
// dead-letters the message (retrying can never succeed — malformed
// data, unknown aggregate); any other error requeues it (transient —
// worth retrying); nil acknowledges it.
type Handler func(ctx context.Context, envelope events.Envelope) error

// Consumer consumes payment domain events from RabbitMQ.
type Consumer struct {
	channel *amqp.Channel
	logger  *slog.Logger
}

// NewConsumer declares the exchange, the dead-letter exchange/queue,
// and order-service's own queue (bound to the exchange for
// "payment.*" routing keys, with dead-lettering configured), then
// returns a ready-to-use Consumer.
func NewConsumer(conn *amqp.Connection, logger *slog.Logger) (*Consumer, error) {
	ch, err := conn.Channel()
	if err != nil {
		return nil, err
	}

	if err := ch.ExchangeDeclare(ExchangeName, "topic", true, false, false, false, nil); err != nil {
		ch.Close()
		return nil, err
	}
	if err := ch.ExchangeDeclare(dlxExchangeName, "fanout", true, false, false, false, nil); err != nil {
		ch.Close()
		return nil, err
	}

	dlq, err := ch.QueueDeclare(deadLetterQueueName, true, false, false, false, nil)
	if err != nil {
		ch.Close()
		return nil, err
	}
	if err := ch.QueueBind(dlq.Name, "", dlxExchangeName, false, nil); err != nil {
		ch.Close()
		return nil, err
	}

	queue, err := ch.QueueDeclare(QueueName, true, false, false, false, amqp.Table{
		"x-dead-letter-exchange": dlxExchangeName,
	})
	if err != nil {
		ch.Close()
		return nil, err
	}
	if err := ch.QueueBind(queue.Name, "payment.*", ExchangeName, false, nil); err != nil {
		ch.Close()
		return nil, err
	}

	// Process (and ack) one message at a time — simple, predictable
	// backpressure; not a throughput optimization target yet.
	if err := ch.Qos(1, 0, false); err != nil {
		ch.Close()
		return nil, err
	}

	return &Consumer{channel: ch, logger: logger}, nil
}

// Run consumes messages until ctx is cancelled or the channel closes.
func (c *Consumer) Run(ctx context.Context, handle Handler) error {
	msgs, err := c.channel.Consume(QueueName, "", false, false, false, false, nil)
	if err != nil {
		return err
	}

	for {
		select {
		case <-ctx.Done():
			return nil
		case msg, ok := <-msgs:
			if !ok {
				return nil
			}
			c.process(ctx, msg, handle)
		}
	}
}

func (c *Consumer) process(ctx context.Context, msg amqp.Delivery, handle Handler) {
	var envelope events.Envelope
	if err := json.Unmarshal(msg.Body, &envelope); err != nil {
		c.logger.Error("malformed event envelope, dead-lettering", "error", err)
		msg.Nack(false, false)
		return
	}

	err := handle(ctx, envelope)
	if err == nil {
		c.logger.Info("event processed",
			"event_id", envelope.EventID, "event_type", envelope.EventType,
			"correlation_id", envelope.CorrelationID)
		msg.Ack(false)
		return
	}

	if _, ok := errors.AsType[*events.PermanentError](err); ok {
		c.logger.Error("permanent processing failure, dead-lettering",
			"event_id", envelope.EventID, "event_type", envelope.EventType,
			"correlation_id", envelope.CorrelationID, "error", err)
		msg.Nack(false, false)
		return
	}

	c.logger.Warn("transient processing failure, requeueing",
		"event_id", envelope.EventID, "event_type", envelope.EventType,
		"correlation_id", envelope.CorrelationID, "error", err)
	msg.Nack(false, true)
}

// Close closes the underlying channel.
func (c *Consumer) Close() error {
	return c.channel.Close()
}
