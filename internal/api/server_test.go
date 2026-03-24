package api

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/juanpablogaviria/golden-path-control-plane/internal/auth"
	"github.com/juanpablogaviria/golden-path-control-plane/internal/domain"
	"github.com/juanpablogaviria/golden-path-control-plane/internal/observability"
)

func TestMissingBearerTokenIsRejected(t *testing.T) {
	handler := newTestHandler(t, stubValidator{})

	request := httptest.NewRequest(http.MethodPost, "/v1/services", nil)
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", response.Code)
	}
}

func TestEngineerCannotListAuditEvents(t *testing.T) {
	handler := newTestHandler(t, stubValidator{
		principal: auth.Principal{
			Subject: "developer@example.com",
			Role:    domain.RoleEngineer,
			Issuer:  "test",
		},
	})

	request := httptest.NewRequest(http.MethodGet, "/v1/audit-events", nil)
	request.Header.Set("Authorization", "Bearer token")
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", response.Code)
	}
}

type stubValidator struct {
	principal auth.Principal
	err       error
}

func (s stubValidator) Validate(context.Context, string) (auth.Principal, error) {
	if s.err != nil {
		return auth.Principal{}, s.err
	}

	return s.principal, nil
}

func newTestHandler(t *testing.T, validator auth.Validator) http.Handler {
	t.Helper()

	metrics, err := observability.NewMetrics("test")
	if err != nil {
		t.Fatalf("NewMetrics returned error: %v", err)
	}

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	return NewHandler(logger, nil, validator, nil, metrics)
}

func TestUnauthorizedValidatorErrorMapsTo401(t *testing.T) {
	handler := newTestHandler(t, stubValidator{err: auth.ErrUnauthorized})

	request := httptest.NewRequest(http.MethodPost, "/v1/services", nil)
	request.Header.Set("Authorization", "Bearer token")
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", response.Code)
	}
}

func TestNonAuthValidatorErrorStillMapsTo401(t *testing.T) {
	handler := newTestHandler(t, stubValidator{err: errors.New("bad token")})

	request := httptest.NewRequest(http.MethodPost, "/v1/services", nil)
	request.Header.Set("Authorization", "Bearer token")
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", response.Code)
	}
}
