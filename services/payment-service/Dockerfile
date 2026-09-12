FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/payment-service ./cmd/server

FROM alpine:3.20
RUN apk add --no-cache ca-certificates
COPY --from=build /out/payment-service /usr/local/bin/payment-service
EXPOSE 9091
ENTRYPOINT ["/usr/local/bin/payment-service"]
