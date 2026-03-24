package httpx

import (
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
