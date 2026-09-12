# PayFlow

Event-driven payment microservices built with Go and Paystack — a
learning-and-portfolio project demonstrating backend, distributed
systems, and payment-domain engineering: gRPC, PostgreSQL, RabbitMQ,
idempotency, and the transactional outbox pattern.

## Problem

Payment integrations sit at the intersection of several hard
distributed-systems problems: a synchronous call to a third-party
provider that must never block the customer for too long, an
asynchronous webhook whose delivery isn't guaranteed exactly once, and
two independent databases (orders, payments) that must end up
consistent without ever sharing a transaction. PayFlow builds a small
but real system around exactly that problem — creating an order,
paying for it via Paystack, and reliably reflecting that payment back
onto the order — using the same patterns (idempotent consumers,
transactional outbox, correlation-based observability) that production
payment systems rely on.

## Architecture

```
Client
  │ HTTP / JSON
  ▼
API Gateway (Go + Gin)
  │ gRPC                              │ gRPC (webhook forwarding)
  ▼                                   ▼
Order Service ───────gRPC────────▶ Payment Service ───HTTPS───▶ Paystack
     ▲                                   │
     │              RabbitMQ             │
     └───── payment.succeeded/failed ────┘
              (payflow.events exchange)

PostgreSQL — one database per service (payflow_order, payflow_payment)
RabbitMQ   — async payment-event delivery
Docker + Docker Compose — local orchestration of all five containers
```

- **api-gateway** — the only service exposed externally. HTTP/JSON in,
  gRPC out. No business logic or database access of its own.
- **order-service** — owns Order data and lifecycle. Consumes payment
  events asynchronously to mark orders paid.
- **payment-service** — owns Payment data, the Paystack integration,
  and webhook verification. The only service holding Paystack
  credentials.

## Stack

Go 1.26, Gin, gRPC + Protocol Buffers (via `buf`), GORM over PostgreSQL
16, RabbitMQ 3 (topic exchange + dead-lettering), Paystack, Docker +
Docker Compose, `golang-migrate` for schema migrations, `swaggo` +
Scalar for API docs.

## Request Flow — Create Order

```
Client → POST /api/v1/orders → api-gateway
       → gRPC CreateOrder → order-service
       → INSERT into payflow_order.orders (status PENDING_PAYMENT)
       → 201 Created back to client
```

## Payment Flow

```
Client → gRPC CreatePayment → payment-service
       → INSERT into payflow_payment.payments (status PENDING)
       → POST /transaction/initialize → Paystack
       → UPDATE payment (status INITIALIZED, provider_reference)
       → returns { payment, authorization_url }
Client → redirected to authorization_url, completes payment on Paystack
```

The payment's own UUID is used as the Paystack transaction reference —
deliberately, so the webhook step below needs no separate lookup
table.

## Event Flow — Webhook to Paid Order

```
Paystack → POST /webhooks/paystack (raw body + X-Paystack-Signature)
         → api-gateway forwards raw bytes + signature, untouched,
           via gRPC HandleWebhook → payment-service
         → verify HMAC-SHA512 signature
         → call GET /transaction/verify/:reference on Paystack
           (webhook body's own status claim is never trusted)
         → one DB transaction: mark event processed + update payment
           status + insert an outbox_events row
         → 200 OK back to Paystack

payment-service outbox worker (poll every 2s, FOR UPDATE SKIP LOCKED)
         → publishes the claimed row to RabbitMQ (payflow.events,
           routing key payment.succeeded / payment.failed)
         → marks it published_at on success

order-service consumer (durable queue, manual ack, Qos(1))
         → looks up the order, marks it PAID
         → ack (or nack → dead-letter on a permanent error)
```

api-gateway never inspects the webhook payload and never sees the
Paystack secret key.

## Database

Database-per-service: one Postgres container, two databases
(`payflow_order`, `payflow_payment`), each owned exclusively by its
service. Cross-service references (`payments.order_id`) are indexed but
are **not** real foreign keys — they can't be, across two databases —
so consistency between them is enforced by the event flow above, not
by the schema.

| Service | Tables |
|---|---|
| order-service | `orders`, `processed_events` (idempotent consumer) |
| payment-service | `payments`, `processed_webhook_events` (idempotent webhook), `outbox_events` (transactional outbox) |

Schema changes go through versioned `golang-migrate` SQL files, never
ORM auto-migrate.

## Idempotency

Both the webhook path and the event-consumer path can legitimately see
the same delivery more than once (Paystack retries undelivered
webhooks; RabbitMQ is at-least-once). Both are protected the same way:
an atomic "insert a processed-marker row, then update the target row"
transaction backed by a real Postgres unique constraint — not a
"check status, then write" read-then-write, which is a real race under
concurrent delivery. A cheap status-check fast path (`payment.Status ==
SUCCESS` / `order.Status == PAID`) sits in front of it to skip the
DB transaction entirely for the common very-late-redelivery case.
Proven under real concurrency: 20 simultaneous identical webhook
deliveries produce exactly one published event; 10 simultaneous workers
racing to claim the same row have exactly one winner.

## Transactional Outbox

`payment-service` never publishes to RabbitMQ directly from the
request path. The webhook handler writes the payment's new status and
an `outbox_events` row in **one** database transaction — so the event
and the state change either both commit or neither does, by
construction. A separate background worker polls
`outbox_events` (`FOR UPDATE SKIP LOCKED`, so multiple worker instances
can't double-publish the same row) and publishes to RabbitMQ,
retrying indefinitely on failure. This closes the dual-write gap that
exists whenever a database write and a message-broker publish aren't
atomic.

**Known limitation:** `amqp091-go` doesn't reconnect a dead connection
automatically. If RabbitMQ goes down and comes back, the outbox worker
and the order-service consumer both need their host service restarted
to pick up a fresh connection — the event itself is never lost (it sits
durably in `outbox_events` until published), but recovery isn't fully
automatic.

## Observability

A correlation ID is generated (or extracted from an inbound header) at
api-gateway and threaded through every hop of a request: HTTP →
outbound gRPC metadata → structured logs on every service → the
RabbitMQ event envelope → the consumer's logs. One ID is traceable
across all three services' logs for a single request.

- `GET /health` — liveness only, always `{"status":"ok"}` if the
  process is up.
- `GET /ready` — readiness: calls each dependency's standard gRPC
  health check and reports per-dependency status. Both order-service
  and payment-service run a background loop pinging their own database
  every 5s and report `NOT_SERVING` if it's unreachable, so `/ready`
  reflects real dependency state, not a static report.

## Configuration

All configuration is environment variables — see `.env.example` for
the full list with descriptions. Copy it to `.env` and fill in real
values (`.env` is git-ignored):

```bash
cp .env.example .env
```

| Variable | Used by | Notes |
|---|---|---|
| `GATEWAY_PORT` | api-gateway | default `8080` |
| `ORDER_SERVICE_ADDR` / `PAYMENT_SERVICE_ADDR` | api-gateway | gRPC targets |
| `ORDER_GRPC_PORT` | order-service | default `9090` |
| `ORDER_DATABASE_URL` | order-service | Postgres DSN |
| `PAYMENT_GRPC_PORT` | payment-service | default `9091` |
| `PAYMENT_DATABASE_URL` | payment-service | Postgres DSN |
| `PAYSTACK_SECRET_KEY` | payment-service | **required** — fails fast at startup if unset |
| `PAYSTACK_BASE_URL` | payment-service | leave empty in normal use; only set to point at a local fake server |
| `RABBITMQ_URL` | order-service, payment-service | AMQP URL |

## Local Setup

Requires Go 1.26+, Docker Desktop, and the `migrate` CLI
(`go install -tags postgres github.com/golang-migrate/migrate/v4/cmd/migrate`).

```bash
git clone <this-repo> && cd payflow
cp .env.example .env               # fill in PAYSTACK_SECRET_KEY (see below)

make docker-up                     # postgres, rabbitmq, and all three services
make migrate-order-up
make migrate-payment-up

curl localhost:8080/health
curl localhost:8080/ready
```

`make docker-up` builds and starts all five containers
(`postgres`, `rabbitmq`, `order-service`, `payment-service`,
`api-gateway`) via `deployments/docker-compose.yml`. To run services
directly on the host instead (faster iteration, same Postgres/RabbitMQ
containers):

```bash
make build && make vet && make test
go run ./services/order-service/cmd/server &
go run ./services/payment-service/cmd/server &
go run ./services/api-gateway/cmd/api &
```

Interactive API docs (generated from code via `swaggo`, rendered with
Scalar): `http://localhost:8080/docs`.

## API Examples

Create an order:

```bash
curl -X POST localhost:8080/api/v1/orders \
  -H "Content-Type: application/json" \
  -d '{"email":"buyer@example.com","amount":500000,"currency":"GHS"}'
```

`amount` is always an integer minor-currency-unit value (e.g. `500000`
= GHS 5,000.00) — never a float.

## gRPC

Both internal services expose gRPC only (no external HTTP). Reflection
is intentionally not enabled — inspect with `grpcurl` using the local
`.proto` files:

```bash
grpcurl -plaintext -proto proto/order/v1/order.proto -import-path proto \
  -d '{"email":"buyer@example.com","amount_minor":500000,"currency":"GHS"}' \
  localhost:9090 order.v1.OrderService/CreateOrder

grpcurl -plaintext -proto proto/payment/v1/payment.proto -import-path proto \
  -d '{"order_id":"<order-id>","email":"buyer@example.com","amount_minor":500000,"currency":"GHS"}' \
  localhost:9091 payment.v1.PaymentService/CreatePayment
```

Both services also expose the standard gRPC health-checking protocol
(`grpc.health.v1.Health/Check`), backed by a live database ping.

## Paystack Test Setup

1. Create a free Paystack account and switch to **test mode**.
2. Dashboard → Settings → API Keys & Webhooks → copy the **test secret
   key** (`sk_test_...`) into `PAYSTACK_SECRET_KEY` in `.env`.
3. Leave `PAYSTACK_BASE_URL` empty — the real Paystack API is the
   default. Only set it if you want to point at a local fake server for
   manual testing without hitting the network at all.

### Testing without real credentials

`scripts/fake-paystack` is a dev-only stand-in implementing just
`/transaction/initialize` and `/transaction/verify/:reference` — enough
to exercise the full payment flow without a real Paystack account.

```bash
make fake-paystack                              # listens on :4123
```

Point payment-service at it via `PAYSTACK_BASE_URL` in `.env`
(`PAYSTACK_SECRET_KEY` can stay a placeholder — the fake server never
checks it):

```
PAYSTACK_BASE_URL=http://localhost:4123
```

Running payment-service via Docker Compose instead of directly on the
host? Use `http://host.docker.internal:4123` so the container can
reach the fake server on the host, then recreate the container to pick
up the change: `make docker-up`.

Never point `PAYSTACK_BASE_URL` at this outside local development — it
does no signature checking of its own and isn't meant to hold real
transaction data.

## Webhook Setup

Paystack signs every webhook body with HMAC-**SHA512** (not SHA256)
over the raw request bytes, sent in the `x-paystack-signature` header.
To register a webhook against a local instance, expose
`api-gateway`'s port with a tunnel (e.g. `ngrok http 8080`) and set the
tunnel's `/webhooks/paystack` URL in the Paystack dashboard. To test
signature verification manually without Paystack:

```bash
BODY='{"event":"charge.success","data":{"reference":"<payment-id>"}}'
SIG=$(printf '%s' "$BODY" | openssl dgst -sha512 -hmac "$PAYSTACK_SECRET_KEY" | sed 's/^.* //')
curl -i -X POST localhost:8080/webhooks/paystack \
  -H "Content-Type: application/json" \
  -H "x-paystack-signature: $SIG" \
  -d "$BODY"
```

Only `charge.success` exists as a payment webhook event — there is no
`charge.failed` delivery; failed attempts are only visible via
`VerifyTransaction`.

## RabbitMQ

Management UI: `http://localhost:15672` (`payflow` / `payflow`).
Topic exchange `payflow.events`; `order-service` binds a durable queue
to `payment.*`, with a dead-letter exchange/queue for permanently
unprocessable messages. RabbitMQ and its data volume are managed
entirely through `deployments/docker-compose.yml` — no manual queue
setup required.

## Repository Layout

```
services/            api-gateway, order-service, payment-service (independent Go modules)
proto/                versioned gRPC service contracts (its own Go module)
pkg/correlation/      shared correlation-ID context/interceptor package
pkg/events/           shared event envelope package
deployments/          docker-compose.yml, Postgres init scripts
```

## Testing

```bash
make docker-up          # real Postgres + RabbitMQ (needed by integration tests)
make build && make vet
go test ./services/... ./pkg/... -race
```

Tests run against real infrastructure where it matters (repository and
messaging integration tests skip gracefully if Postgres/RabbitMQ are
unreachable) rather than mocking the database or broker.

## Commit Convention

Commits follow [Conventional Commits](https://www.conventionalcommits.org/en/v1.0.0/),
enforced via a commit-msg hook (`npm install` to activate it locally).
