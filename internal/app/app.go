package app

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"time"
)

func Run(ctx context.Context, log *slog.Logger, srv *http.Server) (func(context.Context) error, error) {
	const methodCtx = "app.Run"
	errCh := make(chan error, 1)
	go func() {
		log.Info("http server started", "addr", srv.Addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- err
		}
	}()
	select {
	case err := <-errCh:
		return nil, fmt.Errorf("%s: %w", methodCtx, err)
	case <-time.After(100 * time.Millisecond):
	}
	return func(ctx context.Context) error {
		const methodCtx = "app.Shutdown"
		if err := srv.Shutdown(ctx); err != nil {
			return fmt.Errorf("%s: %w", methodCtx, err)
		}
		return nil
	}, nil
}
