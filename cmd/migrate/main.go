package main

import (
	"context"
	"fmt"
	"os"

	"github.com/juanpablogaviria/golden-path-control-plane/internal/config"
	"github.com/juanpablogaviria/golden-path-control-plane/internal/migrations"
	"github.com/juanpablogaviria/golden-path-control-plane/internal/observability"
	"github.com/juanpablogaviria/golden-path-control-plane/internal/postgres"
)

func main() {
	ctx := context.Background()

	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}

	logger, err := observability.NewLogger(cfg.AppLogLevel)
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}

	pool, err := postgres.NewPool(ctx, cfg.Database)
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}
	defer pool.Close()

	if err := migrations.Apply(ctx, pool); err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}

	logger.Info("migrations_applied", "current_version", migrations.CurrentVersion())
}
