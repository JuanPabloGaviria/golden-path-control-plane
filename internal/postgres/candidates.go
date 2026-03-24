package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/juanpablogaviria/golden-path-control-plane/internal/domain"
)

func (s *Store) CreateDeploymentCandidate(ctx context.Context, candidate domain.DeploymentCandidate) (domain.DeploymentCandidate, error) {
	if _, err := s.pool.Exec(ctx, `
		INSERT INTO deployment_candidates (
			id, service_id, environment, version, commit_sha, requested_by,
			status, decision_reason, last_snapshot_id, created_at, updated_at, evaluated_at
		) VALUES (
			$1, $2, $3, $4, $5, $6,
			$7, $8, $9, $10, $11, $12
		)
	`,
		candidate.ID, candidate.ServiceID, candidate.Environment, candidate.Version, candidate.CommitSHA, candidate.RequestedBy,
		candidate.Status, candidate.DecisionReason, candidate.LastSnapshotID, candidate.CreatedAt, candidate.UpdatedAt, candidate.EvaluatedAt,
	); err != nil {
		return domain.DeploymentCandidate{}, fmt.Errorf("postgres: insert deployment candidate: %w", err)
	}

	return s.GetDeploymentCandidate(ctx, candidate.ID)
}

func (s *Store) GetLatestSnapshot(ctx context.Context, serviceID uuid.UUID) (domain.ReadinessSnapshot, error) {
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
		if err == pgx.ErrNoRows {
			return domain.ReadinessSnapshot{}, ErrNotFound
		}
		return domain.ReadinessSnapshot{}, fmt.Errorf("postgres: get latest snapshot: %w", err)
	}

	return snapshot, nil
}

func (s *Store) SavePromotionDecision(ctx context.Context, candidateID uuid.UUID, snapshot *domain.ReadinessSnapshot, decision domain.PromotionDecision, status domain.CandidateStatus, reason string) (domain.DeploymentCandidate, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return domain.DeploymentCandidate{}, fmt.Errorf("postgres: begin save promotion decision: %w", err)
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	var snapshotID *uuid.UUID
	evaluatedAtValue := time.Now().UTC()
	if snapshot != nil {
		snapshotID = &snapshot.ID
	}

	commandTag, err := tx.Exec(ctx, `
		UPDATE deployment_candidates
		SET status = $2,
		    decision_reason = $3,
		    last_snapshot_id = $4,
		    updated_at = $5,
		    evaluated_at = $5
		WHERE id = $1
		  AND status = 'pending'
	`, candidateID, status, reason, snapshotID, evaluatedAtValue)
	if err != nil {
		return domain.DeploymentCandidate{}, fmt.Errorf("postgres: update deployment candidate decision: %w", err)
	}

	if commandTag.RowsAffected() != 1 {
		return domain.DeploymentCandidate{}, ErrStateConflict
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO promotion_decisions (id, candidate_id, snapshot_id, decision, summary, created_at)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (candidate_id) DO UPDATE
		SET snapshot_id = EXCLUDED.snapshot_id,
		    decision = EXCLUDED.decision,
		    summary = EXCLUDED.summary,
		    created_at = EXCLUDED.created_at
	`, decision.ID, decision.CandidateID, decision.SnapshotID, decision.Decision, decision.Summary, decision.CreatedAt); err != nil {
		return domain.DeploymentCandidate{}, fmt.Errorf("postgres: upsert promotion decision: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return domain.DeploymentCandidate{}, fmt.Errorf("postgres: commit promotion decision: %w", err)
	}

	return s.GetDeploymentCandidate(ctx, candidateID)
}

func (s *Store) GetDeploymentCandidate(ctx context.Context, candidateID uuid.UUID) (domain.DeploymentCandidate, error) {
	row := s.pool.QueryRow(ctx, `
		SELECT
			c.id, c.service_id, c.environment, c.version, c.commit_sha, c.requested_by,
			c.status, c.decision_reason, c.last_snapshot_id, c.created_at, c.updated_at, c.evaluated_at,
			p.id, p.snapshot_id, p.decision, p.summary, p.created_at
		FROM deployment_candidates c
		LEFT JOIN promotion_decisions p ON p.candidate_id = c.id
		WHERE c.id = $1
	`, candidateID)

	var candidate domain.DeploymentCandidate
	var lastSnapshotID *uuid.UUID
	var decisionID *uuid.UUID
	var decisionSnapshotID *uuid.UUID
	var decisionValue *domain.CandidateStatus
	var decisionSummary *string
	var decisionCreatedAt *time.Time

	if err := row.Scan(
		&candidate.ID,
		&candidate.ServiceID,
		&candidate.Environment,
		&candidate.Version,
		&candidate.CommitSHA,
		&candidate.RequestedBy,
		&candidate.Status,
		&candidate.DecisionReason,
		&lastSnapshotID,
		&candidate.CreatedAt,
		&candidate.UpdatedAt,
		&candidate.EvaluatedAt,
		&decisionID,
		&decisionSnapshotID,
		&decisionValue,
		&decisionSummary,
		&decisionCreatedAt,
	); err != nil {
		if err == pgx.ErrNoRows {
			return domain.DeploymentCandidate{}, ErrNotFound
		}
		return domain.DeploymentCandidate{}, fmt.Errorf("postgres: get deployment candidate: %w", err)
	}

	candidate.LastSnapshotID = lastSnapshotID
	if decisionID != nil && decisionValue != nil && decisionSummary != nil && decisionCreatedAt != nil {
		candidate.PromotionDecision = &domain.PromotionDecision{
			ID:          *decisionID,
			CandidateID: candidate.ID,
			SnapshotID:  decisionSnapshotID,
			Decision:    *decisionValue,
			Summary:     *decisionSummary,
			CreatedAt:   *decisionCreatedAt,
		}
	}

	return candidate, nil
}
