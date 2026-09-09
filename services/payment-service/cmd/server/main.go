// Command server runs PayFlow's payment-service: a gRPC server,
// internal only, never exposed directly to external clients. Owns all
// Paystack integration (added in Milestone 7) — no other service ever
// holds Paystack credentials.
package main

import (
	"context"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"syscall"

	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	paymentv1 "github.com/Afari-Richmond/payflow/proto/payment/v1"
	"github.com/Afari-Richmond/payflow/services/payment-service/internal/application"
	"github.com/Afari-Richmond/payflow/services/payment-service/internal/config"
	"github.com/Afari-Richmond/payflow/services/payment-service/internal/repository"
	"github.com/Afari-Richmond/payflow/services/payment-service/internal/transport/grpcapi"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	cfg := config.Load()

	db, err := gorm.Open(postgres.Open(cfg.DatabaseURL), &gorm.Config{})
	if err != nil {
		logger.Error("failed to connect to database", "error", err)
		os.Exit(1)
	}

	repo := repository.NewGormPaymentRepository(db)
	paymentService := application.NewPaymentService(repo)

	lis, err := net.Listen("tcp", ":"+cfg.Port)
	if err != nil {
		logger.Error("failed to listen", "error", err)
		os.Exit(1)
	}

	grpcServer := grpc.NewServer()
	paymentv1.RegisterPaymentServiceServer(grpcServer, grpcapi.NewPaymentServer(paymentService))

	healthServer := health.NewServer()
	healthServer.SetServingStatus("", healthpb.HealthCheckResponse_SERVING)
	healthpb.RegisterHealthServer(grpcServer, healthServer)

	serverErr := make(chan error, 1)
	go func() {
		logger.Info("starting payment-service", "port", cfg.Port)
		if err := grpcServer.Serve(lis); err != nil {
			serverErr <- err
		}
		close(serverErr)
	}()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	select {
	case err := <-serverErr:
		logger.Error("server failed", "error", err)
		os.Exit(1)
	case <-ctx.Done():
		logger.Info("shutdown signal received")
	}

	grpcServer.GracefulStop()
	logger.Info("payment-service stopped")
}
