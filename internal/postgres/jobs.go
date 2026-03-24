package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/juanpablogaviria/golden-path-control-plane/internal/domain"
)

func (s *Store) ClaimJobs(ctx context.Context, workerID string, batchSize int, lease time.Duration) ([]domain.Job, error) {
	expiredBefore := time.Now().UTC().Add(-lease)

	rows, err := s.pool.Query(ctx, `
		WITH candidate_jobs AS (
			SELECT id
			FROM jobs
			WHERE attempts < max_attempts
			  AND (
				(status IN ('pending', 'failed') AND available_at <= NOW())
				OR (status = 'processing' AND locked_at IS NOT NULL AND locked_at < $3)
			  )
			ORDER BY available_at ASC, created_at ASC
			LIMIT $1
			FOR UPDATE SKIP LOCKED
		)
		UPDATE jobs j
		SET status = 'processing',
		    locked_at = NOW(),
		    lock_owner = $2,
		    attempts = j.attempts + 1,
		    updated_at = NOW()
		FROM candidate_jobs c
		WHERE j.id = c.id
		RETURNING j.id, j.type, j.resource_id, j.status, j.idempotency_key, j.payload, j.attempts,
		          j.max_attempts, j.available_at, j.locked_at, j.lock_owner, j.last_error, j.created_at, j.updated_at
	`, batchSize, workerID, expiredBefore)
	if err != nil {
		return nil, fmt.Errorf("postgres: claim jobs: %w", err)
	}
	defer rows.Close()

	var jobs []domain.Job
	for rows.Next() {
		job, err := scanJob(rows)
		if err != nil {
			return nil, err
		}
		jobs = append(jobs, job)
	}

	if rows.Err() != nil {
		return nil, fmt.Errorf("postgres: iterate claimed jobs: %w", rows.Err())
	}

	return jobs, nil
}

func (s *Store) MarkJobCompleted(ctx context.Context, jobID uuid.UUID, workerID string) error {
	commandTag, err := s.pool.Exec(ctx, `
		UPDATE jobs
		SET status = 'completed',
		    locked_at = NULL,
		    lock_owner = '',
		    last_error = '',
		    updated_at = NOW()
		WHERE id = $1
		  AND status = 'processing'
		  AND lock_owner = $2
	`, jobID, workerID)
	if err != nil {
		return fmt.Errorf("postgres: mark job completed: %w", err)
	}

	if commandTag.RowsAffected() != 1 {
		return ErrStateConflict
	}

	return nil
}

func (s *Store) MarkJobFailed(ctx context.Context, job domain.Job, failure error) error {
	now := time.Now().UTC()
	backoff := time.Duration(1<<min(job.Attempts, 6)) * time.Second
	if backoff > time.Minute {
		backoff = time.Minute
	}

	commandTag, err := s.pool.Exec(ctx, `
		UPDATE jobs
		SET status = 'failed',
		    locked_at = NULL,
		    lock_owner = '',
		    last_error = $2,
		    available_at = $3,
		    updated_at = $4
		WHERE id = $1
		  AND status = 'processing'
		  AND lock_owner = $5
	`, job.ID, failure.Error(), now.Add(backoff), now, job.LockOwner)
	if err != nil {
		return fmt.Errorf("postgres: mark job failed: %w", err)
	}

	if commandTag.RowsAffected() != 1 {
		return ErrStateConflict
	}

	return nil
}

func scanJob(row interface {
	Scan(dest ...any) error
}) (domain.Job, error) {
	var job domain.Job
	var payloadBytes []byte
	var lockOwner *string
	var lastError *string

	if err := row.Scan(
		&job.ID,
		&job.Type,
		&job.ResourceID,
		&job.Status,
		&job.IdempotencyKey,
		&payloadBytes,
		&job.Attempts,
		&job.MaxAttempts,
		&job.AvailableAt,
		&job.LockedAt,
		&lockOwner,
		&lastError,
		&job.CreatedAt,
		&job.UpdatedAt,
	); err != nil {
		return domain.Job{}, fmt.Errorf("postgres: scan job: %w", err)
	}

	if lockOwner != nil {
		job.LockOwner = *lockOwner
	}

	if lastError != nil {
		job.LastError = *lastError
	}

	if len(payloadBytes) > 0 {
		if err := json.Unmarshal(payloadBytes, &job.Payload); err != nil {
			return domain.Job{}, fmt.Errorf("postgres: unmarshal job payload: %w", err)
		}
	} else {
		job.Payload = map[string]any{}
	}

	return job, nil
}

func min(left, right int) int {
	if left < right {
		return left
	}

	return right
}
