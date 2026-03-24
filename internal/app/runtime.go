package app

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/juanpablogaviria/golden-path-control-plane/internal/auth"
	"github.com/juanpablogaviria/golden-path-control-plane/internal/config"
	"github.com/juanpablogaviria/golden-path-control-plane/internal/migrations"
	"github.com/juanpablogaviria/golden-path-control-plane/internal/observability"
	"github.com/juanpablogaviria/golden-path-control-plane/internal/postgres"
)

type Runtime struct {
	Config        config.Config
	Logger        *slog.Logger
	Metrics       *observability.Metrics
	Store         *postgres.Store
	ControlPlane  *ControlPlane
	AuthValidator auth.Validator
	traceShutdown func(context.Context) error
}

func Bootstrap(ctx context.Context) (*Runtime, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, err
	}

	logger, err := observability.NewLogger(cfg.AppLogLevel)
	if err != nil {
		return nil, err
	}

	metrics, err := observability.NewMetrics(cfg.Observability.Namespace)
	if err != nil {
		return nil, fmt.Errorf("bootstrap: create metrics: %w", err)
	}

	traceShutdown, err := observability.SetupTracing(ctx, cfg.Observability)
	if err != nil {
		return nil, fmt.Errorf("bootstrap: setup tracing: %w", err)
	}

	pool, err := postgres.NewPool(ctx, cfg.Database)
	if err != nil {
		return nil, err
	}

	if err := migrations.EnsureCompatible(ctx, pool); err != nil {
		pool.Close()
		return nil, fmt.Errorf("bootstrap: schema compatibility: %w", err)
	}

	validator, err := auth.NewValidator(ctx, cfg.Auth)
	if err != nil {
		pool.Close()
		return nil, fmt.Errorf("bootstrap: auth validator: %w", err)
	}

	store := postgres.NewStore(pool)
	runtime := &Runtime{
		Config:        cfg,
		Logger:        logger,
		Metrics:       metrics,
		Store:         store,
		ControlPlane:  NewControlPlane(store, cfg.Worker.MaxAttempts),
		AuthValidator: validator,
		traceShutdown: traceShutdown,
	}

	logger.Info("runtime_bootstrapped", "config", runtime.Config.Redacted())
	return runtime, nil
}

func (r *Runtime) Close(ctx context.Context) error {
	if r.Store != nil && r.Store.Pool() != nil {
		r.Store.Pool().Close()
	}

	if r.traceShutdown != nil {
		if err := r.traceShutdown(ctx); err != nil {
			return fmt.Errorf("runtime: shutdown tracing: %w", err)
		}
	}

	return nil
}
