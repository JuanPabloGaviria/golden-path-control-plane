package api

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"

	"github.com/juanpablogaviria/golden-path-control-plane/internal/app"
	"github.com/juanpablogaviria/golden-path-control-plane/internal/auth"
	"github.com/juanpablogaviria/golden-path-control-plane/internal/domain"
	"github.com/juanpablogaviria/golden-path-control-plane/internal/httpx"
	"github.com/juanpablogaviria/golden-path-control-plane/internal/observability"
	"github.com/juanpablogaviria/golden-path-control-plane/internal/postgres"
)

type Server struct {
	logger    *slog.Logger
	service   *app.ControlPlane
	validator auth.Validator
	repo      *postgres.Store
	metrics   *observability.Metrics
}

type contextKey string

const principalKey contextKey = "principal"

func NewHandler(logger *slog.Logger, service *app.ControlPlane, validator auth.Validator, repo *postgres.Store, metrics *observability.Metrics) http.Handler {
	server := &Server{
		logger:    logger,
		service:   service,
		validator: validator,
		repo:      repo,
		metrics:   metrics,
	}

	router := chi.NewRouter()
	router.Use(chimiddleware.Recoverer)
	router.Use(httpx.RequestIDMiddleware)
	router.Use(metricsMiddleware(metrics))
	router.Use(httpx.LoggingMiddleware(logger))

	router.Get("/healthz", server.healthz)
	router.Get("/readyz", server.readyz)
	router.Handle("/metrics", promhttp.HandlerFor(metrics.Registry, promhttp.HandlerOpts{}))

	router.Group(func(r chi.Router) {
		r.Use(server.authMiddleware())

		r.Route("/v1", func(v1 chi.Router) {
			v1.With(server.authorize(domain.RoleEngineer, domain.RolePlatformAdmin)).Post("/services", server.registerService)
			v1.With(server.authorize(domain.RoleEngineer, domain.RolePlatformAdmin)).Patch("/services/{serviceID}", server.updateService)
			v1.With(server.authorize(domain.RoleEngineer, domain.RolePlatformAdmin)).Post("/services/{serviceID}/evaluations", server.queueEvaluation)
			v1.With(server.authorize(domain.RoleEngineer, domain.RolePlatformAdmin)).Get("/services/{serviceID}/scorecard", server.getScorecard)
			v1.With(server.authorize(domain.RoleEngineer, domain.RolePlatformAdmin)).Post("/deployment-candidates", server.createDeploymentCandidate)
			v1.With(server.authorize(domain.RoleEngineer, domain.RolePlatformAdmin)).Post("/deployment-candidates/{candidateID}/evaluate", server.evaluateDeploymentCandidate)
			v1.With(server.authorize(domain.RoleEngineer, domain.RolePlatformAdmin)).Get("/deployment-candidates/{candidateID}", server.getDeploymentCandidate)
			v1.With(server.authorize(domain.RolePlatformAdmin)).Get("/audit-events", server.listAuditEvents)
		})
	})

	return otelhttp.NewHandler(router, "http.server")
}

func (s *Server) registerService(w http.ResponseWriter, r *http.Request) {
	var input domain.ServiceInput
	if err := httpx.DecodeJSON(r, &input); err != nil {
		httpx.RespondError(w, r, http.StatusBadRequest, "invalid_json", "Request body must be valid JSON.", map[string]any{"cause": err.Error()})
		return
	}

	service, err := s.service.RegisterService(r.Context(), input, principalFromContext(r.Context()), httpx.RequestIDFromContext(r.Context()))
	if err != nil {
		s.writeError(w, r, err)
		return
	}

	httpx.WriteJSON(w, http.StatusCreated, service)
}

func (s *Server) updateService(w http.ResponseWriter, r *http.Request) {
	serviceID, err := parseUUIDParam(r, "serviceID")
	if err != nil {
		httpx.RespondError(w, r, http.StatusBadRequest, "invalid_service_id", err.Error(), nil)
		return
	}

	var patch domain.ServicePatch
	if err := httpx.DecodeJSON(r, &patch); err != nil {
		httpx.RespondError(w, r, http.StatusBadRequest, "invalid_json", "Request body must be valid JSON.", map[string]any{"cause": err.Error()})
		return
	}

	service, err := s.service.UpdateService(r.Context(), serviceID, patch, principalFromContext(r.Context()), httpx.RequestIDFromContext(r.Context()))
	if err != nil {
		s.writeError(w, r, err)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, service)
}

func (s *Server) queueEvaluation(w http.ResponseWriter, r *http.Request) {
	serviceID, err := parseUUIDParam(r, "serviceID")
	if err != nil {
		httpx.RespondError(w, r, http.StatusBadRequest, "invalid_service_id", err.Error(), nil)
		return
	}

	job, err := s.service.QueueServiceEvaluation(r.Context(), serviceID, r.Header.Get("Idempotency-Key"), principalFromContext(r.Context()), httpx.RequestIDFromContext(r.Context()))
	if err != nil {
		s.writeError(w, r, err)
		return
	}

	httpx.WriteJSON(w, http.StatusAccepted, map[string]any{
		"job_id":          job.ID,
		"status":          job.Status,
		"idempotency_key": job.IdempotencyKey,
	})
}

func (s *Server) getScorecard(w http.ResponseWriter, r *http.Request) {
	serviceID, err := parseUUIDParam(r, "serviceID")
	if err != nil {
		httpx.RespondError(w, r, http.StatusBadRequest, "invalid_service_id", err.Error(), nil)
		return
	}

	scorecard, err := s.service.GetScorecard(r.Context(), serviceID)
	if err != nil {
		s.writeError(w, r, err)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, scorecard)
}

func (s *Server) createDeploymentCandidate(w http.ResponseWriter, r *http.Request) {
	var input domain.DeploymentCandidateInput
	if err := httpx.DecodeJSON(r, &input); err != nil {
		httpx.RespondError(w, r, http.StatusBadRequest, "invalid_json", "Request body must be valid JSON.", map[string]any{"cause": err.Error()})
		return
	}

	candidate, err := s.service.CreateDeploymentCandidate(r.Context(), input, principalFromContext(r.Context()), httpx.RequestIDFromContext(r.Context()))
	if err != nil {
		s.writeError(w, r, err)
		return
	}

	httpx.WriteJSON(w, http.StatusCreated, candidate)
}

func (s *Server) evaluateDeploymentCandidate(w http.ResponseWriter, r *http.Request) {
	candidateID, err := parseUUIDParam(r, "candidateID")
	if err != nil {
		httpx.RespondError(w, r, http.StatusBadRequest, "invalid_candidate_id", err.Error(), nil)
		return
	}

	candidate, err := s.service.EvaluateDeploymentCandidate(r.Context(), candidateID, principalFromContext(r.Context()), httpx.RequestIDFromContext(r.Context()))
	if err != nil {
		s.writeError(w, r, err)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, candidate)
}

func (s *Server) getDeploymentCandidate(w http.ResponseWriter, r *http.Request) {
	candidateID, err := parseUUIDParam(r, "candidateID")
	if err != nil {
		httpx.RespondError(w, r, http.StatusBadRequest, "invalid_candidate_id", err.Error(), nil)
		return
	}

	candidate, err := s.service.GetDeploymentCandidate(r.Context(), candidateID)
	if err != nil {
		s.writeError(w, r, err)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, candidate)
}

func (s *Server) listAuditEvents(w http.ResponseWriter, r *http.Request) {
	resourceType := r.URL.Query().Get("resource_type")
	var resourceID *uuid.UUID
	if value := r.URL.Query().Get("resource_id"); value != "" {
		parsed, err := uuid.Parse(value)
		if err != nil {
			httpx.RespondError(w, r, http.StatusBadRequest, "invalid_resource_id", "resource_id must be a valid UUID.", nil)
			return
		}
		resourceID = &parsed
	}

	limit := 20
	if value := r.URL.Query().Get("limit"); value != "" {
		parsed, err := strconv.Atoi(value)
		if err != nil {
			httpx.RespondError(w, r, http.StatusBadRequest, "invalid_limit", "limit must be an integer.", nil)
			return
		}
		limit = parsed
	}

	events, err := s.service.ListAuditEvents(r.Context(), resourceType, resourceID, limit)
	if err != nil {
		s.writeError(w, r, err)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{"items": events})
}

func (s *Server) healthz(w http.ResponseWriter, r *http.Request) {
	httpx.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) readyz(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()

	if err := s.repo.Pool().Ping(ctx); err != nil {
		httpx.RespondError(w, r, http.StatusServiceUnavailable, "database_unavailable", "Database ping failed.", map[string]any{"cause": err.Error()})
		return
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]string{"status": "ready"})
}

func (s *Server) writeError(w http.ResponseWriter, r *http.Request, err error) {
	var validationErr app.ValidationError
	var stateConflictErr app.StateConflictError
	var pgErr *pgconn.PgError

	switch {
	case errors.Is(err, auth.ErrUnauthorized):
		httpx.RespondError(w, r, http.StatusUnauthorized, "unauthorized", "Authentication failed.", map[string]any{"cause": err.Error()})
	case errors.Is(err, postgres.ErrNotFound):
		httpx.RespondError(w, r, http.StatusNotFound, "not_found", "Requested resource was not found.", map[string]any{"cause": err.Error()})
	case errors.As(err, &validationErr):
		httpx.RespondError(w, r, http.StatusBadRequest, "validation_failed", validationErr.Error(), nil)
	case errors.As(err, &stateConflictErr) || errors.Is(err, postgres.ErrStateConflict):
		httpx.RespondError(w, r, http.StatusConflict, "state_conflict", err.Error(), nil)
	case errors.As(err, &pgErr) && pgErr.Code == "23505":
		httpx.RespondError(w, r, http.StatusConflict, "conflict", "Resource already exists or violates a uniqueness constraint.", map[string]any{"cause": pgErr.Message})
	default:
		s.logger.Error("request_failed", "request_id", httpx.RequestIDFromContext(r.Context()), "error", err.Error())
		httpx.RespondError(w, r, http.StatusInternalServerError, "internal_error", "The server could not complete the request.", nil)
	}
}

func (s *Server) authMiddleware() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			rawToken, err := auth.ParseBearerToken(r.Header.Get("Authorization"))
			if err != nil {
				httpx.RespondError(w, r, http.StatusUnauthorized, "missing_bearer_token", "Authorization header must contain a Bearer token.", nil)
				return
			}

			principal, err := s.validator.Validate(r.Context(), rawToken)
			if err != nil {
				httpx.RespondError(w, r, http.StatusUnauthorized, "unauthorized", "Bearer token validation failed.", map[string]any{"cause": err.Error()})
				return
			}

			next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), principalKey, principal)))
		})
	}
}

func (s *Server) authorize(roles ...domain.Role) func(http.Handler) http.Handler {
	allowed := map[domain.Role]struct{}{}
	for _, role := range roles {
		allowed[role] = struct{}{}
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			principal := principalFromContext(r.Context())
			if _, ok := allowed[principal.Role]; !ok {
				httpx.RespondError(w, r, http.StatusForbidden, "forbidden", "Caller is not authorized for this endpoint.", nil)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

func principalFromContext(ctx context.Context) auth.Principal {
	principal, _ := ctx.Value(principalKey).(auth.Principal)
	return principal
}

func metricsMiddleware(metrics *observability.Metrics) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			recorder := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
			start := time.Now()
			next.ServeHTTP(recorder, r)
			route := httpx.RoutePattern(r)

			metrics.HTTPRequestsTotal.WithLabelValues(route, r.Method, strconv.Itoa(recorder.status)).Inc()
			metrics.HTTPRequestLatency.WithLabelValues(route, r.Method).Observe(time.Since(start).Seconds())
		})
	}
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(status int) {
	r.status = status
	r.ResponseWriter.WriteHeader(status)
}

func parseUUIDParam(r *http.Request, name string) (uuid.UUID, error) {
	value := chi.URLParam(r, name)
	id, err := uuid.Parse(value)
	if err != nil {
		return uuid.Nil, err
	}

	return id, nil
}
