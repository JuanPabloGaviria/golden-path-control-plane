package devoidc

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/juanpablogaviria/golden-path-control-plane/internal/domain"
)

const (
	DefaultAddr                 = ":9000"
	DefaultIssuerURL            = "http://localhost:9000"
	DefaultAudience             = "golden-path-control-plane"
	DefaultEngineerSubject      = "developer@example.com"
	DefaultPlatformAdminSubject = "platform-admin@example.com"
	EngineerClientID            = "golden-path-engineer"
	PlatformAdminClientID       = "golden-path-platform-admin"
)

type Config struct {
	Addr                 string
	IssuerURL            string
	Audience             string
	TokenTTL             time.Duration
	EngineerSubject      string
	PlatformAdminSubject string
}

type TokenClaims struct {
	Subject string
	Role    string
	TTL     time.Duration
}

type Issuer struct {
	cfg        Config
	privateKey *rsa.PrivateKey
	keyID      string
	clients    map[string]clientDefinition
	now        func() time.Time
}

type clientDefinition struct {
	ClientID       string
	DefaultSubject string
	Role           domain.Role
}

type tokenResponse struct {
	AccessToken     string `json:"access_token"`
	IDToken         string `json:"id_token"`
	TokenType       string `json:"token_type"`
	ExpiresIn       int    `json:"expires_in"`
	IssuedTokenType string `json:"issued_token_type"`
}

type discoveryDocument struct {
	Issuer                            string   `json:"issuer"`
	JWKSURI                           string   `json:"jwks_uri"`
	TokenEndpoint                     string   `json:"token_endpoint"`
	GrantTypesSupported               []string `json:"grant_types_supported"`
	ResponseTypesSupported            []string `json:"response_types_supported"`
	SubjectTypesSupported             []string `json:"subject_types_supported"`
	IDTokenSigningAlgValuesSupported  []string `json:"id_token_signing_alg_values_supported"`
	TokenEndpointAuthMethodsSupported []string `json:"token_endpoint_auth_methods_supported"`
}

func New(cfg Config) (*Issuer, error) {
	if strings.TrimSpace(cfg.Addr) == "" {
		cfg.Addr = DefaultAddr
	}
	if strings.TrimSpace(cfg.IssuerURL) == "" {
		cfg.IssuerURL = DefaultIssuerURL
	}
	if strings.TrimSpace(cfg.Audience) == "" {
		cfg.Audience = DefaultAudience
	}
	if cfg.TokenTTL <= 0 {
		cfg.TokenTTL = time.Hour
	}
	if strings.TrimSpace(cfg.EngineerSubject) == "" {
		cfg.EngineerSubject = DefaultEngineerSubject
	}
	if strings.TrimSpace(cfg.PlatformAdminSubject) == "" {
		cfg.PlatformAdminSubject = DefaultPlatformAdminSubject
	}

	if _, err := url.Parse(cfg.IssuerURL); err != nil {
		return nil, fmt.Errorf("devoidc: parse issuer url: %w", err)
	}

	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, fmt.Errorf("devoidc: generate rsa key: %w", err)
	}

	keyMaterial := x509.MarshalPKCS1PublicKey(&privateKey.PublicKey)
	keyHash := sha256.Sum256(keyMaterial)

	return &Issuer{
		cfg:        cfg,
		privateKey: privateKey,
		keyID:      hex.EncodeToString(keyHash[:8]),
		now:        func() time.Time { return time.Now().UTC() },
		clients: map[string]clientDefinition{
			EngineerClientID: {
				ClientID:       EngineerClientID,
				DefaultSubject: cfg.EngineerSubject,
				Role:           domain.RoleEngineer,
			},
			PlatformAdminClientID: {
				ClientID:       PlatformAdminClientID,
				DefaultSubject: cfg.PlatformAdminSubject,
				Role:           domain.RolePlatformAdmin,
			},
		},
	}, nil
}

func (i *Issuer) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/.well-known/openid-configuration", i.discovery)
	mux.HandleFunc("/keys", i.keys)
	mux.HandleFunc("/token", i.token)
	mux.HandleFunc("/healthz", i.healthz)
	return mux
}

func (i *Issuer) SignToken(claims TokenClaims) (string, error) {
	subject := strings.TrimSpace(claims.Subject)
	if subject == "" {
		return "", errors.New("devoidc: subject is required")
	}

	role := strings.TrimSpace(claims.Role)
	if role == "" {
		return "", errors.New("devoidc: role is required")
	}

	ttl := claims.TTL
	if ttl == 0 {
		ttl = i.cfg.TokenTTL
	}

	now := i.now()
	return i.SignMapClaims(jwt.MapClaims{
		"iss":  i.cfg.IssuerURL,
		"sub":  subject,
		"aud":  []string{i.cfg.Audience},
		"exp":  now.Add(ttl).Unix(),
		"iat":  now.Unix(),
		"nbf":  now.Add(-1 * time.Minute).Unix(),
		"role": role,
	})
}

func (i *Issuer) SignMapClaims(claims jwt.MapClaims) (string, error) {
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	token.Header["kid"] = i.keyID

	signed, err := token.SignedString(i.privateKey)
	if err != nil {
		return "", fmt.Errorf("devoidc: sign token: %w", err)
	}

	return signed, nil
}

func (i *Issuer) discovery(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	writeJSON(w, http.StatusOK, discoveryDocument{
		Issuer:                            i.cfg.IssuerURL,
		JWKSURI:                           strings.TrimSuffix(i.cfg.IssuerURL, "/") + "/keys",
		TokenEndpoint:                     strings.TrimSuffix(i.cfg.IssuerURL, "/") + "/token",
		GrantTypesSupported:               []string{"client_credentials"},
		ResponseTypesSupported:            []string{"token"},
		SubjectTypesSupported:             []string{"public"},
		IDTokenSigningAlgValuesSupported:  []string{"RS256"},
		TokenEndpointAuthMethodsSupported: []string{"none"},
	})
}

func (i *Issuer) keys(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"keys": []map[string]any{{
			"kty": "RSA",
			"alg": "RS256",
			"use": "sig",
			"kid": i.keyID,
			"n":   encodeBigInt(i.privateKey.N),
			"e":   encodeBigInt(big.NewInt(int64(i.privateKey.E))),
		}},
	})
}

func (i *Issuer) token(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form body", http.StatusBadRequest)
		return
	}

	if grantType := r.Form.Get("grant_type"); grantType != "client_credentials" {
		http.Error(w, "unsupported grant_type", http.StatusBadRequest)
		return
	}

	clientID := strings.TrimSpace(r.Form.Get("client_id"))
	client, ok := i.clients[clientID]
	if !ok {
		http.Error(w, "invalid client_id", http.StatusUnauthorized)
		return
	}

	subject := strings.TrimSpace(r.Form.Get("subject"))
	if subject == "" {
		subject = client.DefaultSubject
	}

	signedToken, err := i.SignToken(TokenClaims{
		Subject: subject,
		Role:    string(client.Role),
		TTL:     i.cfg.TokenTTL,
	})
	if err != nil {
		http.Error(w, "failed to issue token", http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, tokenResponse{
		AccessToken:     signedToken,
		IDToken:         signedToken,
		TokenType:       "Bearer",
		ExpiresIn:       int(i.cfg.TokenTTL.Seconds()),
		IssuedTokenType: "urn:ietf:params:oauth:token-type:id_token",
	})
}

func (i *Issuer) healthz(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func encodeBigInt(value *big.Int) string {
	return base64.RawURLEncoding.EncodeToString(value.Bytes())
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}
