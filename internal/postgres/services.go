package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/juanpablogaviria/golden-path-control-plane/internal/domain"
)

func (s *Store) CreateService(ctx context.Context, service domain.Service) (domain.Service, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return domain.Service{}, fmt.Errorf("postgres: begin create service: %w", err)
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	if _, err := tx.Exec(ctx, `
		INSERT INTO services (
			id, name, description, owner_email, repository_url, runbook_url, health_endpoint_url,
			observability_url, deployment_pipeline, has_ci, has_tracing, has_metrics,
			language, tier, lifecycle, created_at, updated_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7,
			$8, $9, $10, $11, $12,
			$13, $14, $15, $16, $17
		)
	`,
		service.ID, service.Name, service.Description, service.OwnerEmail, service.RepositoryURL, service.RunbookURL,
		service.HealthEndpointURL, service.ObservabilityURL, service.DeploymentPipeline, service.HasCI,
		service.HasTracing, service.HasMetrics, service.Language, service.Tier, service.Lifecycle, service.CreatedAt, service.UpdatedAt,
	); err != nil {
		return domain.Service{}, fmt.Errorf("postgres: insert service: %w", err)
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO slo_policies (
			id, service_id, availability_target_percent, latency_target_milliseconds, window, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7)
	`,
		service.SLOPolicy.ID, service.ID, service.SLOPolicy.AvailabilityTargetPercent,
		service.SLOPolicy.LatencyTargetMilliseconds, service.SLOPolicy.Window, service.SLOPolicy.CreatedAt, service.SLOPolicy.UpdatedAt,
	); err != nil {
		return domain.Service{}, fmt.Errorf("postgres: insert slo policy: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return domain.Service{}, fmt.Errorf("postgres: commit create service: %w", err)
	}

	return s.GetService(ctx, service.ID)
}

func (s *Store) UpdateService(ctx context.Context, serviceID uuid.UUID, patch domain.ServicePatch) (domain.Service, error) {
	current, err := s.GetService(ctx, serviceID)
	if err != nil {
		return domain.Service{}, err
	}

	applyServicePatch(&current, patch)
	current.UpdatedAt = time.Now().UTC()
	current.SLOPolicy.UpdatedAt = current.UpdatedAt

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return domain.Service{}, fmt.Errorf("postgres: begin update service: %w", err)
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	if _, err := tx.Exec(ctx, `
		UPDATE services SET
			description = $2,
			owner_email = $3,
			repository_url = $4,
			runbook_url = $5,
			health_endpoint_url = $6,
			observability_url = $7,
			deployment_pipeline = $8,
			has_ci = $9,
			has_tracing = $10,
			has_metrics = $11,
			language = $12,
			tier = $13,
			lifecycle = $14,
			updated_at = $15
		WHERE id = $1
	`,
		current.ID, current.Description, current.OwnerEmail, current.RepositoryURL, current.RunbookURL,
		current.HealthEndpointURL, current.ObservabilityURL, current.DeploymentPipeline, current.HasCI, current.HasTracing,
		current.HasMetrics, current.Language, current.Tier, current.Lifecycle, current.UpdatedAt,
	); err != nil {
		return domain.Service{}, fmt.Errorf("postgres: update service: %w", err)
	}

	if _, err := tx.Exec(ctx, `
		UPDATE slo_policies SET
			availability_target_percent = $2,
			latency_target_milliseconds = $3,
			window = $4,
			updated_at = $5
		WHERE service_id = $1
	`,
		current.ID, current.SLOPolicy.AvailabilityTargetPercent, current.SLOPolicy.LatencyTargetMilliseconds,
		current.SLOPolicy.Window, current.SLOPolicy.UpdatedAt,
	); err != nil {
		return domain.Service{}, fmt.Errorf("postgres: update slo policy: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return domain.Service{}, fmt.Errorf("postgres: commit update service: %w", err)
	}

	return s.GetService(ctx, current.ID)
}

func (s *Store) GetService(ctx context.Context, serviceID uuid.UUID) (domain.Service, error) {
	row := s.pool.QueryRow(ctx, `
		SELECT
			s.id, s.name, s.description, s.owner_email, s.repository_url, s.runbook_url,
			s.health_endpoint_url, s.observability_url, s.deployment_pipeline, s.has_ci,
			s.has_tracing, s.has_metrics, s.language, s.tier, s.lifecycle,
			s.created_at, s.updated_at,
			p.id, p.availability_target_percent, p.latency_target_milliseconds, p.window, p.created_at, p.updated_at
		FROM services s
		JOIN slo_policies p ON p.service_id = s.id
		WHERE s.id = $1
	`, serviceID)

	service, err := scanService(row)
	if err != nil {
		return domain.Service{}, err
	}

	return service, nil
}

func (s *Store) SaveReadinessEvaluation(ctx context.Context, snapshot domain.ReadinessSnapshot, checks []domain.ReadinessCheckResult) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("postgres: begin save readiness evaluation: %w", err)
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	if _, err := tx.Exec(ctx, `
		INSERT INTO readiness_snapshots (id, service_id, state, score, summary, evaluated_at, job_id, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	`, snapshot.ID, snapshot.ServiceID, snapshot.State, snapshot.Score, snapshot.Summary, snapshot.EvaluatedAt, snapshot.JobID, snapshot.CreatedAt); err != nil {
		return fmt.Errorf("postgres: insert readiness snapshot: %w", err)
	}

	batch := &pgx.Batch{}
	for _, check := range checks {
		batch.Queue(`
			INSERT INTO readiness_check_results (id, snapshot_id, service_id, check_name, status, message, remediation_hint, created_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		`, check.ID, check.SnapshotID, check.ServiceID, check.CheckName, check.Status, check.Message, check.RemediationHint, check.CreatedAt)
	}

	results := tx.SendBatch(ctx, batch)
	defer func() {
		_ = results.Close()
	}()

	for range checks {
		if _, err := results.Exec(); err != nil {
			return fmt.Errorf("postgres: insert readiness check result: %w", err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("postgres: commit readiness evaluation: %w", err)
	}

	return nil
}

func (s *Store) GetLatestScorecard(ctx context.Context, serviceID uuid.UUID) (domain.Scorecard, error) {
	service, err := s.GetService(ctx, serviceID)
	if err != nil {
		return domain.Scorecard{}, err
	}

	row := s.pool.QueryRow(ctx, `
		SELECT id, service_id, state, score, summary, evaluated_at, job_id, created_at
		FROM readiness_snapshots
		WHERE service_id = $1
		ORDER BY evaluated_at DESC
		LIMIT 1
	`, serviceID)

	var snapshot domain.ReadinessSnapshot
	if err := row.Scan(
		&snapshot.ID,
		&snapshot.ServiceID,
		&snapshot.State,
		&snapshot.Score,
		&snapshot.Summary,
		&snapshot.EvaluatedAt,
		&snapshot.JobID,
		&snapshot.CreatedAt,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.Scorecard{}, ErrNotFound
		}
		return domain.Scorecard{}, fmt.Errorf("postgres: get latest scorecard snapshot: %w", err)
	}

	rows, err := s.pool.Query(ctx, `
		SELECT id, snapshot_id, service_id, check_name, status, message, remediation_hint, created_at
		FROM readiness_check_results
		WHERE snapshot_id = $1
		ORDER BY check_name ASC
	`, snapshot.ID)
	if err != nil {
		return domain.Scorecard{}, fmt.Errorf("postgres: get readiness check results: %w", err)
	}
	defer rows.Close()

	var checks []domain.ReadinessCheckResult
	for rows.Next() {
		var check domain.ReadinessCheckResult
		if err := rows.Scan(
			&check.ID,
			&check.SnapshotID,
			&check.ServiceID,
			&check.CheckName,
			&check.Status,
			&check.Message,
			&check.RemediationHint,
			&check.CreatedAt,
		); err != nil {
			return domain.Scorecard{}, fmt.Errorf("postgres: scan readiness check result: %w", err)
		}
		checks = append(checks, check)
	}

	if rows.Err() != nil {
		return domain.Scorecard{}, fmt.Errorf("postgres: iterate readiness check results: %w", rows.Err())
	}

	return domain.Scorecard{
		Service:  service,
		Snapshot: snapshot,
		Checks:   checks,
	}, nil
}

func (s *Store) EnqueueJob(ctx context.Context, job domain.Job) (domain.Job, error) {
	payload, err := json.Marshal(job.Payload)
	if err != nil {
		return domain.Job{}, fmt.Errorf("postgres: marshal job payload: %w", err)
	}

	row := s.pool.QueryRow(ctx, `
		INSERT INTO jobs (
			id, type, resource_id, status, idempotency_key, payload, attempts,
			max_attempts, available_at, locked_at, lock_owner, last_error, created_at, updated_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7,
			$8, $9, NULL, NULL, '', $10, $11
		)
		ON CONFLICT (type, idempotency_key) DO UPDATE
		SET updated_at = EXCLUDED.updated_at
		RETURNING id, type, resource_id, status, idempotency_key, payload, attempts,
		          max_attempts, available_at, locked_at, lock_owner, last_error, created_at, updated_at
	`,
		job.ID, job.Type, job.ResourceID, job.Status, job.IdempotencyKey, payload, job.Attempts,
		job.MaxAttempts, job.AvailableAt, job.CreatedAt, job.UpdatedAt,
	)

	return scanJob(row)
}

func scanService(row pgx.Row) (domain.Service, error) {
	var service domain.Service
	if err := row.Scan(
		&service.ID,
		&service.Name,
		&service.Description,
		&service.OwnerEmail,
		&service.RepositoryURL,
		&service.RunbookURL,
		&service.HealthEndpointURL,
		&service.ObservabilityURL,
		&service.DeploymentPipeline,
		&service.HasCI,
		&service.HasTracing,
		&service.HasMetrics,
		&service.Language,
		&service.Tier,
		&service.Lifecycle,
		&service.CreatedAt,
		&service.UpdatedAt,
		&service.SLOPolicy.ID,
		&service.SLOPolicy.AvailabilityTargetPercent,
		&service.SLOPolicy.LatencyTargetMilliseconds,
		&service.SLOPolicy.Window,
		&service.SLOPolicy.CreatedAt,
		&service.SLOPolicy.UpdatedAt,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.Service{}, ErrNotFound
		}
		return domain.Service{}, fmt.Errorf("postgres: scan service: %w", err)
	}

	service.SLOPolicy.ServiceID = service.ID
	return service, nil
}

func applyServicePatch(service *domain.Service, patch domain.ServicePatch) {
	if patch.Description != nil {
		service.Description = *patch.Description
	}
	if patch.OwnerEmail != nil {
		service.OwnerEmail = *patch.OwnerEmail
	}
	if patch.RepositoryURL != nil {
		service.RepositoryURL = *patch.RepositoryURL
	}
	if patch.RunbookURL != nil {
		service.RunbookURL = *patch.RunbookURL
	}
	if patch.HealthEndpointURL != nil {
		service.HealthEndpointURL = *patch.HealthEndpointURL
	}
	if patch.ObservabilityURL != nil {
		service.ObservabilityURL = *patch.ObservabilityURL
	}
	if patch.DeploymentPipeline != nil {
		service.DeploymentPipeline = *patch.DeploymentPipeline
	}
	if patch.HasCI != nil {
		service.HasCI = *patch.HasCI
	}
	if patch.HasTracing != nil {
		service.HasTracing = *patch.HasTracing
	}
	if patch.HasMetrics != nil {
		service.HasMetrics = *patch.HasMetrics
	}
	if patch.Language != nil {
		service.Language = *patch.Language
	}
	if patch.Tier != nil {
		service.Tier = *patch.Tier
	}
	if patch.Lifecycle != nil {
		service.Lifecycle = *patch.Lifecycle
	}
	if patch.SLOPolicy != nil {
		service.SLOPolicy.AvailabilityTargetPercent = patch.SLOPolicy.AvailabilityTargetPercent
		service.SLOPolicy.LatencyTargetMilliseconds = patch.SLOPolicy.LatencyTargetMilliseconds
		service.SLOPolicy.Window = patch.SLOPolicy.Window
	}
}
