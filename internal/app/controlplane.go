package app

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/juanpablogaviria/golden-path-control-plane/internal/auth"
	"github.com/juanpablogaviria/golden-path-control-plane/internal/domain"
	"github.com/juanpablogaviria/golden-path-control-plane/internal/platformchecks"
	"github.com/juanpablogaviria/golden-path-control-plane/internal/postgres"
)

type Repository interface {
	CreateService(ctx context.Context, service domain.Service) (domain.Service, error)
	UpdateService(ctx context.Context, serviceID uuid.UUID, patch domain.ServicePatch) (domain.Service, error)
	GetService(ctx context.Context, serviceID uuid.UUID) (domain.Service, error)
	SaveReadinessEvaluation(ctx context.Context, snapshot domain.ReadinessSnapshot, checks []domain.ReadinessCheckResult) error
	GetLatestScorecard(ctx context.Context, serviceID uuid.UUID) (domain.Scorecard, error)
	EnqueueJob(ctx context.Context, job domain.Job) (domain.Job, error)
	CreateDeploymentCandidate(ctx context.Context, candidate domain.DeploymentCandidate) (domain.DeploymentCandidate, error)
	GetLatestSnapshot(ctx context.Context, serviceID uuid.UUID) (domain.ReadinessSnapshot, error)
	SavePromotionDecision(ctx context.Context, candidateID uuid.UUID, snapshot *domain.ReadinessSnapshot, decision domain.PromotionDecision, status domain.CandidateStatus, reason string) (domain.DeploymentCandidate, error)
	GetDeploymentCandidate(ctx context.Context, candidateID uuid.UUID) (domain.DeploymentCandidate, error)
	InsertAuditEvent(ctx context.Context, event domain.AuditEvent) error
	ListAuditEvents(ctx context.Context, resourceType string, resourceID *uuid.UUID, limit int) ([]domain.AuditEvent, error)
	ClaimJobs(ctx context.Context, workerID string, batchSize int, lease time.Duration) ([]domain.Job, error)
	MarkJobCompleted(ctx context.Context, jobID uuid.UUID, workerID string) error
	MarkJobFailed(ctx context.Context, job domain.Job, failure error) error
}

type ControlPlane struct {
	repo           Repository
	now            func() time.Time
	jobMaxAttempts int
}

func NewControlPlane(repo Repository, jobMaxAttempts int) *ControlPlane {
	return &ControlPlane{
		repo:           repo,
		now:            func() time.Time { return time.Now().UTC() },
		jobMaxAttempts: jobMaxAttempts,
	}
}

func (c *ControlPlane) RegisterService(ctx context.Context, input domain.ServiceInput, principal auth.Principal, requestID string) (domain.Service, error) {
	if err := input.Validate(); err != nil {
		return domain.Service{}, ValidationError{Err: err}
	}

	now := c.now()
	serviceID := uuid.New()
	service := domain.Service{
		ID:                 serviceID,
		Name:               input.Name,
		Description:        input.Description,
		OwnerEmail:         input.OwnerEmail,
		RepositoryURL:      input.RepositoryURL,
		RunbookURL:         input.RunbookURL,
		HealthEndpointURL:  input.HealthEndpointURL,
		ObservabilityURL:   input.ObservabilityURL,
		DeploymentPipeline: input.DeploymentPipeline,
		HasCI:              input.HasCI,
		HasTracing:         input.HasTracing,
		HasMetrics:         input.HasMetrics,
		Language:           input.Language,
		Tier:               input.Tier,
		Lifecycle:          input.Lifecycle,
		CreatedAt:          now,
		UpdatedAt:          now,
		SLOPolicy: domain.SLOPolicy{
			ID:                        uuid.New(),
			ServiceID:                 serviceID,
			AvailabilityTargetPercent: input.SLOPolicy.AvailabilityTargetPercent,
			LatencyTargetMilliseconds: input.SLOPolicy.LatencyTargetMilliseconds,
			TimeWindow:                input.SLOPolicy.Window,
			CreatedAt:                 now,
			UpdatedAt:                 now,
		},
	}

	created, err := c.repo.CreateService(ctx, service)
	if err != nil {
		return domain.Service{}, err
	}

	if err := c.repo.InsertAuditEvent(ctx, auditEvent(now, principal, requestID, "service.registered", "service", created.ID, map[string]any{
		"name": created.Name,
	})); err != nil {
		return domain.Service{}, err
	}

	return created, nil
}

func (c *ControlPlane) UpdateService(ctx context.Context, serviceID uuid.UUID, patch domain.ServicePatch, principal auth.Principal, requestID string) (domain.Service, error) {
	if err := patch.Validate(); err != nil {
		return domain.Service{}, ValidationError{Err: err}
	}

	updated, err := c.repo.UpdateService(ctx, serviceID, patch)
	if err != nil {
		return domain.Service{}, err
	}

	if err := c.repo.InsertAuditEvent(ctx, auditEvent(c.now(), principal, requestID, "service.updated", "service", updated.ID, map[string]any{
		"name": updated.Name,
	})); err != nil {
		return domain.Service{}, err
	}

	return updated, nil
}

func (c *ControlPlane) QueueServiceEvaluation(ctx context.Context, serviceID uuid.UUID, idempotencyKey string, principal auth.Principal, requestID string) (domain.Job, error) {
	if _, err := c.repo.GetService(ctx, serviceID); err != nil {
		return domain.Job{}, err
	}

	now := c.now()
	if idempotencyKey == "" {
		idempotencyKey = uuid.NewString()
	}

	job := domain.Job{
		ID:             uuid.New(),
		Type:           domain.JobTypeServiceEvaluation,
		ResourceID:     serviceID,
		Status:         domain.JobStatusPending,
		IdempotencyKey: idempotencyKey,
		Payload: map[string]any{
			"service_id": serviceID.String(),
		},
		Attempts:    0,
		MaxAttempts: c.jobMaxAttempts,
		AvailableAt: now,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	enqueued, err := c.repo.EnqueueJob(ctx, job)
	if err != nil {
		return domain.Job{}, err
	}

	if err := c.repo.InsertAuditEvent(ctx, auditEvent(now, principal, requestID, "service.evaluation_requested", "service", serviceID, map[string]any{
		"job_id": enqueued.ID.String(),
	})); err != nil {
		return domain.Job{}, err
	}

	return enqueued, nil
}

func (c *ControlPlane) GetScorecard(ctx context.Context, serviceID uuid.UUID) (domain.Scorecard, error) {
	return c.repo.GetLatestScorecard(ctx, serviceID)
}

func (c *ControlPlane) CreateDeploymentCandidate(ctx context.Context, input domain.DeploymentCandidateInput, principal auth.Principal, requestID string) (domain.DeploymentCandidate, error) {
	if err := input.Validate(); err != nil {
		return domain.DeploymentCandidate{}, ValidationError{Err: err}
	}

	if principal.Role == domain.RoleEngineer && principal.Subject != input.RequestedBy {
		return domain.DeploymentCandidate{}, ValidationError{Err: fmt.Errorf("deployment_candidate.requested_by must match authenticated subject")}
	}

	if _, err := c.repo.GetService(ctx, input.ServiceID); err != nil {
		return domain.DeploymentCandidate{}, err
	}

	now := c.now()
	candidate := domain.DeploymentCandidate{
		ID:             uuid.New(),
		ServiceID:      input.ServiceID,
		Environment:    input.Environment,
		Version:        input.Version,
		CommitSHA:      input.CommitSHA,
		RequestedBy:    input.RequestedBy,
		Status:         domain.CandidateStatusPending,
		DecisionReason: "Awaiting explicit promotion evaluation.",
		CreatedAt:      now,
		UpdatedAt:      now,
	}

	created, err := c.repo.CreateDeploymentCandidate(ctx, candidate)
	if err != nil {
		return domain.DeploymentCandidate{}, err
	}

	if err := c.repo.InsertAuditEvent(ctx, auditEvent(now, principal, requestID, "deployment_candidate.created", "deployment_candidate", created.ID, map[string]any{
		"service_id":  created.ServiceID.String(),
		"environment": created.Environment,
		"version":     created.Version,
	})); err != nil {
		return domain.DeploymentCandidate{}, err
	}

	return created, nil
}

func (c *ControlPlane) EvaluateDeploymentCandidate(ctx context.Context, candidateID uuid.UUID, principal auth.Principal, requestID string) (domain.DeploymentCandidate, error) {
	candidate, err := c.repo.GetDeploymentCandidate(ctx, candidateID)
	if err != nil {
		return domain.DeploymentCandidate{}, err
	}

	if candidate.Status != domain.CandidateStatusPending {
		return domain.DeploymentCandidate{}, StateConflictError{Err: fmt.Errorf("deployment candidate %s is already in terminal status %s", candidate.ID, candidate.Status)}
	}

	snapshot, err := c.repo.GetLatestSnapshot(ctx, candidate.ServiceID)
	var decisionStatus domain.CandidateStatus
	var reason string
	var decisionSnapshot *domain.ReadinessSnapshot

	switch {
	case errors.Is(err, postgres.ErrNotFound):
		decisionStatus = domain.CandidateStatusRejected
		reason = "Rejected: no readiness snapshot exists for this service yet."
	case err != nil:
		return domain.DeploymentCandidate{}, err
	case snapshot.State == domain.ReadinessReady:
		decisionStatus = domain.CandidateStatusApproved
		reason = "Approved: latest readiness snapshot is ready."
		decisionSnapshot = &snapshot
	default:
		decisionStatus = domain.CandidateStatusRejected
		reason = fmt.Sprintf("Rejected: latest readiness snapshot is %s.", snapshot.State)
		decisionSnapshot = &snapshot
	}

	now := c.now()
	var snapshotID *uuid.UUID
	if decisionSnapshot != nil {
		snapshotID = &decisionSnapshot.ID
	}

	decision := domain.PromotionDecision{
		ID:          uuid.New(),
		CandidateID: candidate.ID,
		SnapshotID:  snapshotID,
		Decision:    decisionStatus,
		Summary:     reason,
		CreatedAt:   now,
	}

	updated, err := c.repo.SavePromotionDecision(ctx, candidate.ID, decisionSnapshot, decision, decisionStatus, reason)
	if err != nil {
		return domain.DeploymentCandidate{}, err
	}

	if err := c.repo.InsertAuditEvent(ctx, auditEvent(now, principal, requestID, "deployment_candidate.evaluated", "deployment_candidate", updated.ID, map[string]any{
		"decision": updated.Status,
		"reason":   updated.DecisionReason,
	})); err != nil {
		return domain.DeploymentCandidate{}, err
	}

	return updated, nil
}

func (c *ControlPlane) GetDeploymentCandidate(ctx context.Context, candidateID uuid.UUID) (domain.DeploymentCandidate, error) {
	return c.repo.GetDeploymentCandidate(ctx, candidateID)
}

func (c *ControlPlane) ListAuditEvents(ctx context.Context, resourceType string, resourceID *uuid.UUID, limit int) ([]domain.AuditEvent, error) {
	return c.repo.ListAuditEvents(ctx, resourceType, resourceID, limit)
}

func (c *ControlPlane) ProcessServiceEvaluationJob(ctx context.Context, job domain.Job, workerID string) error {
	service, err := c.repo.GetService(ctx, job.ResourceID)
	if err != nil {
		return err
	}

	result := platformchecks.Evaluate(service, job.ID, c.now())
	if err := c.repo.SaveReadinessEvaluation(ctx, result.Snapshot, result.Checks); err != nil {
		return err
	}

	return c.repo.InsertAuditEvent(ctx, auditEvent(c.now(), auth.Principal{
		Subject: workerID,
		Role:    domain.RolePlatformAdmin,
		Issuer:  "worker",
	}, job.ID.String(), "service.evaluated", "service", service.ID, map[string]any{
		"snapshot_id": result.Snapshot.ID.String(),
		"state":       result.Snapshot.State,
		"score":       result.Snapshot.Score,
	}))
}

func auditEvent(now time.Time, principal auth.Principal, requestID, eventType, resourceType string, resourceID uuid.UUID, details map[string]any) domain.AuditEvent {
	return domain.AuditEvent{
		ID:           uuid.New(),
		ActorSubject: principal.Subject,
		ActorRole:    principal.Role,
		EventType:    eventType,
		ResourceType: resourceType,
		ResourceID:   resourceID,
		RequestID:    requestID,
		Details:      details,
		CreatedAt:    now,
	}
}
