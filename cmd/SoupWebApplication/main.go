package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"SoupWebApplication/internal/app"
	"SoupWebApplication/internal/storage/postgres"
	jobsv1 "SoupWebApplication/pkg/api/jobs/v1"

	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"google.golang.org/grpc"

	_ "github.com/jackc/pgx/v5/stdlib"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		return errors.New("DATABASE_URL is required")
	}

	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return err
	}
	defer db.Close()

	if err := db.PingContext(ctx); err != nil {
		return fmt.Errorf("connect to postgres: %w", err)
	}

	grpcAddr := address("GRPC_ADDR", "127.0.0.1:9090")
	httpAddr := address("HTTP_ADDR", "127.0.0.1:8080")

	listener, err := net.Listen("tcp", grpcAddr)
	if err != nil {
		return fmt.Errorf("listen on gRPC address: %w", err)
	}

	service := app.NewService(postgres.New(db))

	grpcServer := grpc.NewServer()
	jobsv1.RegisterJobServiceServer(grpcServer, service)

	gateway := runtime.NewServeMux()
	if err := jobsv1.RegisterJobServiceHandlerServer(ctx, gateway, service); err != nil {
		listener.Close()
		return fmt.Errorf("register HTTP gateway: %w", err)
	}

	httpServer := &http.Server{
		Addr:    httpAddr,
		Handler: gateway,
	}

	serverErrors := make(chan error, 2)

	go func() {
		serverErrors <- grpcServer.Serve(listener)
	}()

	go func() {
		serverErrors <- httpServer.ListenAndServe()
	}()

	var runErr error

	select {
	case <-ctx.Done():
	case runErr = <-serverErrors:
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	shutdownErr := httpServer.Shutdown(shutdownCtx)
	grpcServer.GracefulStop()

	if runErr != nil && !errors.Is(runErr, http.ErrServerClosed) &&
		!errors.Is(runErr, grpc.ErrServerStopped) {
		return runErr
	}

	return shutdownErr
}

func address(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}

	return fallback
}
