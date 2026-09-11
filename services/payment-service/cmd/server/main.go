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
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/Afari-Richmond/payflow/pkg/correlation"
	paymentv1 "github.com/Afari-Richmond/payflow/proto/payment/v1"
	"github.com/Afari-Richmond/payflow/services/payment-service/internal/application"
	"github.com/Afari-Richmond/payflow/services/payment-service/internal/config"
	"github.com/Afari-Richmond/payflow/services/payment-service/internal/messaging"
	"github.com/Afari-Richmond/payflow/services/payment-service/internal/outbox"
	"github.com/Afari-Richmond/payflow/services/payment-service/internal/provider/paystack"
	"github.com/Afari-Richmond/payflow/services/payment-service/internal/repository"
	"github.com/Afari-Richmond/payflow/services/payment-service/internal/transport/grpcapi"
	"github.com/Afari-Richmond/payflow/services/payment-service/internal/webhook"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	cfg := config.Load()

	if cfg.PaystackSecretKey == "" {
		logger.Error("PAYSTACK_SECRET_KEY is not set")
		os.Exit(1)
	}

	db, err := gorm.Open(postgres.Open(cfg.DatabaseURL), &gorm.Config{})
	if err != nil {
		logger.Error("failed to connect to database", "error", err)
		os.Exit(1)
	}

	var paystackOpts []paystack.Option
	if cfg.PaystackBaseURL != "" {
		paystackOpts = append(paystackOpts, paystack.WithBaseURL(cfg.PaystackBaseURL))
	}
	paymentProvider := paystack.NewClient(cfg.PaystackSecretKey, paystackOpts...)

	amqpConn, err := amqp.Dial(cfg.RabbitMQURL)
	if err != nil {
		logger.Error("failed to connect to RabbitMQ", "error", err)
		os.Exit(1)
	}
	defer amqpConn.Close()

	publisher, err := messaging.NewPublisher(amqpConn)
	if err != nil {
		logger.Error("failed to create event publisher", "error", err)
		os.Exit(1)
	}
	defer publisher.Close()

	repo := repository.NewGormPaymentRepository(db)
	outboxRepo := repository.NewGormOutboxRepository(db)
	paymentService := application.NewPaymentService(repo, paymentProvider)
	webhookService := webhook.NewService(repo, paymentProvider, cfg.PaystackSecretKey, logger)

	outboxWorker := outbox.NewWorker(outboxRepo, publisher, logger)

	lis, err := net.Listen("tcp", ":"+cfg.Port)
	if err != nil {
		logger.Error("failed to listen", "error", err)
		os.Exit(1)
	}

	grpcServer := grpc.NewServer(grpc.ChainUnaryInterceptor(
		correlation.UnaryServerInterceptor(),
		loggingInterceptor(logger),
	))
	paymentv1.RegisterPaymentServiceServer(grpcServer, grpcapi.NewPaymentServer(paymentService, webhookService))

	healthServer := health.NewServer()
	healthpb.RegisterHealthServer(grpcServer, healthServer)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go runDBHealthCheck(ctx, db, healthServer, logger)

	go func() {
		logger.Info("starting outbox worker")
		outboxWorker.Run(ctx)
	}()

	serverErr := make(chan error, 1)
	go func() {
		logger.Info("starting payment-service", "port", cfg.Port)
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
	logger.Info("payment-service stopped")
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
