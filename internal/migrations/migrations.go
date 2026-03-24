package migrations

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed sql/*.sql
var sqlFiles embed.FS

func Ensure(ctx context.Context, pool *pgxpool.Pool) error {
	if _, err := pool.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version TEXT PRIMARY KEY,
			applied_at TIMESTAMPTZ NOT NULL
		)
	`); err != nil {
		return fmt.Errorf("migrations: ensure schema_migrations table: %w", err)
	}

	entries, err := fs.ReadDir(sqlFiles, "sql")
	if err != nil {
		return fmt.Errorf("migrations: read embedded SQL: %w", err)
	}

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Name() < entries[j].Name()
	})

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}

		applied, err := migrationApplied(ctx, pool, entry.Name())
		if err != nil {
			return err
		}
		if applied {
			continue
		}

		contents, err := sqlFiles.ReadFile("sql/" + entry.Name())
		if err != nil {
			return fmt.Errorf("migrations: read file %s: %w", entry.Name(), err)
		}

		tx, err := pool.Begin(ctx)
		if err != nil {
			return fmt.Errorf("migrations: begin transaction for %s: %w", entry.Name(), err)
		}

		if _, err := tx.Exec(ctx, string(contents)); err != nil {
			_ = tx.Rollback(ctx)
			return fmt.Errorf("migrations: apply %s: %w", entry.Name(), err)
		}

		if _, err := tx.Exec(ctx, `INSERT INTO schema_migrations (version, applied_at) VALUES ($1, NOW())`, entry.Name()); err != nil {
			_ = tx.Rollback(ctx)
			return fmt.Errorf("migrations: record %s: %w", entry.Name(), err)
		}

		if err := tx.Commit(ctx); err != nil {
			return fmt.Errorf("migrations: commit %s: %w", entry.Name(), err)
		}
	}

	return nil
}

func migrationApplied(ctx context.Context, pool *pgxpool.Pool, version string) (bool, error) {
	var exists bool
	if err := pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM schema_migrations WHERE version = $1)`, version).Scan(&exists); err != nil {
		return false, fmt.Errorf("migrations: check version %s: %w", version, err)
	}

	return exists, nil
}
