package httpx

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDecodeJSONRejectsMultipleDocuments(t *testing.T) {
	request := httptest.NewRequest("POST", "/v1/services", strings.NewReader(`{"name":"one"}{"name":"two"}`))
	var payload map[string]any

	if err := DecodeJSON(request, &payload); err == nil {
		t.Fatal("expected DecodeJSON to reject multiple JSON documents")
	}
}

func TestRespondErrorIncludesRemediation(t *testing.T) {
	request := httptest.NewRequest(http.MethodPost, "/v1/services", nil)
	request = request.WithContext(WithRequestID(request.Context(), "req-123"))
	response := httptest.NewRecorder()

	RespondError(response, request, http.StatusBadRequest, "invalid_json", "Request body must be valid JSON.", nil)

	var envelope ErrorEnvelope
	if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("json.Unmarshal returned error: %v", err)
	}

	if envelope.Error.RequestID != "req-123" {
		t.Fatalf("expected request_id req-123, got %s", envelope.Error.RequestID)
	}

	if envelope.Error.Remediation == "" {
		t.Fatal("expected remediation to be populated")
	}
}
