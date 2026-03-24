package platformchecks

import (
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/juanpablogaviria/golden-path-control-plane/internal/domain"
)

func TestEvaluateReadyService(t *testing.T) {
	service := validService()

	result := Evaluate(service, uuid.New(), time.Now().UTC())

	if result.Snapshot.State != domain.ReadinessReady {
		t.Fatalf("expected ready state, got %s", result.Snapshot.State)
	}

	if result.Snapshot.Score != 100 {
		t.Fatalf("expected score 100, got %d", result.Snapshot.Score)
	}
}

func TestEvaluateBlockedService(t *testing.T) {
	service := validService()
	service.OwnerEmail = ""
	service.HasCI = false
	service.RunbookURL = ""

	result := Evaluate(service, uuid.New(), time.Now().UTC())

	if result.Snapshot.State != domain.ReadinessBlocked {
		t.Fatalf("expected blocked state, got %s", result.Snapshot.State)
	}

	if result.Snapshot.Score >= 100 {
		t.Fatalf("expected degraded score, got %d", result.Snapshot.Score)
	}
}

func validService() domain.Service {
	serviceID := uuid.New()
	return domain.Service{
		ID:                 serviceID,
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
		SLOPolicy: domain.SLOPolicy{
			ID:                        uuid.New(),
			ServiceID:                 serviceID,
			AvailabilityTargetPercent: 99.9,
			LatencyTargetMilliseconds: 250,
			Window:                    "30d",
		},
	}
}
