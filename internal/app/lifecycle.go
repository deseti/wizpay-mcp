package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"time"
)

const shutdownTimeout = 10 * time.Second

type lifecycleServer interface {
	Serve(net.Listener) error
	Shutdown(context.Context) error
}

func serveUntilCanceled(ctx context.Context, server lifecycleServer, listener net.Listener, logger *slog.Logger) error {
	serveResult := make(chan error, 1)
	go func() {
		serveResult <- server.Serve(listener)
	}()

	select {
	case err := <-serveResult:
		if err == nil || errors.Is(err, http.ErrServerClosed) {
			logger.Info("server_shutdown", "reason", "server_closed")
			return nil
		}
		return fmt.Errorf("serve HTTP: %w", err)
	case <-ctx.Done():
		shutdownContext, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancel()

		if err := server.Shutdown(shutdownContext); err != nil {
			return fmt.Errorf("graceful HTTP shutdown: %w", err)
		}

		err := <-serveResult
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			return fmt.Errorf("serve HTTP after shutdown: %w", err)
		}

		logger.Info("server_shutdown", "reason", "context_cancelled")
		return nil
	}
}
