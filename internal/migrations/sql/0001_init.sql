CREATE TABLE IF NOT EXISTS jobs (
    id UUID PRIMARY KEY,
    type TEXT NOT NULL,
    resource_id UUID NOT NULL,
    status TEXT NOT NULL,
    idempotency_key TEXT NOT NULL,
    payload JSONB NOT NULL DEFAULT '{}'::jsonb,
    attempts INTEGER NOT NULL DEFAULT 0,
    max_attempts INTEGER NOT NULL,
    available_at TIMESTAMPTZ NOT NULL,
    locked_at TIMESTAMPTZ NULL,
    lock_owner TEXT NULL,
    last_error TEXT NULL,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    UNIQUE (type, idempotency_key)
);

CREATE INDEX IF NOT EXISTS idx_jobs_pending_available
    ON jobs (status, available_at);

CREATE TABLE IF NOT EXISTS services (
    id UUID PRIMARY KEY,
    name TEXT NOT NULL UNIQUE,
    description TEXT NOT NULL,
    owner_email TEXT NOT NULL,
    repository_url TEXT NOT NULL,
    runbook_url TEXT NOT NULL,
    health_endpoint_url TEXT NOT NULL,
    observability_url TEXT NOT NULL,
    deployment_pipeline TEXT NOT NULL,
    has_ci BOOLEAN NOT NULL,
    has_tracing BOOLEAN NOT NULL,
    has_metrics BOOLEAN NOT NULL,
    language TEXT NOT NULL,
    tier INTEGER NOT NULL,
    lifecycle TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL
);

CREATE TABLE IF NOT EXISTS slo_policies (
    id UUID PRIMARY KEY,
    service_id UUID NOT NULL UNIQUE REFERENCES services(id) ON DELETE CASCADE,
    availability_target_percent DOUBLE PRECISION NOT NULL,
    latency_target_milliseconds INTEGER NOT NULL,
    window TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL
);

CREATE TABLE IF NOT EXISTS readiness_snapshots (
    id UUID PRIMARY KEY,
    service_id UUID NOT NULL REFERENCES services(id) ON DELETE CASCADE,
    state TEXT NOT NULL,
    score INTEGER NOT NULL,
    summary TEXT NOT NULL,
    evaluated_at TIMESTAMPTZ NOT NULL,
    job_id UUID NOT NULL REFERENCES jobs(id),
    created_at TIMESTAMPTZ NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_readiness_snapshots_service_eval
    ON readiness_snapshots (service_id, evaluated_at DESC);

CREATE TABLE IF NOT EXISTS readiness_check_results (
    id UUID PRIMARY KEY,
    snapshot_id UUID NOT NULL REFERENCES readiness_snapshots(id) ON DELETE CASCADE,
    service_id UUID NOT NULL REFERENCES services(id) ON DELETE CASCADE,
    check_name TEXT NOT NULL,
    status TEXT NOT NULL,
    message TEXT NOT NULL,
    remediation_hint TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL
);

CREATE TABLE IF NOT EXISTS deployment_candidates (
    id UUID PRIMARY KEY,
    service_id UUID NOT NULL REFERENCES services(id) ON DELETE CASCADE,
    environment TEXT NOT NULL,
    version TEXT NOT NULL,
    commit_sha TEXT NOT NULL,
    requested_by TEXT NOT NULL,
    status TEXT NOT NULL,
    decision_reason TEXT NOT NULL,
    last_snapshot_id UUID NULL REFERENCES readiness_snapshots(id),
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    evaluated_at TIMESTAMPTZ NULL
);

CREATE INDEX IF NOT EXISTS idx_deployment_candidates_service_created
    ON deployment_candidates (service_id, created_at DESC);

CREATE TABLE IF NOT EXISTS promotion_decisions (
    id UUID PRIMARY KEY,
    candidate_id UUID NOT NULL UNIQUE REFERENCES deployment_candidates(id) ON DELETE CASCADE,
    snapshot_id UUID NULL REFERENCES readiness_snapshots(id),
    decision TEXT NOT NULL,
    summary TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL
);

CREATE TABLE IF NOT EXISTS audit_events (
    id UUID PRIMARY KEY,
    actor_subject TEXT NOT NULL,
    actor_role TEXT NOT NULL,
    event_type TEXT NOT NULL,
    resource_type TEXT NOT NULL,
    resource_id UUID NOT NULL,
    request_id TEXT NOT NULL,
    details JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_audit_events_resource_created
    ON audit_events (resource_type, resource_id, created_at DESC);
