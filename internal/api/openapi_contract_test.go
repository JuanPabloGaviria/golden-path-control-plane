package api

import (
	"context"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
)

func TestOpenAPIContractIsValid(t *testing.T) {
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("failed to determine current file path")
	}

	repoRoot := filepath.Dir(filepath.Dir(filepath.Dir(currentFile)))
	specPath := filepath.Join(repoRoot, "openapi", "openapi.yaml")

	loader := openapi3.NewLoader()
	document, err := loader.LoadFromFile(specPath)
	if err != nil {
		t.Fatalf("LoadFromFile returned error: %v", err)
	}

	if err := document.Validate(context.Background()); err != nil {
		t.Fatalf("Validate returned error: %v", err)
	}
}
