// Command api runs PayFlow's api-gateway: the single external-facing
// HTTP entrypoint into the system.
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Afari-Richmond/payflow/services/api-gateway/internal/client"
	"github.com/Afari-Richmond/payflow/services/api-gateway/internal/config"
	"github.com/Afari-Richmond/payflow/services/api-gateway/internal/transport/httpapi"
)

// @title        PayFlow API Gateway
// @version      0.1
// @description  External-facing HTTP API for PayFlow. Translates client requests into internal gRPC calls.
// @BasePath     /
func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	cfg := config.Load()

	orderClient, err := client.NewOrderClient(cfg.OrderServiceAddr)
	if err != nil {
		logger.Error("failed to create order-service client", "error", err)
		os.Exit(1)
	}
	defer orderClient.Close()

	paymentClient, err := client.NewPaymentClient(cfg.PaymentServiceAddr)
	if err != nil {
		logger.Error("failed to create payment-service client", "error", err)
		os.Exit(1)
	}
	defer paymentClient.Close()

	readiness := client.NewReadiness(orderClient, paymentClient)

	router := httpapi.NewRouter(logger, orderClient, paymentClient, readiness)

	srv := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           router,
		ReadHeaderTimeout: 5 * time.Second,
	}

	serverErr := make(chan error, 1)
	go func() {
		logger.Info("starting api-gateway", "port", cfg.Port)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErr <- err
		}
		close(serverErr)
	}()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	select {
	case err := <-serverErr:
		logger.Error("server failed to start", "error", err)
		os.Exit(1)
	case <-ctx.Done():
		logger.Info("shutdown signal received")
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		logger.Error("graceful shutdown failed", "error", err)
		os.Exit(1)
	}

	logger.Info("api-gateway stopped")
}
