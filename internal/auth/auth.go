package auth

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/juanpablogaviria/golden-path-control-plane/internal/config"
	"github.com/juanpablogaviria/golden-path-control-plane/internal/domain"
)

var ErrUnauthorized = errors.New("auth: unauthorized")

type Principal struct {
	Subject string      `json:"subject"`
	Role    domain.Role `json:"role"`
	Issuer  string      `json:"issuer"`
}

type Validator interface {
	Validate(ctx context.Context, rawToken string) (Principal, error)
}

type Claims struct {
	Role string `json:"role"`
	jwt.RegisteredClaims
}

func NewValidator(ctx context.Context, cfg config.AuthConfig) (Validator, error) {
	switch cfg.Mode {
	case "hmac":
		return &hmacValidator{cfg: cfg}, nil
	case "oidc":
		return newOIDCValidator(ctx, cfg)
	default:
		return nil, fmt.Errorf("auth: unsupported mode %q", cfg.Mode)
	}
}

func ParseBearerToken(header string) (string, error) {
	value := strings.TrimSpace(header)
	if value == "" {
		return "", ErrUnauthorized
	}

	parts := strings.SplitN(value, " ", 2)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") || strings.TrimSpace(parts[1]) == "" {
		return "", ErrUnauthorized
	}

	return strings.TrimSpace(parts[1]), nil
}

func IssueHMACToken(cfg config.AuthConfig, subject string, role domain.Role, ttl time.Duration) (string, error) {
	if cfg.Mode != "hmac" {
		return "", errors.New("auth: HMAC token issuance requires AUTH_MODE=hmac")
	}

	if subject == "" {
		return "", errors.New("auth: subject is required")
	}

	if role != domain.RoleEngineer && role != domain.RolePlatformAdmin {
		return "", fmt.Errorf("auth: unsupported role %q", role)
	}

	now := time.Now().UTC()
	claims := Claims{
		Role: string(role),
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   subject,
			Issuer:    cfg.Issuer,
			Audience:  jwt.ClaimStrings{cfg.Audience},
			ExpiresAt: jwt.NewNumericDate(now.Add(ttl)),
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now.Add(-1 * time.Minute)),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(cfg.HMACSecret))
}

type hmacValidator struct {
	cfg config.AuthConfig
}

func (v *hmacValidator) Validate(_ context.Context, rawToken string) (Principal, error) {
	token, err := jwt.ParseWithClaims(rawToken, &Claims{}, func(token *jwt.Token) (any, error) {
		if token.Method != jwt.SigningMethodHS256 {
			return nil, fmt.Errorf("auth: unexpected signing method %s", token.Method.Alg())
		}

		return []byte(v.cfg.HMACSecret), nil
	}, jwt.WithAudience(v.cfg.Audience), jwt.WithIssuer(v.cfg.Issuer))
	if err != nil {
		return Principal{}, fmt.Errorf("%w: %v", ErrUnauthorized, err)
	}

	claims, ok := token.Claims.(*Claims)
	if !ok || !token.Valid {
		return Principal{}, ErrUnauthorized
	}

	role := domain.Role(claims.Role)
	if role != domain.RoleEngineer && role != domain.RolePlatformAdmin {
		return Principal{}, fmt.Errorf("%w: invalid role claim", ErrUnauthorized)
	}

	return Principal{
		Subject: claims.Subject,
		Role:    role,
		Issuer:  claims.Issuer,
	}, nil
}
