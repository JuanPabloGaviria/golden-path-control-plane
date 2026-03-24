package main

import (
	"context"
	"errors"
	"os"
	"os/signal"
	"syscall"

	"github.com/juanpablogaviria/golden-path-control-plane/internal/app"
	"github.com/juanpablogaviria/golden-path-control-plane/internal/jobs"
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

	worker := jobs.NewWorker(
		runtime.Logger,
		runtime.ControlPlane,
		runtime.Store,
		runtime.Metrics,
		runtime.Config.Worker.PollInterval,
		runtime.Config.Worker.BatchSize,
		runtime.Config.Worker.Lease,
	)

	runtime.Logger.Info("worker_started")
	if err := worker.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
		runtime.Logger.Error("worker_failed", "error", err.Error())
		os.Exit(1)
	}
}
