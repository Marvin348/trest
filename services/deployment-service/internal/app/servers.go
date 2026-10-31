package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"
)

func runServers() error {
	grpcServer, listener, err := newGRPCServer(":50051")
	if err != nil {
		return fmt.Errorf("initialize gRPC server: %w", err)
	}

	defer listener.Close()

	mux := http.NewServeMux()

	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("OK"))
	})

	httpServer := &http.Server{
		Addr:    ":8090",
		Handler: mux,
	}

	errCh := make(chan error, 2)

	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()

		slog.Info("gRPC server starting", "address", ":50051")

		if err := grpcServer.Serve(listener); err != nil {
			errCh <- fmt.Errorf("grpc server: %w", err)
		}

		errCh <- errors.New("grpc server stopped unexpectedly")
	}()

	go func() {
		defer wg.Done()

		slog.Info("Deployment Service starting", "adress", ":8090")

		if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- fmt.Errorf("http server: %w", err)
		}

		errCh <- errors.New("deployment server stopped unexpectedly")
	}()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	var serverError error

	select {
	case <-ctx.Done():
		slog.Info("shutdown signal received")

	case serverError = <-errCh:
		slog.Info("server faild", "error", serverError)
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	httpErr := httpServer.Shutdown(shutdownCtx)

	timeout := time.AfterFunc(10*time.Second, grpcServer.Stop)
	grpcServer.GracefulStop()
	timeout.Stop()

	wg.Wait()

	return errors.Join(serverError, httpErr)
}
