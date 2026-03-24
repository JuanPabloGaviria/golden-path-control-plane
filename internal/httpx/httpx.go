package httpx

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/google/uuid"
)

type ctxKey string

const requestIDKey ctxKey = "request_id"

type ErrorEnvelope struct {
	Error ErrorBody `json:"error"`
}

type ErrorBody struct {
	Code        string         `json:"code"`
	Message     string         `json:"message"`
	RequestID   string         `json:"request_id"`
	Remediation string         `json:"remediation,omitempty"`
	Details     map[string]any `json:"details,omitempty"`
}

func DecodeJSON(r *http.Request, target any) error {
	defer func() {
		_ = r.Body.Close()
	}()

	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()

	if err := decoder.Decode(target); err != nil {
		return err
	}

	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return errors.New("request body must contain a single JSON document")
	}

	return nil
}

func WriteJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func RespondError(w http.ResponseWriter, r *http.Request, status int, code, message string, details map[string]any) {
	RespondErrorWithRemediation(w, r, status, code, message, defaultRemediation(code), details)
}

func RespondErrorWithRemediation(w http.ResponseWriter, r *http.Request, status int, code, message, remediation string, details map[string]any) {
	WriteJSON(w, status, ErrorEnvelope{
		Error: ErrorBody{
			Code:        code,
			Message:     message,
			RequestID:   RequestIDFromContext(r.Context()),
			Remediation: remediation,
			Details:     details,
		},
	})
}

func WithRequestID(ctx context.Context, requestID string) context.Context {
	return context.WithValue(ctx, requestIDKey, requestID)
}

func RequestIDFromContext(ctx context.Context) string {
	value, _ := ctx.Value(requestIDKey).(string)
	return value
}

func EnsureRequestID(value string) string {
	if value != "" {
		return value
	}

	return uuid.NewString()
}

func defaultRemediation(code string) string {
	switch code {
	case "invalid_json":
		return "Send exactly one JSON document that matches the endpoint schema and retry."
	case "invalid_service_id", "invalid_candidate_id", "invalid_resource_id":
		return "Provide a valid UUID for the requested resource and retry."
	case "invalid_limit":
		return "Provide an integer limit value and retry."
	case "missing_bearer_token":
		return "Set an Authorization header with a Bearer token and retry."
	case "unauthorized":
		return "Request a valid token with the expected issuer, audience, expiry, and role claims, then retry."
	case "forbidden":
		return "Use a principal with the required role for this endpoint and retry."
	case "validation_failed":
		return "Correct the request payload so it satisfies the API validation rules, then retry."
	case "not_found":
		return "Verify the resource identifier exists in the current environment and retry."
	case "state_conflict", "conflict":
		return "Refresh the resource state, resolve the conflict, and retry with a new request if needed."
	case "database_unavailable":
		return "Restore database connectivity and retry once the runtime is healthy."
	default:
		return "Retry after correcting the request or investigating the attached error details."
	}
}
