package auth

import (
	"context"
	"fmt"
	"strings"

	"github.com/coreos/go-oidc/v3/oidc"

	"github.com/juanpablogaviria/golden-path-control-plane/internal/config"
	"github.com/juanpablogaviria/golden-path-control-plane/internal/domain"
)

type oidcValidator struct {
	verifier *oidc.IDTokenVerifier
}

type oidcClaims struct {
	Role string `json:"role"`
	Sub  string `json:"sub"`
	Iss  string `json:"iss"`
}

func newOIDCValidator(ctx context.Context, cfg config.AuthConfig) (Validator, error) {
	if strings.HasPrefix(cfg.OIDCIssuerURL, "http://") {
		ctx = oidc.InsecureIssuerURLContext(ctx, cfg.OIDCIssuerURL)
	}

	var verifier *oidc.IDTokenVerifier
	if cfg.OIDCJWKSURL != "" {
		keySet := oidc.NewRemoteKeySet(ctx, cfg.OIDCJWKSURL)
		verifier = oidc.NewVerifier(cfg.OIDCIssuerURL, keySet, &oidc.Config{ClientID: cfg.Audience})
	} else {
		provider, err := oidc.NewProvider(ctx, cfg.OIDCIssuerURL)
		if err != nil {
			return nil, fmt.Errorf("auth: build oidc provider: %w", err)
		}
		verifier = provider.Verifier(&oidc.Config{ClientID: cfg.Audience})
	}

	return &oidcValidator{
		verifier: verifier,
	}, nil
}

func (v *oidcValidator) Validate(ctx context.Context, rawToken string) (Principal, error) {
	idToken, err := v.verifier.Verify(ctx, rawToken)
	if err != nil {
		return Principal{}, fmt.Errorf("%w: %v", ErrUnauthorized, err)
	}

	var claims oidcClaims
	if err := idToken.Claims(&claims); err != nil {
		return Principal{}, fmt.Errorf("%w: parse claims: %v", ErrUnauthorized, err)
	}

	if claims.Sub == "" {
		return Principal{}, fmt.Errorf("%w: subject claim is required", ErrUnauthorized)
	}

	if claims.Iss == "" {
		return Principal{}, fmt.Errorf("%w: issuer claim is required", ErrUnauthorized)
	}

	role := domain.Role(claims.Role)
	if role != domain.RoleEngineer && role != domain.RolePlatformAdmin {
		return Principal{}, fmt.Errorf("%w: invalid role claim", ErrUnauthorized)
	}

	return Principal{
		Subject: claims.Sub,
		Role:    role,
		Issuer:  claims.Iss,
	}, nil
}
