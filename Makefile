.PHONY: build test fmt vet tidy docker-up docker-down migrate-order-up migrate-order-down migrate-payment-up migrate-payment-down

SERVICES := ./services/api-gateway/... ./services/order-service/... ./services/payment-service/...

ORDER_DB_URL := postgres://payflow:payflow@localhost:5433/payflow_order?sslmode=disable
PAYMENT_DB_URL := postgres://payflow:payflow@localhost:5433/payflow_payment?sslmode=disable

build:
	go build $(SERVICES)

test:
	go test $(SERVICES)

fmt:
	gofmt -l -w .

vet:
	go vet $(SERVICES)

tidy:
	go work sync

docker-up:
	docker compose -f deployments/docker-compose.yml up -d

docker-down:
	docker compose -f deployments/docker-compose.yml down

migrate-order-up:
	migrate -database "$(ORDER_DB_URL)" -path services/order-service/migrations up

migrate-order-down:
	migrate -database "$(ORDER_DB_URL)" -path services/order-service/migrations down

migrate-payment-up:
	migrate -database "$(PAYMENT_DB_URL)" -path services/payment-service/migrations up

migrate-payment-down:
	migrate -database "$(PAYMENT_DB_URL)" -path services/payment-service/migrations down
