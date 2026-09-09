.PHONY: build test fmt vet tidy

SERVICES := ./services/api-gateway/... ./services/order-service/... ./services/payment-service/...

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
