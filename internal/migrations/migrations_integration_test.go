//go:build integration

package migrations

import (
	"context"
	"errors"
	"os"
	"reflect"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/juanpablogaviria/golden-path-control-plane/internal/config"
	"github.com/juanpablogaviria/golden-path-control-plane/internal/postgres"
)

func TestApplyIsIdempotent(t *testing.T) {
	ctx := context.Background()
	pool := integrationPool(t, ctx)
	resetPublicSchema(t, ctx, pool)

	if err := Apply(ctx, pool); err != nil {
		t.Fatalf("Apply returned error: %v", err)
	}

	if err := Apply(ctx, pool); err != nil {
		t.Fatalf("second Apply returned error: %v", err)
	}

	applied, err := AppliedVersions(ctx, pool)
	if err != nil {
		t.Fatalf("AppliedVersions returned error: %v", err)
	}

	if !reflect.DeepEqual(applied, Versions()) {
		t.Fatalf("expected applied versions %v, got %v", Versions(), applied)
	}
}

func TestEnsureCompatibleRejectsMissingSchema(t *testing.T) {
	ctx := context.Background()
	pool := integrationPool(t, ctx)
	resetPublicSchema(t, ctx, pool)

	err := EnsureCompatible(ctx, pool)
	if !errors.Is(err, ErrSchemaUninitialized) {
		t.Fatalf("expected ErrSchemaUninitialized, got %v", err)
	}
}

func TestEnsureCompatibleAcceptsCurrentSchema(t *testing.T) {
	ctx := context.Background()
	pool := integrationPool(t, ctx)
	resetPublicSchema(t, ctx, pool)

	if err := Apply(ctx, pool); err != nil {
		t.Fatalf("Apply returned error: %v", err)
	}

	if err := EnsureCompatible(ctx, pool); err != nil {
		t.Fatalf("EnsureCompatible returned error: %v", err)
	}
}

func integrationPool(t *testing.T, ctx context.Context) *pgxpool.Pool {
	t.Helper()

	databaseURL := os.Getenv("CONTROL_PLANE_INTEGRATION_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("CONTROL_PLANE_INTEGRATION_DATABASE_URL is not set")
	}

	pool, err := postgres.NewPool(ctx, config.DatabaseConfig{
		URL:             databaseURL,
		MaxOpenConns:    4,
		MinIdleConns:    1,
		MaxConnLifetime: time.Minute,
		MaxConnIdleTime: time.Minute,
	})
	if err != nil {
		t.Fatalf("NewPool returned error: %v", err)
	}

	t.Cleanup(pool.Close)
	return pool
}

func resetPublicSchema(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Helper()

	if _, err := pool.Exec(ctx, `DROP SCHEMA IF EXISTS public CASCADE`); err != nil {
		t.Fatalf("drop public schema: %v", err)
	}

	if _, err := pool.Exec(ctx, `CREATE SCHEMA public`); err != nil {
		t.Fatalf("create public schema: %v", err)
	}
}
