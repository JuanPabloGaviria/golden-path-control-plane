package main

import (
	"context"
	"errors"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/juanpablogaviria/golden-path-control-plane/internal/api"
	"github.com/juanpablogaviria/golden-path-control-plane/internal/app"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	runtime, err := app.Bootstrap(ctx)
	if err != nil {
		panic(err)
	}
	defer func() {
		_ = runtime.Close(context.Background())
	}()

	server := &http.Server{
		Addr:         runtime.Config.HTTPAddr,
		Handler:      api.NewHandler(runtime.Logger, runtime.ControlPlane, runtime.AuthValidator, runtime.Store, runtime.Metrics),
		ReadTimeout:  runtime.Config.HTTPReadTimeout,
		WriteTimeout: runtime.Config.HTTPWriteTimeout,
		IdleTimeout:  runtime.Config.HTTPIdleTimeout,
	}

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), runtime.Config.ShutdownTimeout)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
	}()

	runtime.Logger.Info("api_listening", "addr", runtime.Config.HTTPAddr)
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		runtime.Logger.Error("api_server_failed", "error", err.Error())
		os.Exit(1)
	}

	<-time.After(100 * time.Millisecond)
}
