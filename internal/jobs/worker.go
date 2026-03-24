package jobs

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"github.com/juanpablogaviria/golden-path-control-plane/internal/app"
	"github.com/juanpablogaviria/golden-path-control-plane/internal/domain"
	"github.com/juanpablogaviria/golden-path-control-plane/internal/observability"
)

type Worker struct {
	id           string
	logger       *slog.Logger
	service      *app.ControlPlane
	repo         app.Repository
	metrics      *observability.Metrics
	pollInterval time.Duration
	batchSize    int
	lease        time.Duration
}

func NewWorker(logger *slog.Logger, service *app.ControlPlane, repo app.Repository, metrics *observability.Metrics, pollInterval time.Duration, batchSize int, lease time.Duration) *Worker {
	return &Worker{
		id:           "worker-" + uuid.NewString(),
		logger:       logger,
		service:      service,
		repo:         repo,
		metrics:      metrics,
		pollInterval: pollInterval,
		batchSize:    batchSize,
		lease:        lease,
	}
}

func (w *Worker) Run(ctx context.Context) error {
	ticker := time.NewTicker(w.pollInterval)
	defer ticker.Stop()

	for {
		if err := w.runOnce(ctx); err != nil {
			w.logger.Error("worker_iteration_failed", "worker_id", w.id, "error", err.Error())
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

func (w *Worker) runOnce(ctx context.Context) error {
	jobs, err := w.repo.ClaimJobs(ctx, w.id, w.batchSize, w.lease)
	if err != nil {
		return err
	}

	for _, job := range jobs {
		if err := w.processJob(ctx, job); err != nil {
			if markErr := w.repo.MarkJobFailed(ctx, job, err); markErr != nil {
				return fmt.Errorf("worker: process job %s failed: %v; additionally failed to persist failure: %w", job.ID, err, markErr)
			}
			w.metrics.WorkerJobsTotal.WithLabelValues(string(job.Type), "failed").Inc()
			w.logger.Error("worker_job_failed", "worker_id", w.id, "job_id", job.ID, "type", job.Type, "error", err.Error())
			continue
		}

		if err := w.repo.MarkJobCompleted(ctx, job.ID); err != nil {
			return fmt.Errorf("worker: mark job completed: %w", err)
		}
		w.metrics.WorkerJobsTotal.WithLabelValues(string(job.Type), "completed").Inc()
		w.logger.Info("worker_job_completed", "worker_id", w.id, "job_id", job.ID, "type", job.Type)
	}

	return nil
}

func (w *Worker) processJob(ctx context.Context, job domain.Job) error {
	switch job.Type {
	case domain.JobTypeServiceEvaluation:
		return w.service.ProcessServiceEvaluationJob(ctx, job, w.id)
	default:
		return fmt.Errorf("worker: unsupported job type %q", job.Type)
	}
}
