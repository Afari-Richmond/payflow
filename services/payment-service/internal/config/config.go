// Package config loads payment-service's runtime configuration from
// environment variables.
package config

import "os"

// Config holds payment-service's runtime configuration.
type Config struct {
	// Port is the TCP port the gRPC server listens on.
	Port string
	// DatabaseURL is payment-service's PostgreSQL connection string.
	DatabaseURL string
	// PaystackSecretKey authenticates calls to the Paystack API. Only
	// payment-service ever holds this.
	PaystackSecretKey string
	// PaystackBaseURL overrides the Paystack API base URL. Empty in
	// production (the real API is used) — only set to point at a local
	// fake server for testing/manual verification.
	PaystackBaseURL string
	// RabbitMQURL is the AMQP connection string used to publish domain
	// events.
	RabbitMQURL string
}

// Load reads configuration from the environment, applying defaults for
// anything unset.
func Load() Config {
	return Config{
		Port:              envOrDefault("PAYMENT_GRPC_PORT", "9091"),
		DatabaseURL:       envOrDefault("PAYMENT_DATABASE_URL", "postgres://payflow:payflow@localhost:5433/payflow_payment?sslmode=disable"),
		PaystackSecretKey: os.Getenv("PAYSTACK_SECRET_KEY"),
		PaystackBaseURL:   os.Getenv("PAYSTACK_BASE_URL"),
		RabbitMQURL:       envOrDefault("RABBITMQ_URL", "amqp://payflow:payflow@localhost:5672/"),
	}
}

func envOrDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
