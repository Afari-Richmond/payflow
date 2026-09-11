// Command server runs PayFlow's order-service: a gRPC server, internal
// only, never exposed directly to external clients.
package main

import (
	"context"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/Afari-Richmond/payflow/pkg/correlation"
	orderv1 "github.com/Afari-Richmond/payflow/proto/order/v1"
	"github.com/Afari-Richmond/payflow/services/order-service/internal/application"
	"github.com/Afari-Richmond/payflow/services/order-service/internal/config"
	"github.com/Afari-Richmond/payflow/services/order-service/internal/messaging"
	"github.com/Afari-Richmond/payflow/services/order-service/internal/repository"
	"github.com/Afari-Richmond/payflow/services/order-service/internal/transport/grpcapi"
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

	repo := repository.NewGormOrderRepository(db)
	orderService := application.NewOrderService(repo)

	amqpConn, err := amqp.Dial(cfg.RabbitMQURL)
	if err != nil {
		logger.Error("failed to connect to RabbitMQ", "error", err)
		os.Exit(1)
	}
	defer amqpConn.Close()

	consumer, err := messaging.NewConsumer(amqpConn, logger)
	if err != nil {
		logger.Error("failed to create event consumer", "error", err)
		os.Exit(1)
	}
	defer consumer.Close()

	lis, err := net.Listen("tcp", ":"+cfg.Port)
	if err != nil {
		logger.Error("failed to listen", "error", err)
		os.Exit(1)
	}

	grpcServer := grpc.NewServer(grpc.ChainUnaryInterceptor(
		correlation.UnaryServerInterceptor(),
		loggingInterceptor(logger),
	))
	orderv1.RegisterOrderServiceServer(grpcServer, grpcapi.NewOrderServer(orderService))

	healthServer := health.NewServer()
	healthpb.RegisterHealthServer(grpcServer, healthServer)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go runDBHealthCheck(ctx, db, healthServer, logger)

	go func() {
		logger.Info("starting payment-events consumer")
		if err := consumer.Run(ctx, orderService.HandlePaymentEvent); err != nil {
			logger.Error("consumer stopped unexpectedly", "error", err)
		}
	}()

	serverErr := make(chan error, 1)
	go func() {
		logger.Info("starting order-service", "port", cfg.Port)
		if err := grpcServer.Serve(lis); err != nil {
			serverErr <- err
		}
		close(serverErr)
	}()

	select {
	case err := <-serverErr:
		logger.Error("server failed", "error", err)
		os.Exit(1)
	case <-ctx.Done():
		logger.Info("shutdown signal received")
	}

	grpcServer.GracefulStop()
	logger.Info("order-service stopped")
}

// loggingInterceptor logs the start and end of every unary RPC,
// tagged with the request's correlation ID so a single request can be
// traced across this service's logs.
func loggingInterceptor(logger *slog.Logger) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		id := correlation.FromContext(ctx)
		logger.Info("grpc request started", "method", info.FullMethod, "correlation_id", id)
		resp, err := handler(ctx, req)
		if err != nil {
			logger.Error("grpc request failed", "method", info.FullMethod, "correlation_id", id, "error", err)
		} else {
			logger.Info("grpc request completed", "method", info.FullMethod, "correlation_id", id)
		}
		return resp, err
	}
}

// runDBHealthCheck periodically pings the database and reflects
// reachability in the gRPC health server, so downstream health checks
// (see api-gateway's /ready) mean something rather than reporting a
// static SERVING status regardless of actual DB connectivity.
func runDBHealthCheck(ctx context.Context, db *gorm.DB, healthServer *health.Server, logger *slog.Logger) {
	const interval = 5 * time.Second
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	check := func() {
		sqlDB, err := db.DB()
		if err != nil {
			logger.Error("health check: failed to get underlying sql.DB", "error", err)
			healthServer.SetServingStatus("", healthpb.HealthCheckResponse_NOT_SERVING)
			return
		}
		if err := sqlDB.PingContext(ctx); err != nil {
			logger.Error("health check: database unreachable", "error", err)
			healthServer.SetServingStatus("", healthpb.HealthCheckResponse_NOT_SERVING)
			return
		}
		healthServer.SetServingStatus("", healthpb.HealthCheckResponse_SERVING)
	}

	check()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			check()
		}
	}
}
