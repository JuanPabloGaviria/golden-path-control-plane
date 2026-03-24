package platformchecks

import (
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/juanpablogaviria/golden-path-control-plane/internal/domain"
)

type Result struct {
	Snapshot domain.ReadinessSnapshot
	Checks   []domain.ReadinessCheckResult
}

func Evaluate(service domain.Service, jobID uuid.UUID, evaluatedAt time.Time) Result {
	ownerStatus, ownerMessage, ownerHint := passIf(service.OwnerEmail != "", "Owner email present", "Owner email missing", "Set a durable owner_email for operational accountability.")
	repositoryStatus, repositoryMessage, repositoryHint := passIf(service.RepositoryURL != "", "Repository URL present", "Repository URL missing", "Register the canonical repository_url.")
	runbookStatus, runbookMessage, runbookHint := warnIf(service.RunbookURL != "", "Runbook URL present", "Runbook missing", "Add a runbook_url before promotion.")
	healthStatus, healthMessage, healthHint := passIf(service.HealthEndpointURL != "", "Health endpoint URL present", "Health endpoint missing", "Expose and register a health_endpoint_url.")
	observabilityStatus, observabilityMessage, observabilityHint := warnIf(service.ObservabilityURL != "", "Observability dashboard present", "Observability dashboard missing", "Register an observability_url for debugging and ownership.")
	ciStatus, ciMessage, ciHint := passIf(service.HasCI, "CI declared", "CI not declared", "Set has_ci=true and wire CI before promotion.")
	tracingStatus, tracingMessage, tracingHint := warnIf(service.HasTracing, "Tracing declared", "Tracing not declared", "Enable tracing or document the gap before production use.")
	metricsStatus, metricsMessage, metricsHint := warnIf(service.HasMetrics, "Metrics declared", "Metrics not declared", "Enable metrics or document the gap before production use.")
	deployStatus, deployMessage, deployHint := passIf(service.DeploymentPipeline != "", "Deployment pipeline declared", "Deployment pipeline missing", "Set deployment_pipeline to the owning CI/CD workflow.")
	sloStatus, sloMessage, sloHint := warnIf(service.SLOPolicy.TimeWindow != "" && service.SLOPolicy.AvailabilityTargetPercent > 0 && service.SLOPolicy.LatencyTargetMilliseconds > 0, "SLO policy declared", "SLO policy incomplete", "Define availability, latency, and window targets.")

	checks := []domain.ReadinessCheckResult{
		buildCheck(service.ID, "owner_declared", ownerStatus, ownerMessage, ownerHint),
		buildCheck(service.ID, "repository_connected", repositoryStatus, repositoryMessage, repositoryHint),
		buildCheck(service.ID, "runbook_present", runbookStatus, runbookMessage, runbookHint),
		buildCheck(service.ID, "health_endpoint_present", healthStatus, healthMessage, healthHint),
		buildCheck(service.ID, "observability_dashboard_present", observabilityStatus, observabilityMessage, observabilityHint),
		buildCheck(service.ID, "ci_enabled", ciStatus, ciMessage, ciHint),
		buildCheck(service.ID, "tracing_enabled", tracingStatus, tracingMessage, tracingHint),
		buildCheck(service.ID, "metrics_enabled", metricsStatus, metricsMessage, metricsHint),
		buildCheck(service.ID, "deployment_pipeline_declared", deployStatus, deployMessage, deployHint),
		buildCheck(service.ID, "slo_policy_declared", sloStatus, sloMessage, sloHint),
	}

	failures := 0
	warnings := 0
	for _, check := range checks {
		switch check.Status {
		case domain.CheckStatusFail:
			failures++
		case domain.CheckStatusWarn:
			warnings++
		}
	}

	score := 100 - (failures * 30) - (warnings * 10)
	if score < 0 {
		score = 0
	}

	state := domain.ReadinessReady
	summary := "Service is ready for promotion."
	switch {
	case failures > 0:
		state = domain.ReadinessBlocked
		summary = fmt.Sprintf("Service is blocked by %d failing golden-path checks.", failures)
	case warnings > 0:
		state = domain.ReadinessDegraded
		summary = fmt.Sprintf("Service is degraded with %d warning-level golden-path checks.", warnings)
	}

	snapshotID := uuid.New()
	for index := range checks {
		checks[index].SnapshotID = snapshotID
		checks[index].CreatedAt = evaluatedAt
	}

	return Result{
		Snapshot: domain.ReadinessSnapshot{
			ID:          snapshotID,
			ServiceID:   service.ID,
			State:       state,
			Score:       score,
			Summary:     summary,
			EvaluatedAt: evaluatedAt,
			JobID:       jobID,
			CreatedAt:   evaluatedAt,
		},
		Checks: checks,
	}
}

func buildCheck(serviceID uuid.UUID, name string, status domain.CheckStatus, message, hint string) domain.ReadinessCheckResult {
	return domain.ReadinessCheckResult{
		ID:              uuid.New(),
		ServiceID:       serviceID,
		CheckName:       strings.TrimSpace(name),
		Status:          status,
		Message:         message,
		RemediationHint: hint,
	}
}

func passIf(ok bool, passMessage, failMessage, hint string) (domain.CheckStatus, string, string) {
	if ok {
		return domain.CheckStatusPass, passMessage, ""
	}

	return domain.CheckStatusFail, failMessage, hint
}

func warnIf(ok bool, passMessage, warnMessage, hint string) (domain.CheckStatus, string, string) {
	if ok {
		return domain.CheckStatusPass, passMessage, ""
	}

	return domain.CheckStatusWarn, warnMessage, hint
}
