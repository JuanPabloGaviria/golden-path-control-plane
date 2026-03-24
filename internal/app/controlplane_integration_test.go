//go:build integration

package app

import (
	"context"
	"errors"
	"os"
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

	if err := store.MarkJobCompleted(ctx, claimedJobs[0].ID); err != nil {
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
