# PayFlow

Event-driven payment microservices built with Go and Paystack —
demonstrating gRPC, PostgreSQL, RabbitMQ, idempotency, and the
transactional outbox pattern.

**Status: Foundation only.** No services are implemented yet.

## Planned Architecture

```
Client → API Gateway (Gin) → Order Service ─gRPC─→ Payment Service ─HTTPS─→ Paystack
                                                          │
                                                     RabbitMQ
                                                          │
                                                  Order Service (consumer)
```

- **api-gateway** — the only service exposed externally. HTTP/JSON in,
  gRPC out. No business logic or database access.
- **order-service** — owns Order data and lifecycle. Calls Payment
  Service synchronously to start a payment; consumes payment events
  asynchronously to mark orders paid.
- **payment-service** — owns Payment data, Paystack integration, and
  webhook processing. The only service holding Paystack credentials.

## Stack (Planned)

Go, Gin, gRPC + Protocol Buffers, PostgreSQL, RabbitMQ, Paystack, Docker
+ Docker Compose.

## Repository Layout

```
services/   # api-gateway, order-service, payment-service (Go modules)
proto/      # versioned gRPC service contracts
```

## Local Development

Not yet available — no service has a runnable entrypoint yet.

## Commit Convention

Commits follow [Conventional Commits](https://www.conventionalcommits.org/en/v1.0.0/),
enforced via a commit-msg hook (`npm install` to activate it locally).
