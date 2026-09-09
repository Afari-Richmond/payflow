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

	"google.golang.org/grpc"

	orderv1 "github.com/Afari-Richmond/payflow/proto/order/v1"
	"github.com/Afari-Richmond/payflow/services/order-service/internal/config"
	"github.com/Afari-Richmond/payflow/services/order-service/internal/transport/grpcapi"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	cfg := config.Load()

	lis, err := net.Listen("tcp", ":"+cfg.Port)
	if err != nil {
		logger.Error("failed to listen", "error", err)
		os.Exit(1)
	}

	grpcServer := grpc.NewServer()
	orderv1.RegisterOrderServiceServer(grpcServer, grpcapi.NewOrderServer())

	serverErr := make(chan error, 1)
	go func() {
		logger.Info("starting order-service", "port", cfg.Port)
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
	logger.Info("order-service stopped")
}
