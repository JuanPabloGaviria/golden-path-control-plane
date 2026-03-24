//go:build integration

package app

import (
	"context"
	"errors"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/juanpablogaviria/golden-path-control-plane/internal/auth"
	"github.com/juanpablogaviria/golden-path-control-plane/internal/config"
	"github.com/juanpablogaviria/golden-path-control-plane/internal/domain"
	"github.com/juanpablogaviria/golden-path-control-plane/internal/migrations"
	"github.com/juanpablogaviria/golden-path-control-plane/internal/postgres"
)

func TestControlPlaneHappyPath(t *testing.T) {
	databaseURL := os.Getenv("CONTROL_PLANE_INTEGRATION_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("CONTROL_PLANE_INTEGRATION_DATABASE_URL is not set")
	}

	ctx := context.Background()
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
	defer pool.Close()

	if err := migrations.Ensure(ctx, pool); err != nil {
		t.Fatalf("Ensure returned error: %v", err)
	}

	if _, err := pool.Exec(ctx, `
		TRUNCATE TABLE audit_events, promotion_decisions, deployment_candidates, readiness_check_results, readiness_snapshots, slo_policies, services, jobs CASCADE
	`); err != nil {
		t.Fatalf("truncate test tables: %v", err)
	}

	store := postgres.NewStore(pool)
	controlPlane := NewControlPlane(store, 3)
	principal := auth.Principal{
		Subject: "developer@example.com",
		Role:    domain.RoleEngineer,
		Issuer:  "test",
	}

	service, err := controlPlane.RegisterService(ctx, validServiceInput(), principal, uuid.NewString())
	if err != nil {
		t.Fatalf("RegisterService returned error: %v", err)
	}

	jobOne, err := controlPlane.QueueServiceEvaluation(ctx, service.ID, "same-key", principal, uuid.NewString())
	if err != nil {
		t.Fatalf("QueueServiceEvaluation returned error: %v", err)
	}

	jobTwo, err := controlPlane.QueueServiceEvaluation(ctx, service.ID, "same-key", principal, uuid.NewString())
	if err != nil {
		t.Fatalf("QueueServiceEvaluation with duplicate idempotency key returned error: %v", err)
	}

	if jobOne.ID != jobTwo.ID {
		t.Fatalf("expected duplicate idempotency key to return same job id, got %s and %s", jobOne.ID, jobTwo.ID)
	}

	claimedJobs, err := store.ClaimJobs(ctx, "integration-worker", 5, 30*time.Second)
	if err != nil {
		t.Fatalf("ClaimJobs returned error: %v", err)
	}

	if len(claimedJobs) != 1 {
		t.Fatalf("expected 1 claimed job, got %d", len(claimedJobs))
	}

	if err := controlPlane.ProcessServiceEvaluationJob(ctx, claimedJobs[0], "integration-worker"); err != nil {
		t.Fatalf("ProcessServiceEvaluationJob returned error: %v", err)
	}

	if err := store.MarkJobCompleted(ctx, claimedJobs[0].ID, "integration-worker"); err != nil {
		t.Fatalf("MarkJobCompleted returned error: %v", err)
	}

	scorecard, err := controlPlane.GetScorecard(ctx, service.ID)
	if err != nil {
		t.Fatalf("GetScorecard returned error: %v", err)
	}

	if scorecard.Snapshot.State != domain.ReadinessReady {
		t.Fatalf("expected ready scorecard, got %s", scorecard.Snapshot.State)
	}

	candidate, err := controlPlane.CreateDeploymentCandidate(ctx, domain.DeploymentCandidateInput{
		ServiceID:   service.ID,
		Environment: "production",
		Version:     "v1.0.0",
		CommitSHA:   "abc123",
		RequestedBy: "developer@example.com",
	}, principal, uuid.NewString())
	if err != nil {
		t.Fatalf("CreateDeploymentCandidate returned error: %v", err)
	}

	evaluated, err := controlPlane.EvaluateDeploymentCandidate(ctx, candidate.ID, principal, uuid.NewString())
	if err != nil {
		t.Fatalf("EvaluateDeploymentCandidate returned error: %v", err)
	}

	if evaluated.Status != domain.CandidateStatusApproved {
		t.Fatalf("expected approved candidate, got %s", evaluated.Status)
	}

	events, err := controlPlane.ListAuditEvents(ctx, "", nil, 50)
	if err != nil {
		t.Fatalf("ListAuditEvents returned error: %v", err)
	}

	if len(events) < 5 {
		t.Fatalf("expected at least 5 audit events, got %d", len(events))
	}

	if _, err := controlPlane.GetScorecard(ctx, uuid.New()); !errors.Is(err, postgres.ErrNotFound) {
		t.Fatalf("expected missing scorecard to return ErrNotFound, got %v", err)
	}
}

func TestEvaluateDeploymentCandidateRejectsRepeatedEvaluation(t *testing.T) {
	ctx, store, controlPlane := setupIntegrationHarness(t)

	principal := auth.Principal{
		Subject: "developer@example.com",
		Role:    domain.RoleEngineer,
		Issuer:  "test",
	}

	service, err := controlPlane.RegisterService(ctx, validServiceInput(), principal, uuid.NewString())
	if err != nil {
		t.Fatalf("RegisterService returned error: %v", err)
	}

	job, err := controlPlane.QueueServiceEvaluation(ctx, service.ID, "repeated-evaluation", principal, uuid.NewString())
	if err != nil {
		t.Fatalf("QueueServiceEvaluation returned error: %v", err)
	}

	claimedJobs, err := store.ClaimJobs(ctx, "integration-worker", 1, 30*time.Second)
	if err != nil {
		t.Fatalf("ClaimJobs returned error: %v", err)
	}

	if err := controlPlane.ProcessServiceEvaluationJob(ctx, claimedJobs[0], "integration-worker"); err != nil {
		t.Fatalf("ProcessServiceEvaluationJob returned error: %v", err)
	}

	if err := store.MarkJobCompleted(ctx, job.ID, "integration-worker"); err != nil {
		t.Fatalf("MarkJobCompleted returned error: %v", err)
	}

	candidate, err := controlPlane.CreateDeploymentCandidate(ctx, domain.DeploymentCandidateInput{
		ServiceID:   service.ID,
		Environment: "production",
		Version:     "v1.0.0",
		CommitSHA:   "abc123",
		RequestedBy: "developer@example.com",
	}, principal, uuid.NewString())
	if err != nil {
		t.Fatalf("CreateDeploymentCandidate returned error: %v", err)
	}

	if _, err := controlPlane.EvaluateDeploymentCandidate(ctx, candidate.ID, principal, uuid.NewString()); err != nil {
		t.Fatalf("first EvaluateDeploymentCandidate returned error: %v", err)
	}

	var stateConflictErr StateConflictError
	if _, err := controlPlane.EvaluateDeploymentCandidate(ctx, candidate.ID, principal, uuid.NewString()); !errors.As(err, &stateConflictErr) {
		t.Fatalf("expected second evaluation to fail with StateConflictError, got %v", err)
	}
}

func TestEvaluateDeploymentCandidateWithoutSnapshotSetsEvaluatedAt(t *testing.T) {
	ctx, _, controlPlane := setupIntegrationHarness(t)

	principal := auth.Principal{
		Subject: "developer@example.com",
		Role:    domain.RoleEngineer,
		Issuer:  "test",
	}

	service, err := controlPlane.RegisterService(ctx, validServiceInput(), principal, uuid.NewString())
	if err != nil {
		t.Fatalf("RegisterService returned error: %v", err)
	}

	candidate, err := controlPlane.CreateDeploymentCandidate(ctx, domain.DeploymentCandidateInput{
		ServiceID:   service.ID,
		Environment: "production",
		Version:     "v1.0.0",
		CommitSHA:   "abc123",
		RequestedBy: "developer@example.com",
	}, principal, uuid.NewString())
	if err != nil {
		t.Fatalf("CreateDeploymentCandidate returned error: %v", err)
	}

	evaluated, err := controlPlane.EvaluateDeploymentCandidate(ctx, candidate.ID, principal, uuid.NewString())
	if err != nil {
		t.Fatalf("EvaluateDeploymentCandidate returned error: %v", err)
	}

	if evaluated.Status != domain.CandidateStatusRejected {
		t.Fatalf("expected rejected candidate, got %s", evaluated.Status)
	}

	if evaluated.EvaluatedAt == nil {
		t.Fatal("expected evaluated_at to be recorded even when no snapshot exists")
	}
}

func TestClaimJobsReturnsExclusiveLeasePerWorker(t *testing.T) {
	ctx, store, controlPlane := setupIntegrationHarness(t)

	principal := auth.Principal{
		Subject: "developer@example.com",
		Role:    domain.RoleEngineer,
		Issuer:  "test",
	}

	service, err := controlPlane.RegisterService(ctx, validServiceInput(), principal, uuid.NewString())
	if err != nil {
		t.Fatalf("RegisterService returned error: %v", err)
	}

	if _, err := controlPlane.QueueServiceEvaluation(ctx, service.ID, "exclusive-lease", principal, uuid.NewString()); err != nil {
		t.Fatalf("QueueServiceEvaluation returned error: %v", err)
	}

	type claimResult struct {
		jobs []domain.Job
		err  error
	}

	results := make(chan claimResult, 2)
	var workers sync.WaitGroup
	for _, workerID := range []string{"worker-a", "worker-b"} {
		workers.Add(1)
		go func(id string) {
			defer workers.Done()
			jobs, err := store.ClaimJobs(ctx, id, 1, 30*time.Second)
			results <- claimResult{jobs: jobs, err: err}
		}(workerID)
	}

	workers.Wait()
	close(results)

	totalClaims := 0
	for result := range results {
		if result.err != nil {
			t.Fatalf("ClaimJobs returned error: %v", result.err)
		}
		totalClaims += len(result.jobs)
	}

	if totalClaims != 1 {
		t.Fatalf("expected only one worker to claim the job, got %d total claims", totalClaims)
	}
}

func setupIntegrationHarness(t *testing.T) (context.Context, *postgres.Store, *ControlPlane) {
	t.Helper()

	databaseURL := os.Getenv("CONTROL_PLANE_INTEGRATION_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("CONTROL_PLANE_INTEGRATION_DATABASE_URL is not set")
	}

	ctx := context.Background()
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

	if err := migrations.Ensure(ctx, pool); err != nil {
		t.Fatalf("Ensure returned error: %v", err)
	}

	if _, err := pool.Exec(ctx, `
		TRUNCATE TABLE audit_events, promotion_decisions, deployment_candidates, readiness_check_results, readiness_snapshots, slo_policies, services, jobs CASCADE
	`); err != nil {
		t.Fatalf("truncate test tables: %v", err)
	}

	store := postgres.NewStore(pool)
	return ctx, store, NewControlPlane(store, 3)
}

func validServiceInput() domain.ServiceInput {
	return domain.ServiceInput{
		Name:               "control-plane",
		Description:        "Golden path control plane",
		OwnerEmail:         "owner@example.com",
		RepositoryURL:      "https://github.com/example/control-plane",
		RunbookURL:         "https://example.com/runbook",
		HealthEndpointURL:  "https://example.com/healthz",
		ObservabilityURL:   "https://example.com/dashboard",
		DeploymentPipeline: "github-actions",
		HasCI:              true,
		HasTracing:         true,
		HasMetrics:         true,
		Language:           "go",
		Tier:               1,
		Lifecycle:          "production",
		SLOPolicy: domain.SLOPolicyInput{
			AvailabilityTargetPercent: 99.9,
			LatencyTargetMilliseconds: 250,
			Window:                    "30d",
		},
	}
}
