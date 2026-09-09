// Package config loads api-gateway's runtime configuration from
// environment variables.
package config

import "os"

// Config holds api-gateway's runtime configuration.
type Config struct {
	// Port is the TCP port the HTTP server listens on.
	Port string
	// OrderServiceAddr is order-service's gRPC address.
	OrderServiceAddr string
}

// Load reads configuration from the environment, applying defaults for
// anything unset.
func Load() Config {
	return Config{
		Port:             envOrDefault("GATEWAY_PORT", "8080"),
		OrderServiceAddr: envOrDefault("ORDER_SERVICE_ADDR", "localhost:9090"),
	}
}

func envOrDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
