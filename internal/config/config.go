package config

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	authModeHMAC = "hmac"
	authModeOIDC = "oidc"
)

type Config struct {
	AppEnv            string
	AppLogLevel       string
	HTTPAddr          string
	HTTPReadTimeout   time.Duration
	HTTPWriteTimeout  time.Duration
	HTTPIdleTimeout   time.Duration
	ShutdownTimeout   time.Duration
	Database          DatabaseConfig
	Worker            WorkerConfig
	Auth              AuthConfig
	Observability     ObservabilityConfig
	IntegrationTestDB string
}

type TokenIssuerConfig struct {
	Auth AuthConfig
}

type DatabaseConfig struct {
	URL             string
	MaxOpenConns    int
	MinIdleConns    int
	MaxConnLifetime time.Duration
	MaxConnIdleTime time.Duration
}

type WorkerConfig struct {
	PollInterval time.Duration
	BatchSize    int
	Lease        time.Duration
	MaxAttempts  int
}

type AuthConfig struct {
	Mode          string
	Audience      string
	Issuer        string
	HMACSecret    string
	OIDCIssuerURL string
	OIDCJWKSURL   string
}

type ObservabilityConfig struct {
	ServiceName  string
	OTLPEndpoint string
	Namespace    string
}

func Load() (Config, error) {
	cfg := Config{
		AppEnv:            getDefault("APP_ENV", "development"),
		AppLogLevel:       strings.ToUpper(getDefault("APP_LOG_LEVEL", "INFO")),
		HTTPAddr:          getDefault("HTTP_ADDR", ":8080"),
		IntegrationTestDB: strings.TrimSpace(os.Getenv("CONTROL_PLANE_INTEGRATION_DATABASE_URL")),
	}

	var err error

	cfg.HTTPReadTimeout, err = getDuration("HTTP_READ_TIMEOUT", 10*time.Second)
	if err != nil {
		return Config{}, err
	}

	cfg.HTTPWriteTimeout, err = getDuration("HTTP_WRITE_TIMEOUT", 15*time.Second)
	if err != nil {
		return Config{}, err
	}

	cfg.HTTPIdleTimeout, err = getDuration("HTTP_IDLE_TIMEOUT", 60*time.Second)
	if err != nil {
		return Config{}, err
	}

	cfg.ShutdownTimeout, err = getDuration("SHUTDOWN_TIMEOUT", 20*time.Second)
	if err != nil {
		return Config{}, err
	}

	cfg.Database, err = loadDatabase()
	if err != nil {
		return Config{}, err
	}

	cfg.Worker, err = loadWorker()
	if err != nil {
		return Config{}, err
	}

	cfg.Auth, err = loadAuth()
	if err != nil {
		return Config{}, err
	}

	cfg.Observability = ObservabilityConfig{
		ServiceName:  getDefault("OTEL_SERVICE_NAME", "golden-path-control-plane"),
		OTLPEndpoint: strings.TrimSpace(os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT")),
		Namespace:    getDefault("PROMETHEUS_NAMESPACE", "goldenpath"),
	}

	if err := cfg.validate(); err != nil {
		return Config{}, err
	}

	return cfg, nil
}

func LoadTokenIssuerConfig() (TokenIssuerConfig, error) {
	authCfg, err := loadAuth()
	if err != nil {
		return TokenIssuerConfig{}, err
	}

	return TokenIssuerConfig{Auth: authCfg}, nil
}

func (c Config) Redacted() map[string]any {
	return map[string]any{
		"app_env":            c.AppEnv,
		"app_log_level":      c.AppLogLevel,
		"http_addr":          c.HTTPAddr,
		"http_read_timeout":  c.HTTPReadTimeout.String(),
		"http_write_timeout": c.HTTPWriteTimeout.String(),
		"http_idle_timeout":  c.HTTPIdleTimeout.String(),
		"shutdown_timeout":   c.ShutdownTimeout.String(),
		"database": map[string]any{
			"url":                redactURL(c.Database.URL),
			"max_open_conns":     c.Database.MaxOpenConns,
			"min_idle_conns":     c.Database.MinIdleConns,
			"max_conn_lifetime":  c.Database.MaxConnLifetime.String(),
			"max_conn_idle_time": c.Database.MaxConnIdleTime.String(),
		},
		"worker": map[string]any{
			"poll_interval": c.Worker.PollInterval.String(),
			"batch_size":    c.Worker.BatchSize,
			"lease":         c.Worker.Lease.String(),
			"max_attempts":  c.Worker.MaxAttempts,
		},
		"auth": map[string]any{
			"mode":            c.Auth.Mode,
			"audience":        c.Auth.Audience,
			"issuer":          c.Auth.Issuer,
			"hmac_secret":     redactSecret(c.Auth.HMACSecret),
			"oidc_issuer_url": c.Auth.OIDCIssuerURL,
			"oidc_jwks_url":   c.Auth.OIDCJWKSURL,
		},
		"observability": map[string]any{
			"service_name":  c.Observability.ServiceName,
			"otlp_endpoint": c.Observability.OTLPEndpoint,
			"namespace":     c.Observability.Namespace,
		},
	}
}

func (c Config) validate() error {
	switch c.AppEnv {
	case "development", "test", "production":
	default:
		return fmt.Errorf("config: APP_ENV must be one of development|test|production, got %q", c.AppEnv)
	}

	switch c.AppLogLevel {
	case "DEBUG", "INFO", "WARN", "ERROR":
	default:
		return fmt.Errorf("config: APP_LOG_LEVEL must be one of DEBUG|INFO|WARN|ERROR, got %q", c.AppLogLevel)
	}

	if c.HTTPAddr == "" {
		return errors.New("config: HTTP_ADDR must not be empty")
	}

	if c.HTTPReadTimeout <= 0 || c.HTTPWriteTimeout <= 0 || c.HTTPIdleTimeout <= 0 || c.ShutdownTimeout <= 0 {
		return errors.New("config: HTTP and shutdown timeouts must be greater than zero")
	}

	if c.Observability.ServiceName == "" {
		return errors.New("config: OTEL_SERVICE_NAME must not be empty")
	}

	if c.Observability.Namespace == "" {
		return errors.New("config: PROMETHEUS_NAMESPACE must not be empty")
	}

	if c.Worker.BatchSize < 1 {
		return errors.New("config: WORKER_BATCH_SIZE must be greater than zero")
	}

	if c.Worker.PollInterval <= 0 || c.Worker.Lease <= 0 {
		return errors.New("config: WORKER_POLL_INTERVAL and JOB_LEASE_DURATION must be greater than zero")
	}

	if c.Worker.MaxAttempts < 1 {
		return errors.New("config: JOB_MAX_ATTEMPTS must be greater than zero")
	}

	if c.Auth.Audience == "" {
		return errors.New("config: AUTH_AUDIENCE must not be empty")
	}

	if c.Auth.Issuer == "" {
		return errors.New("config: AUTH_ISSUER must not be empty")
	}

	if c.AppEnv == "production" {
		if c.Auth.Mode == authModeHMAC {
			return errors.New("config: AUTH_MODE=hmac is not allowed when APP_ENV=production")
		}

		if err := validateProductionDatabaseURL(c.Database.URL); err != nil {
			return err
		}
	}

	return nil
}

func loadDatabase() (DatabaseConfig, error) {
	cfg := DatabaseConfig{
		URL: strings.TrimSpace(os.Getenv("DATABASE_URL")),
	}

	if cfg.URL == "" {
		return DatabaseConfig{}, errors.New("config: DATABASE_URL must not be empty")
	}

	var err error

	cfg.MaxOpenConns, err = getInt("DATABASE_MAX_OPEN_CONNS", 10)
	if err != nil {
		return DatabaseConfig{}, err
	}

	cfg.MinIdleConns, err = getInt("DATABASE_MIN_IDLE_CONNS", 2)
	if err != nil {
		return DatabaseConfig{}, err
	}

	cfg.MaxConnLifetime, err = getDuration("DATABASE_MAX_CONN_LIFETIME", 30*time.Minute)
	if err != nil {
		return DatabaseConfig{}, err
	}

	cfg.MaxConnIdleTime, err = getDuration("DATABASE_MAX_CONN_IDLE_TIME", 5*time.Minute)
	if err != nil {
		return DatabaseConfig{}, err
	}

	if cfg.MaxOpenConns < 1 {
		return DatabaseConfig{}, errors.New("config: DATABASE_MAX_OPEN_CONNS must be greater than zero")
	}

	if cfg.MinIdleConns < 0 {
		return DatabaseConfig{}, errors.New("config: DATABASE_MIN_IDLE_CONNS must be zero or greater")
	}

	if cfg.MinIdleConns > cfg.MaxOpenConns {
		return DatabaseConfig{}, errors.New("config: DATABASE_MIN_IDLE_CONNS must not exceed DATABASE_MAX_OPEN_CONNS")
	}

	if cfg.MaxConnLifetime <= 0 || cfg.MaxConnIdleTime <= 0 {
		return DatabaseConfig{}, errors.New("config: database connection lifetimes must be greater than zero")
	}

	return cfg, nil
}

func loadWorker() (WorkerConfig, error) {
	cfg := WorkerConfig{}

	var err error

	cfg.PollInterval, err = getDuration("WORKER_POLL_INTERVAL", 2*time.Second)
	if err != nil {
		return WorkerConfig{}, err
	}

	cfg.BatchSize, err = getInt("WORKER_BATCH_SIZE", 5)
	if err != nil {
		return WorkerConfig{}, err
	}

	cfg.Lease, err = getDuration("JOB_LEASE_DURATION", 30*time.Second)
	if err != nil {
		return WorkerConfig{}, err
	}

	cfg.MaxAttempts, err = getInt("JOB_MAX_ATTEMPTS", 5)
	if err != nil {
		return WorkerConfig{}, err
	}

	return cfg, nil
}

func loadAuth() (AuthConfig, error) {
	mode := strings.ToLower(getDefault("AUTH_MODE", authModeHMAC))

	cfg := AuthConfig{
		Mode:          mode,
		Audience:      strings.TrimSpace(os.Getenv("AUTH_AUDIENCE")),
		Issuer:        strings.TrimSpace(os.Getenv("AUTH_ISSUER")),
		HMACSecret:    strings.TrimSpace(os.Getenv("AUTH_HMAC_SECRET")),
		OIDCIssuerURL: strings.TrimSpace(os.Getenv("AUTH_OIDC_ISSUER_URL")),
		OIDCJWKSURL:   strings.TrimSpace(os.Getenv("AUTH_OIDC_JWKS_URL")),
	}

	if cfg.Audience == "" {
		return AuthConfig{}, errors.New("config: AUTH_AUDIENCE must not be empty")
	}

	if cfg.Issuer == "" {
		return AuthConfig{}, errors.New("config: AUTH_ISSUER must not be empty")
	}

	switch cfg.Mode {
	case authModeHMAC:
		if cfg.HMACSecret == "" {
			return AuthConfig{}, errors.New("config: AUTH_HMAC_SECRET is required when AUTH_MODE=hmac")
		}

		if len(cfg.HMACSecret) < 32 {
			return AuthConfig{}, errors.New("config: AUTH_HMAC_SECRET must be at least 32 characters")
		}

		if strings.Contains(strings.ToLower(cfg.HMACSecret), "replace-me") {
			return AuthConfig{}, errors.New("config: AUTH_HMAC_SECRET contains a placeholder value")
		}
	case authModeOIDC:
		if cfg.OIDCIssuerURL == "" {
			return AuthConfig{}, errors.New("config: AUTH_OIDC_ISSUER_URL is required when AUTH_MODE=oidc")
		}
	default:
		return AuthConfig{}, fmt.Errorf("config: AUTH_MODE must be one of %s|%s, got %q", authModeHMAC, authModeOIDC, cfg.Mode)
	}

	return cfg, nil
}

func getDefault(key, fallback string) string {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}

	return value
}

func getInt(key string, fallback int) (int, error) {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback, nil
	}

	parsed, err := strconv.Atoi(value)
	if err != nil {
		return 0, fmt.Errorf("config: %s must be an integer: %w", key, err)
	}

	return parsed, nil
}

func getDuration(key string, fallback time.Duration) (time.Duration, error) {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback, nil
	}

	parsed, err := time.ParseDuration(value)
	if err != nil {
		return 0, fmt.Errorf("config: %s must be a valid duration: %w", key, err)
	}

	return parsed, nil
}

func redactSecret(value string) string {
	if value == "" {
		return ""
	}

	return "***redacted***"
}

func redactURL(value string) string {
	if value == "" {
		return ""
	}

	at := strings.LastIndex(value, "@")
	if at == -1 {
		return value
	}

	schemeSplit := strings.Index(value, "://")
	if schemeSplit == -1 {
		return "***redacted***"
	}

	return value[:schemeSplit+3] + "***redacted***" + value[at:]
}

func validateProductionDatabaseURL(rawURL string) error {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("config: DATABASE_URL must be a valid URL in production: %w", err)
	}

	sslMode := strings.TrimSpace(parsed.Query().Get("sslmode"))
	if sslMode == "" || strings.EqualFold(sslMode, "disable") {
		return errors.New("config: DATABASE_URL must set sslmode to a non-disable value when APP_ENV=production")
	}

	return nil
}
