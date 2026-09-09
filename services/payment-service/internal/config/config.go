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
}

// Load reads configuration from the environment, applying defaults for
// anything unset.
func Load() Config {
	return Config{
		Port:        envOrDefault("PAYMENT_GRPC_PORT", "9091"),
		DatabaseURL: envOrDefault("PAYMENT_DATABASE_URL", "postgres://payflow:payflow@localhost:5433/payflow_payment?sslmode=disable"),
	}
}

func envOrDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
