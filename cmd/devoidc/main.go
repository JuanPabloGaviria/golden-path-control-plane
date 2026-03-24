package main

import (
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/juanpablogaviria/golden-path-control-plane/internal/devoidc"
)

func main() {
	cfg, err := loadConfig()
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}

	issuer, err := devoidc.New(cfg)
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}

	server := &http.Server{
		Addr:              cfg.Addr,
		Handler:           issuer.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
	}

	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}
}

func loadConfig() (devoidc.Config, error) {
	cfg := devoidc.Config{
		Addr:                 defaultValue("OIDC_DEV_ADDR", devoidc.DefaultAddr),
		IssuerURL:            defaultValue("OIDC_DEV_ISSUER_URL", devoidc.DefaultIssuerURL),
		Audience:             defaultValue("OIDC_DEV_AUDIENCE", devoidc.DefaultAudience),
		EngineerSubject:      defaultValue("OIDC_DEV_ENGINEER_SUBJECT", devoidc.DefaultEngineerSubject),
		PlatformAdminSubject: defaultValue("OIDC_DEV_PLATFORM_ADMIN_SUBJECT", devoidc.DefaultPlatformAdminSubject),
		TokenTTL:             time.Hour,
	}

	if rawTTL := strings.TrimSpace(os.Getenv("OIDC_DEV_TOKEN_TTL")); rawTTL != "" {
		ttl, err := time.ParseDuration(rawTTL)
		if err != nil {
			return devoidc.Config{}, fmt.Errorf("OIDC_DEV_TOKEN_TTL must be a valid duration: %w", err)
		}
		cfg.TokenTTL = ttl
	}

	return cfg, nil
}

func defaultValue(key, fallback string) string {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}

	return value
}
