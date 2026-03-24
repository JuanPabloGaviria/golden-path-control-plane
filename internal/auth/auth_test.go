package auth

import (
	"context"
	"testing"
	"time"

	"github.com/juanpablogaviria/golden-path-control-plane/internal/config"
	"github.com/juanpablogaviria/golden-path-control-plane/internal/domain"
)

func TestIssueAndValidateHMACToken(t *testing.T) {
	authCfg := config.AuthConfig{
		Mode:       "hmac",
		Audience:   "golden-path-control-plane",
		Issuer:     "golden-path-local",
		HMACSecret: "12345678901234567890123456789012",
	}

	token, err := IssueHMACToken(authCfg, "developer@example.com", domain.RoleEngineer, time.Hour)
	if err != nil {
		t.Fatalf("IssueHMACToken returned error: %v", err)
	}

	validator, err := NewValidator(context.Background(), authCfg)
	if err != nil {
		t.Fatalf("NewValidator returned error: %v", err)
	}

	principal, err := validator.Validate(context.Background(), token)
	if err != nil {
		t.Fatalf("Validate returned error: %v", err)
	}

	if principal.Subject != "developer@example.com" {
		t.Fatalf("expected subject developer@example.com, got %s", principal.Subject)
	}

	if principal.Role != domain.RoleEngineer {
		t.Fatalf("expected role engineer, got %s", principal.Role)
	}
}

func TestIssueHMACTokenRejectsNonPositiveTTL(t *testing.T) {
	authCfg := config.AuthConfig{
		Mode:       "hmac",
		Audience:   "golden-path-control-plane",
		Issuer:     "golden-path-local",
		HMACSecret: "12345678901234567890123456789012",
	}

	if _, err := IssueHMACToken(authCfg, "developer@example.com", domain.RoleEngineer, 0); err == nil {
		t.Fatal("expected IssueHMACToken to reject zero TTL")
	}
}
