package config

import (
	"strings"
	"testing"
)

func TestLoadRejectsPlaceholderSecret(t *testing.T) {
	setValidEnv(t)
	t.Setenv("AUTH_HMAC_SECRET", "replace-me-with-a-32-character-minimum-secret")

	_, err := Load()
	if err == nil {
		t.Fatal("expected Load to fail for placeholder auth secret")
	}

	if !strings.Contains(err.Error(), "placeholder") {
		t.Fatalf("expected placeholder error, got %v", err)
	}
}

func TestLoadRedactsSecrets(t *testing.T) {
	setValidEnv(t)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}

	redacted := cfg.Redacted()
	auth, ok := redacted["auth"].(map[string]any)
	if !ok {
		t.Fatal("expected auth config to be present in redacted output")
	}

	if auth["hmac_secret"] != "***redacted***" {
		t.Fatalf("expected redacted secret, got %#v", auth["hmac_secret"])
	}
}

func TestLoadTokenIssuerConfigDoesNotRequireDatabase(t *testing.T) {
	setValidEnv(t)
	t.Setenv("DATABASE_URL", "")

	cfg, err := LoadTokenIssuerConfig()
	if err != nil {
		t.Fatalf("LoadTokenIssuerConfig returned error: %v", err)
	}

	if cfg.Auth.Mode != "hmac" {
		t.Fatalf("expected hmac auth mode, got %s", cfg.Auth.Mode)
	}
}

func TestLoadRejectsProductionHMAC(t *testing.T) {
	setValidEnv(t)
	t.Setenv("APP_ENV", "production")
	t.Setenv("DATABASE_URL", "postgres://controlplane:secret@db.internal:5432/controlplane?sslmode=require")

	_, err := Load()
	if err == nil {
		t.Fatal("expected Load to reject production hmac auth")
	}

	if !strings.Contains(err.Error(), "AUTH_MODE=hmac") {
		t.Fatalf("expected production auth error, got %v", err)
	}
}

func TestLoadRejectsProductionDatabaseWithoutTLS(t *testing.T) {
	setValidEnv(t)
	t.Setenv("APP_ENV", "production")
	t.Setenv("AUTH_MODE", "oidc")
	t.Setenv("AUTH_HMAC_SECRET", "")
	t.Setenv("AUTH_OIDC_ISSUER_URL", "https://issuer.example.com")
	t.Setenv("DATABASE_URL", "postgres://controlplane:secret@db.internal:5432/controlplane?sslmode=disable")

	_, err := Load()
	if err == nil {
		t.Fatal("expected Load to reject production database without TLS")
	}

	if !strings.Contains(err.Error(), "sslmode") {
		t.Fatalf("expected sslmode error, got %v", err)
	}
}

func setValidEnv(t *testing.T) {
	t.Helper()

	t.Setenv("APP_ENV", "development")
	t.Setenv("APP_LOG_LEVEL", "INFO")
	t.Setenv("HTTP_ADDR", ":8080")
	t.Setenv("HTTP_READ_TIMEOUT", "10s")
	t.Setenv("HTTP_WRITE_TIMEOUT", "15s")
	t.Setenv("HTTP_IDLE_TIMEOUT", "60s")
	t.Setenv("SHUTDOWN_TIMEOUT", "20s")
	t.Setenv("DATABASE_URL", "postgres://controlplane:secret@localhost:5432/controlplane?sslmode=disable")
	t.Setenv("DATABASE_MAX_OPEN_CONNS", "10")
	t.Setenv("DATABASE_MIN_IDLE_CONNS", "2")
	t.Setenv("DATABASE_MAX_CONN_LIFETIME", "30m")
	t.Setenv("DATABASE_MAX_CONN_IDLE_TIME", "5m")
	t.Setenv("WORKER_POLL_INTERVAL", "2s")
	t.Setenv("WORKER_BATCH_SIZE", "5")
	t.Setenv("JOB_LEASE_DURATION", "30s")
	t.Setenv("JOB_MAX_ATTEMPTS", "5")
	t.Setenv("AUTH_MODE", "hmac")
	t.Setenv("AUTH_AUDIENCE", "golden-path-control-plane")
	t.Setenv("AUTH_ISSUER", "golden-path-local")
	t.Setenv("AUTH_HMAC_SECRET", "12345678901234567890123456789012")
	t.Setenv("AUTH_OIDC_ISSUER_URL", "")
	t.Setenv("AUTH_OIDC_JWKS_URL", "")
	t.Setenv("OTEL_SERVICE_NAME", "golden-path-control-plane")
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "")
	t.Setenv("PROMETHEUS_NAMESPACE", "goldenpath")
}
