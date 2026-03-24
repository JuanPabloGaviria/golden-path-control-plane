//go:build integration

package app

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/juanpablogaviria/golden-path-control-plane/internal/config"
	"github.com/juanpablogaviria/golden-path-control-plane/internal/migrations"
	"github.com/juanpablogaviria/golden-path-control-plane/internal/postgres"
)

func TestBootstrapRequiresCompatibleSchema(t *testing.T) {
	databaseURL := os.Getenv("CONTROL_PLANE_INTEGRATION_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("CONTROL_PLANE_INTEGRATION_DATABASE_URL is not set")
	}

	ctx := context.Background()
	pool, err := postgres.NewPool(ctx, testDatabaseConfig(databaseURL))
	if err != nil {
		t.Fatalf("NewPool returned error: %v", err)
	}
	defer pool.Close()

	if _, err := pool.Exec(ctx, `DROP SCHEMA IF EXISTS public CASCADE`); err != nil {
		t.Fatalf("drop public schema: %v", err)
	}
	if _, err := pool.Exec(ctx, `CREATE SCHEMA public`); err != nil {
		t.Fatalf("create public schema: %v", err)
	}

	setRuntimeEnv(t, databaseURL)

	runtime, err := Bootstrap(ctx)
	if runtime != nil {
		t.Fatal("expected Bootstrap to fail against an unmigrated database")
	}
	if !errors.Is(err, migrations.ErrSchemaUninitialized) {
		t.Fatalf("expected ErrSchemaUninitialized, got %v", err)
	}

	if err := migrations.Apply(ctx, pool); err != nil {
		t.Fatalf("Apply returned error: %v", err)
	}

	runtime, err = Bootstrap(ctx)
	if err != nil {
		t.Fatalf("Bootstrap returned error after Apply: %v", err)
	}
	defer func() {
		_ = runtime.Close(context.Background())
	}()
}

func testDatabaseConfig(databaseURL string) config.DatabaseConfig {
	return config.DatabaseConfig{
		URL:             databaseURL,
		MaxOpenConns:    4,
		MinIdleConns:    1,
		MaxConnLifetime: time.Minute,
		MaxConnIdleTime: time.Minute,
	}
}

func setRuntimeEnv(t *testing.T, databaseURL string) {
	t.Helper()

	t.Setenv("APP_ENV", "test")
	t.Setenv("APP_LOG_LEVEL", "INFO")
	t.Setenv("HTTP_ADDR", ":8080")
	t.Setenv("HTTP_READ_TIMEOUT", "10s")
	t.Setenv("HTTP_WRITE_TIMEOUT", "15s")
	t.Setenv("HTTP_IDLE_TIMEOUT", "60s")
	t.Setenv("SHUTDOWN_TIMEOUT", "20s")
	t.Setenv("DATABASE_URL", databaseURL)
	t.Setenv("DATABASE_MAX_OPEN_CONNS", "4")
	t.Setenv("DATABASE_MIN_IDLE_CONNS", "1")
	t.Setenv("DATABASE_MAX_CONN_LIFETIME", "1m")
	t.Setenv("DATABASE_MAX_CONN_IDLE_TIME", "1m")
	t.Setenv("WORKER_POLL_INTERVAL", "2s")
	t.Setenv("WORKER_BATCH_SIZE", "5")
	t.Setenv("JOB_LEASE_DURATION", "30s")
	t.Setenv("JOB_MAX_ATTEMPTS", "5")
	t.Setenv("AUTH_MODE", "hmac")
	t.Setenv("AUTH_AUDIENCE", "golden-path-control-plane")
	t.Setenv("AUTH_ISSUER", "golden-path-local")
	t.Setenv("AUTH_HMAC_SECRET", "12345678901234567890123456789012")
	t.Setenv("AUTH_OIDC_ISSUER_URL", "")
	t.Setenv("AUTH_OIDC_JWKS_URL", "")
	t.Setenv("OTEL_SERVICE_NAME", "golden-path-control-plane")
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "")
	t.Setenv("PROMETHEUS_NAMESPACE", "goldenpath")
}
