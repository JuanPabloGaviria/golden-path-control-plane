package migrations

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed sql/*.sql
var sqlFiles embed.FS

const advisoryLockKey int64 = 486031

var (
	ErrSchemaUninitialized = errors.New("migrations: schema is not initialized")
	ErrSchemaBehind        = errors.New("migrations: schema is behind the current binary")
	ErrSchemaAhead         = errors.New("migrations: schema is newer than the current binary")
)

func Versions() []string {
	entries, err := migrationEntries()
	if err != nil {
		panic(err)
	}

	versions := make([]string, 0, len(entries))
	for _, entry := range entries {
		versions = append(versions, entry.Name())
	}

	return versions
}

func CurrentVersion() string {
	versions := Versions()
	if len(versions) == 0 {
		return ""
	}

	return versions[len(versions)-1]
}

func Apply(ctx context.Context, pool *pgxpool.Pool) error {
	conn, err := pool.Acquire(ctx)
	if err != nil {
		return fmt.Errorf("migrations: acquire connection: %w", err)
	}
	defer conn.Release()

	if _, err := conn.Exec(ctx, `SELECT pg_advisory_lock($1)`, advisoryLockKey); err != nil {
		return fmt.Errorf("migrations: acquire advisory lock: %w", err)
	}
	defer func() {
		_, _ = conn.Exec(context.Background(), `SELECT pg_advisory_unlock($1)`, advisoryLockKey)
	}()

	if _, err := conn.Exec(ctx, `CREATE SCHEMA IF NOT EXISTS public`); err != nil {
		return fmt.Errorf("migrations: ensure public schema: %w", err)
	}

	if _, err := conn.Exec(ctx, `SET search_path TO public`); err != nil {
		return fmt.Errorf("migrations: set search_path: %w", err)
	}

	if _, err := conn.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version TEXT PRIMARY KEY,
			applied_at TIMESTAMPTZ NOT NULL
		)
	`); err != nil {
		return fmt.Errorf("migrations: ensure schema_migrations table: %w", err)
	}

	entries, err := migrationEntries()
	if err != nil {
		return err
	}

	for _, entry := range entries {
		applied, err := migrationApplied(ctx, conn, entry.Name())
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

		tx, err := conn.Begin(ctx)
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

func EnsureCompatible(ctx context.Context, pool *pgxpool.Pool) error {
	applied, err := AppliedVersions(ctx, pool)
	if err != nil {
		return err
	}

	expected := Versions()
	expectedSet := make(map[string]struct{}, len(expected))
	for _, version := range expected {
		expectedSet[version] = struct{}{}
	}

	appliedSet := make(map[string]struct{}, len(applied))
	for _, version := range applied {
		appliedSet[version] = struct{}{}
		if _, ok := expectedSet[version]; !ok {
			return fmt.Errorf("%w: database contains unknown migration version %s while binary expects up to %s", ErrSchemaAhead, version, CurrentVersion())
		}
	}

	missing := make([]string, 0)
	for _, version := range expected {
		if _, ok := appliedSet[version]; !ok {
			missing = append(missing, version)
		}
	}

	if len(missing) > 0 {
		return fmt.Errorf("%w: missing versions %s; run cmd/migrate", ErrSchemaBehind, strings.Join(missing, ", "))
	}

	return nil
}

func AppliedVersions(ctx context.Context, pool *pgxpool.Pool) ([]string, error) {
	conn, err := pool.Acquire(ctx)
	if err != nil {
		return nil, fmt.Errorf("migrations: acquire connection: %w", err)
	}
	defer conn.Release()

	exists, err := schemaMigrationsTableExists(ctx, conn)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, fmt.Errorf("%w: run cmd/migrate before starting the runtime", ErrSchemaUninitialized)
	}

	rows, err := conn.Query(ctx, `SELECT version FROM schema_migrations ORDER BY version ASC`)
	if err != nil {
		return nil, fmt.Errorf("migrations: list applied versions: %w", err)
	}
	defer rows.Close()

	versions := make([]string, 0)
	for rows.Next() {
		var version string
		if err := rows.Scan(&version); err != nil {
			return nil, fmt.Errorf("migrations: scan applied version: %w", err)
		}
		versions = append(versions, version)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("migrations: iterate applied versions: %w", err)
	}

	return versions, nil
}

type queryRower interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}

func migrationApplied(ctx context.Context, conn queryRower, version string) (bool, error) {
	var exists bool
	if err := conn.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM schema_migrations WHERE version = $1)`, version).Scan(&exists); err != nil {
		return false, fmt.Errorf("migrations: check version %s: %w", version, err)
	}

	return exists, nil
}

func schemaMigrationsTableExists(ctx context.Context, conn queryRower) (bool, error) {
	var relationName *string
	if err := conn.QueryRow(ctx, `SELECT to_regclass('public.schema_migrations')`).Scan(&relationName); err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "3D000" {
			return false, fmt.Errorf("%w: database does not exist", ErrSchemaUninitialized)
		}
		return false, fmt.Errorf("migrations: check schema_migrations table: %w", err)
	}

	return relationName != nil, nil
}

func migrationEntries() ([]fs.DirEntry, error) {
	entries, err := fs.ReadDir(sqlFiles, "sql")
	if err != nil {
		return nil, fmt.Errorf("migrations: read embedded SQL: %w", err)
	}

	filtered := make([]fs.DirEntry, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}
		filtered = append(filtered, entry)
	}

	sort.Slice(filtered, func(i, j int) bool {
		return filtered[i].Name() < filtered[j].Name()
	})

	return filtered, nil
}
