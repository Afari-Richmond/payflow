// Package config loads order-service's runtime configuration from
// environment variables.
package config

import "os"

// Config holds order-service's runtime configuration.
type Config struct {
	// Port is the TCP port the gRPC server listens on.
	Port string
	// DatabaseURL is order-service's PostgreSQL connection string.
	DatabaseURL string
	// RabbitMQURL is the AMQP connection string used to consume domain
	// events.
	RabbitMQURL string
}

// Load reads configuration from the environment, applying defaults for
// anything unset.
func Load() Config {
	return Config{
		Port:        envOrDefault("ORDER_GRPC_PORT", "9090"),
		DatabaseURL: envOrDefault("ORDER_DATABASE_URL", "postgres://payflow:payflow@localhost:5433/payflow_order?sslmode=disable"),
		RabbitMQURL: envOrDefault("RABBITMQ_URL", "amqp://payflow:payflow@localhost:5672/"),
	}
}

func envOrDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
