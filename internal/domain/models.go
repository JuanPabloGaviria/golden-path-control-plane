package domain

import (
	"errors"
	"fmt"
	"net/mail"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
)

type Role string

const (
	RoleEngineer      Role = "engineer"
	RolePlatformAdmin Role = "platform-admin"
)

type ReadinessState string

const (
	ReadinessReady    ReadinessState = "ready"
	ReadinessDegraded ReadinessState = "degraded"
	ReadinessBlocked  ReadinessState = "blocked"
)

type CheckStatus string

const (
	CheckStatusPass CheckStatus = "pass"
	CheckStatusWarn CheckStatus = "warn"
	CheckStatusFail CheckStatus = "fail"
)

type CandidateStatus string

const (
	CandidateStatusPending  CandidateStatus = "pending"
	CandidateStatusApproved CandidateStatus = "approved"
	CandidateStatusRejected CandidateStatus = "rejected"
)

type JobStatus string

const (
	JobStatusPending    JobStatus = "pending"
	JobStatusProcessing JobStatus = "processing"
	JobStatusCompleted  JobStatus = "completed"
	JobStatusFailed     JobStatus = "failed"
)

type JobType string

const (
	JobTypeServiceEvaluation JobType = "service_evaluation"
)

type Service struct {
	ID                 uuid.UUID `json:"id"`
	Name               string    `json:"name"`
	Description        string    `json:"description"`
	OwnerEmail         string    `json:"owner_email"`
	RepositoryURL      string    `json:"repository_url"`
	RunbookURL         string    `json:"runbook_url"`
	HealthEndpointURL  string    `json:"health_endpoint_url"`
	ObservabilityURL   string    `json:"observability_url"`
	DeploymentPipeline string    `json:"deployment_pipeline"`
	HasCI              bool      `json:"has_ci"`
	HasTracing         bool      `json:"has_tracing"`
	HasMetrics         bool      `json:"has_metrics"`
	Language           string    `json:"language"`
	Tier               int       `json:"tier"`
	Lifecycle          string    `json:"lifecycle"`
	SLOPolicy          SLOPolicy `json:"slo_policy"`
	CreatedAt          time.Time `json:"created_at"`
	UpdatedAt          time.Time `json:"updated_at"`
}

type SLOPolicy struct {
	ID                        uuid.UUID `json:"id"`
	ServiceID                 uuid.UUID `json:"service_id"`
	AvailabilityTargetPercent float64   `json:"availability_target_percent"`
	LatencyTargetMilliseconds int       `json:"latency_target_milliseconds"`
	TimeWindow                string    `json:"window"`
	CreatedAt                 time.Time `json:"created_at"`
	UpdatedAt                 time.Time `json:"updated_at"`
}

type ReadinessSnapshot struct {
	ID          uuid.UUID      `json:"id"`
	ServiceID   uuid.UUID      `json:"service_id"`
	State       ReadinessState `json:"state"`
	Score       int            `json:"score"`
	Summary     string         `json:"summary"`
	EvaluatedAt time.Time      `json:"evaluated_at"`
	JobID       uuid.UUID      `json:"job_id"`
	CreatedAt   time.Time      `json:"created_at"`
}

type ReadinessCheckResult struct {
	ID              uuid.UUID   `json:"id"`
	SnapshotID      uuid.UUID   `json:"snapshot_id"`
	ServiceID       uuid.UUID   `json:"service_id"`
	CheckName       string      `json:"check_name"`
	Status          CheckStatus `json:"status"`
	Message         string      `json:"message"`
	RemediationHint string      `json:"remediation_hint"`
	CreatedAt       time.Time   `json:"created_at"`
}

type Scorecard struct {
	Service  Service                `json:"service"`
	Snapshot ReadinessSnapshot      `json:"snapshot"`
	Checks   []ReadinessCheckResult `json:"checks"`
}

type DeploymentCandidate struct {
	ID                uuid.UUID          `json:"id"`
	ServiceID         uuid.UUID          `json:"service_id"`
	Environment       string             `json:"environment"`
	Version           string             `json:"version"`
	CommitSHA         string             `json:"commit_sha"`
	RequestedBy       string             `json:"requested_by"`
	Status            CandidateStatus    `json:"status"`
	DecisionReason    string             `json:"decision_reason"`
	LastSnapshotID    *uuid.UUID         `json:"last_snapshot_id,omitempty"`
	CreatedAt         time.Time          `json:"created_at"`
	UpdatedAt         time.Time          `json:"updated_at"`
	EvaluatedAt       *time.Time         `json:"evaluated_at,omitempty"`
	PromotionDecision *PromotionDecision `json:"promotion_decision,omitempty"`
}

type PromotionDecision struct {
	ID          uuid.UUID       `json:"id"`
	CandidateID uuid.UUID       `json:"candidate_id"`
	SnapshotID  *uuid.UUID      `json:"snapshot_id,omitempty"`
	Decision    CandidateStatus `json:"decision"`
	Summary     string          `json:"summary"`
	CreatedAt   time.Time       `json:"created_at"`
}

type AuditEvent struct {
	ID           uuid.UUID      `json:"id"`
	ActorSubject string         `json:"actor_subject"`
	ActorRole    Role           `json:"actor_role"`
	EventType    string         `json:"event_type"`
	ResourceType string         `json:"resource_type"`
	ResourceID   uuid.UUID      `json:"resource_id"`
	RequestID    string         `json:"request_id"`
	Details      map[string]any `json:"details"`
	CreatedAt    time.Time      `json:"created_at"`
}

type Job struct {
	ID             uuid.UUID      `json:"id"`
	Type           JobType        `json:"type"`
	ResourceID     uuid.UUID      `json:"resource_id"`
	Status         JobStatus      `json:"status"`
	IdempotencyKey string         `json:"idempotency_key"`
	Payload        map[string]any `json:"payload"`
	Attempts       int            `json:"attempts"`
	MaxAttempts    int            `json:"max_attempts"`
	AvailableAt    time.Time      `json:"available_at"`
	LockedAt       *time.Time     `json:"locked_at,omitempty"`
	LockOwner      string         `json:"lock_owner,omitempty"`
	LastError      string         `json:"last_error,omitempty"`
	CreatedAt      time.Time      `json:"created_at"`
	UpdatedAt      time.Time      `json:"updated_at"`
}

type ServiceInput struct {
	Name               string         `json:"name"`
	Description        string         `json:"description"`
	OwnerEmail         string         `json:"owner_email"`
	RepositoryURL      string         `json:"repository_url"`
	RunbookURL         string         `json:"runbook_url"`
	HealthEndpointURL  string         `json:"health_endpoint_url"`
	ObservabilityURL   string         `json:"observability_url"`
	DeploymentPipeline string         `json:"deployment_pipeline"`
	HasCI              bool           `json:"has_ci"`
	HasTracing         bool           `json:"has_tracing"`
	HasMetrics         bool           `json:"has_metrics"`
	Language           string         `json:"language"`
	Tier               int            `json:"tier"`
	Lifecycle          string         `json:"lifecycle"`
	SLOPolicy          SLOPolicyInput `json:"slo_policy"`
}

type ServicePatch struct {
	Description        *string         `json:"description,omitempty"`
	OwnerEmail         *string         `json:"owner_email,omitempty"`
	RepositoryURL      *string         `json:"repository_url,omitempty"`
	RunbookURL         *string         `json:"runbook_url,omitempty"`
	HealthEndpointURL  *string         `json:"health_endpoint_url,omitempty"`
	ObservabilityURL   *string         `json:"observability_url,omitempty"`
	DeploymentPipeline *string         `json:"deployment_pipeline,omitempty"`
	HasCI              *bool           `json:"has_ci,omitempty"`
	HasTracing         *bool           `json:"has_tracing,omitempty"`
	HasMetrics         *bool           `json:"has_metrics,omitempty"`
	Language           *string         `json:"language,omitempty"`
	Tier               *int            `json:"tier,omitempty"`
	Lifecycle          *string         `json:"lifecycle,omitempty"`
	SLOPolicy          *SLOPolicyInput `json:"slo_policy,omitempty"`
}

type SLOPolicyInput struct {
	AvailabilityTargetPercent float64 `json:"availability_target_percent"`
	LatencyTargetMilliseconds int     `json:"latency_target_milliseconds"`
	Window                    string  `json:"window"`
}

type DeploymentCandidateInput struct {
	ServiceID   uuid.UUID `json:"service_id"`
	Environment string    `json:"environment"`
	Version     string    `json:"version"`
	CommitSHA   string    `json:"commit_sha"`
	RequestedBy string    `json:"requested_by"`
}

func (in ServiceInput) Validate() error {
	if strings.TrimSpace(in.Name) == "" {
		return errors.New("service.name is required")
	}

	if strings.TrimSpace(in.Description) == "" {
		return errors.New("service.description is required")
	}

	if err := validateEmail(in.OwnerEmail); err != nil {
		return fmt.Errorf("service.owner_email: %w", err)
	}

	if err := validateURL("service.repository_url", in.RepositoryURL); err != nil {
		return err
	}

	if err := validateURL("service.runbook_url", in.RunbookURL); err != nil {
		return err
	}

	if err := validateURL("service.health_endpoint_url", in.HealthEndpointURL); err != nil {
		return err
	}

	if err := validateURL("service.observability_url", in.ObservabilityURL); err != nil {
		return err
	}

	if strings.TrimSpace(in.DeploymentPipeline) == "" {
		return errors.New("service.deployment_pipeline is required")
	}

	if strings.TrimSpace(in.Language) == "" {
		return errors.New("service.language is required")
	}

	if in.Tier < 0 || in.Tier > 3 {
		return errors.New("service.tier must be between 0 and 3")
	}

	if strings.TrimSpace(in.Lifecycle) == "" {
		return errors.New("service.lifecycle is required")
	}

	return in.SLOPolicy.Validate()
}

func (in SLOPolicyInput) Validate() error {
	if in.AvailabilityTargetPercent <= 0 || in.AvailabilityTargetPercent > 100 {
		return errors.New("slo_policy.availability_target_percent must be between 0 and 100")
	}

	if in.LatencyTargetMilliseconds <= 0 {
		return errors.New("slo_policy.latency_target_milliseconds must be greater than zero")
	}

	if strings.TrimSpace(in.Window) == "" {
		return errors.New("slo_policy.window is required")
	}

	return nil
}

func (in ServicePatch) Validate() error {
	if in.Description == nil &&
		in.OwnerEmail == nil &&
		in.RepositoryURL == nil &&
		in.RunbookURL == nil &&
		in.HealthEndpointURL == nil &&
		in.ObservabilityURL == nil &&
		in.DeploymentPipeline == nil &&
		in.HasCI == nil &&
		in.HasTracing == nil &&
		in.HasMetrics == nil &&
		in.Language == nil &&
		in.Tier == nil &&
		in.Lifecycle == nil &&
		in.SLOPolicy == nil {
		return errors.New("service patch must include at least one field")
	}

	if in.Description != nil && strings.TrimSpace(*in.Description) == "" {
		return errors.New("service.description must not be empty")
	}

	if in.OwnerEmail != nil {
		if err := validateEmail(*in.OwnerEmail); err != nil {
			return fmt.Errorf("service.owner_email: %w", err)
		}
	}

	if in.RepositoryURL != nil {
		if err := validateURL("service.repository_url", *in.RepositoryURL); err != nil {
			return err
		}
	}

	if in.RunbookURL != nil {
		if err := validateURL("service.runbook_url", *in.RunbookURL); err != nil {
			return err
		}
	}

	if in.HealthEndpointURL != nil {
		if err := validateURL("service.health_endpoint_url", *in.HealthEndpointURL); err != nil {
			return err
		}
	}

	if in.ObservabilityURL != nil {
		if err := validateURL("service.observability_url", *in.ObservabilityURL); err != nil {
			return err
		}
	}

	if in.Tier != nil && (*in.Tier < 0 || *in.Tier > 3) {
		return errors.New("service.tier must be between 0 and 3")
	}

	if in.DeploymentPipeline != nil && strings.TrimSpace(*in.DeploymentPipeline) == "" {
		return errors.New("service.deployment_pipeline must not be empty")
	}

	if in.Language != nil && strings.TrimSpace(*in.Language) == "" {
		return errors.New("service.language must not be empty")
	}

	if in.Lifecycle != nil && strings.TrimSpace(*in.Lifecycle) == "" {
		return errors.New("service.lifecycle must not be empty")
	}

	if in.SLOPolicy != nil {
		return in.SLOPolicy.Validate()
	}

	return nil
}

func (in DeploymentCandidateInput) Validate() error {
	if in.ServiceID == uuid.Nil {
		return errors.New("deployment_candidate.service_id is required")
	}

	switch strings.TrimSpace(in.Environment) {
	case "development", "staging", "production":
	default:
		return errors.New("deployment_candidate.environment must be one of development|staging|production")
	}

	if strings.TrimSpace(in.Version) == "" {
		return errors.New("deployment_candidate.version is required")
	}

	if strings.TrimSpace(in.CommitSHA) == "" {
		return errors.New("deployment_candidate.commit_sha is required")
	}

	if err := validateEmail(in.RequestedBy); err != nil {
		return fmt.Errorf("deployment_candidate.requested_by: %w", err)
	}

	return nil
}

func validateEmail(value string) error {
	if strings.TrimSpace(value) == "" {
		return errors.New("is required")
	}

	if _, err := mail.ParseAddress(value); err != nil {
		return fmt.Errorf("must be a valid email: %w", err)
	}

	return nil
}

func validateURL(field, value string) error {
	if strings.TrimSpace(value) == "" {
		return fmt.Errorf("%s is required", field)
	}

	parsed, err := url.ParseRequestURI(value)
	if err != nil {
		return fmt.Errorf("%s must be a valid URL: %w", field, err)
	}

	switch parsed.Scheme {
	case "http", "https":
	default:
		return fmt.Errorf("%s must use http or https", field)
	}

	return nil
}

func (status CandidateStatus) CanTransitionTo(next CandidateStatus) bool {
	switch status {
	case CandidateStatusPending:
		return next == CandidateStatusApproved || next == CandidateStatusRejected
	case CandidateStatusApproved, CandidateStatusRejected:
		return false
	default:
		return false
	}
}
