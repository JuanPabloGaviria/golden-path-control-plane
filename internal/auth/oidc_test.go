package auth

import (
	"context"
	"net"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/juanpablogaviria/golden-path-control-plane/internal/config"
	"github.com/juanpablogaviria/golden-path-control-plane/internal/devoidc"
	"github.com/juanpablogaviria/golden-path-control-plane/internal/domain"
)

func TestOIDCValidatorAcceptsValidToken(t *testing.T) {
	issuer, issuerURL := startOIDCTestIssuer(t, devoidc.DefaultAudience)
	token, err := issuer.SignToken(devoidc.TokenClaims{
		Subject: "developer@example.com",
		Role:    string(domain.RoleEngineer),
		TTL:     time.Hour,
	})
	if err != nil {
		t.Fatalf("SignToken returned error: %v", err)
	}

	validator, err := NewValidator(context.Background(), config.AuthConfig{
		Mode:          "oidc",
		Audience:      devoidc.DefaultAudience,
		Issuer:        issuerURL,
		OIDCIssuerURL: issuerURL,
	})
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

func TestOIDCValidatorRejectsWrongAudience(t *testing.T) {
	issuer, issuerURL := startOIDCTestIssuer(t, devoidc.DefaultAudience)
	token, err := issuer.SignToken(devoidc.TokenClaims{
		Subject: "developer@example.com",
		Role:    string(domain.RoleEngineer),
		TTL:     time.Hour,
	})
	if err != nil {
		t.Fatalf("SignToken returned error: %v", err)
	}

	validator, err := NewValidator(context.Background(), config.AuthConfig{
		Mode:          "oidc",
		Audience:      "unexpected-audience",
		Issuer:        issuerURL,
		OIDCIssuerURL: issuerURL,
	})
	if err != nil {
		t.Fatalf("NewValidator returned error: %v", err)
	}

	if _, err := validator.Validate(context.Background(), token); err == nil {
		t.Fatal("expected Validate to reject wrong audience")
	}
}

func TestOIDCValidatorRejectsWrongIssuer(t *testing.T) {
	issuer, issuerURL := startOIDCTestIssuer(t, devoidc.DefaultAudience)
	token, err := issuer.SignToken(devoidc.TokenClaims{
		Subject: "developer@example.com",
		Role:    string(domain.RoleEngineer),
		TTL:     time.Hour,
	})
	if err != nil {
		t.Fatalf("SignToken returned error: %v", err)
	}

	validator, err := NewValidator(context.Background(), config.AuthConfig{
		Mode:          "oidc",
		Audience:      devoidc.DefaultAudience,
		Issuer:        "http://unexpected-issuer",
		OIDCIssuerURL: "http://unexpected-issuer",
		OIDCJWKSURL:   issuerURL + "/keys",
	})
	if err != nil {
		t.Fatalf("NewValidator returned error: %v", err)
	}

	if _, err := validator.Validate(context.Background(), token); err == nil {
		t.Fatal("expected Validate to reject wrong issuer")
	}
}

func TestOIDCValidatorRejectsExpiredToken(t *testing.T) {
	issuer, issuerURL := startOIDCTestIssuer(t, devoidc.DefaultAudience)
	token, err := issuer.SignToken(devoidc.TokenClaims{
		Subject: "developer@example.com",
		Role:    string(domain.RoleEngineer),
		TTL:     -1 * time.Minute,
	})
	if err != nil {
		t.Fatalf("SignToken returned error: %v", err)
	}

	validator, err := NewValidator(context.Background(), config.AuthConfig{
		Mode:          "oidc",
		Audience:      devoidc.DefaultAudience,
		Issuer:        issuerURL,
		OIDCIssuerURL: issuerURL,
	})
	if err != nil {
		t.Fatalf("NewValidator returned error: %v", err)
	}

	if _, err := validator.Validate(context.Background(), token); err == nil {
		t.Fatal("expected Validate to reject expired token")
	}
}

func TestOIDCValidatorRejectsMissingRoleClaim(t *testing.T) {
	issuer, issuerURL := startOIDCTestIssuer(t, devoidc.DefaultAudience)
	now := time.Now().UTC()
	token, err := issuer.SignMapClaims(jwt.MapClaims{
		"iss": issuerURL,
		"sub": "developer@example.com",
		"aud": []string{devoidc.DefaultAudience},
		"exp": now.Add(time.Hour).Unix(),
		"iat": now.Unix(),
		"nbf": now.Add(-1 * time.Minute).Unix(),
	})
	if err != nil {
		t.Fatalf("SignMapClaims returned error: %v", err)
	}

	validator, err := NewValidator(context.Background(), config.AuthConfig{
		Mode:          "oidc",
		Audience:      devoidc.DefaultAudience,
		Issuer:        issuerURL,
		OIDCIssuerURL: issuerURL,
	})
	if err != nil {
		t.Fatalf("NewValidator returned error: %v", err)
	}

	if _, err := validator.Validate(context.Background(), token); err == nil {
		t.Fatal("expected Validate to reject missing role claim")
	}
}

func TestOIDCValidatorRejectsUnknownRoleClaim(t *testing.T) {
	issuer, issuerURL := startOIDCTestIssuer(t, devoidc.DefaultAudience)
	token, err := issuer.SignToken(devoidc.TokenClaims{
		Subject: "developer@example.com",
		Role:    "viewer",
		TTL:     time.Hour,
	})
	if err != nil {
		t.Fatalf("SignToken returned error: %v", err)
	}

	validator, err := NewValidator(context.Background(), config.AuthConfig{
		Mode:          "oidc",
		Audience:      devoidc.DefaultAudience,
		Issuer:        issuerURL,
		OIDCIssuerURL: issuerURL,
	})
	if err != nil {
		t.Fatalf("NewValidator returned error: %v", err)
	}

	if _, err := validator.Validate(context.Background(), token); err == nil {
		t.Fatal("expected Validate to reject unknown role claim")
	}
}

func startOIDCTestIssuer(t *testing.T, audience string) (*devoidc.Issuer, string) {
	t.Helper()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Listen returned error: %v", err)
	}

	issuerURL := "http://" + listener.Addr().String()
	issuer, err := devoidc.New(devoidc.Config{
		IssuerURL: issuerURL,
		Audience:  audience,
		TokenTTL:  time.Hour,
	})
	if err != nil {
		t.Fatalf("devoidc.New returned error: %v", err)
	}

	server := httptest.NewUnstartedServer(issuer.Handler())
	server.Listener = listener
	server.Start()
	t.Cleanup(server.Close)

	return issuer, issuerURL
}
