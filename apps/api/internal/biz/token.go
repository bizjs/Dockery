package biz

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// TokenIssuer signs Docker-registry-spec JWTs with Ed25519.
//
// The registry process verifies these tokens against the public key
// pinned at auth.token.rootcertbundle. Keys are stable across process
// restarts (persisted to disk) but rotate via Keystore replacement.
type TokenIssuer struct {
	keystore *Keystore
	issuer   string        // JWT "iss" — must match registry conf.auth.token.issuer
	audience string        // JWT "aud" — must match registry conf.auth.token.service
	ttl      time.Duration // token lifetime
	now      func() time.Time
}

// TokenIssuerConfig parameterises issuer/audience/ttl. M2.3 will wire
// these from conf; M2.2 tests pass them inline.
type TokenIssuerConfig struct {
	Issuer   string
	Audience string
	TTL      time.Duration
}

// NewTokenIssuer validates config and returns a ready-to-sign issuer.
// TTL must be ≥ 60s — Docker Registry rejects tokens whose exp-iat
// window is smaller.
func NewTokenIssuer(ks *Keystore, c TokenIssuerConfig) (*TokenIssuer, error) {
	if ks == nil {
		return nil, errors.New("token: keystore is required")
	}
	if c.Issuer == "" {
		return nil, errors.New("token: Issuer is required")
	}
	if c.Audience == "" {
		return nil, errors.New("token: Audience is required")
	}
	if c.TTL < time.Minute {
		return nil, fmt.Errorf("token: TTL %s too short (minimum 60s)", c.TTL)
	}
	return &TokenIssuer{
		keystore: ks,
		issuer:   c.Issuer,
		audience: c.Audience,
		ttl:      c.TTL,
		now:      time.Now,
	}, nil
}

// RegistryAccess is one entry of the JWT "access" claim.
// Name is the repository (or "catalog"), Type is "repository"/"registry",
// Actions are the granted operations per the Docker token-auth spec.
type RegistryAccess struct {
	Type    string   `json:"type"`
	Name    string   `json:"name"`
	Actions []string `json:"actions"`
}

// registryClaims is the full JWT payload. The extra "access" field is
// the Docker-registry-specific extension over RFC 7519.
type registryClaims struct {
	jwt.RegisteredClaims
	Access []RegistryAccess `json:"access"`
}

// IssueRegistryToken signs a JWT for docker CLI consumption.
//
// subject — the authenticated username (JWT "sub").
// access  — post-intersection granted access entries; may be empty (the
//
//	Docker spec explicitly allows empty access claims when the
//	user authenticated successfully but was not granted any of
//	the requested scopes; the registry then produces 401 for
//	the follow-up call).
func (i *TokenIssuer) IssueRegistryToken(subject string, access []RegistryAccess) (string, error) {
	now := i.now()

	jti, err := newJTI()
	if err != nil {
		return "", err
	}

	claims := registryClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    i.issuer,
			Subject:   subject,
			Audience:  jwt.ClaimStrings{i.audience},
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now.Add(-5 * time.Second)), // clock skew slack
			ExpiresAt: jwt.NewNumericDate(now.Add(i.ttl)),
			ID:        jti,
		},
		Access: access,
	}

	tok := jwt.NewWithClaims(jwt.SigningMethodEdDSA, claims)
	tok.Header["kid"] = i.keystore.KID()

	signed, err := tok.SignedString(i.keystore.Private())
	if err != nil {
		return "", fmt.Errorf("token: sign: %w", err)
	}
	return signed, nil
}

// ExpiresIn reports the issuer's configured token lifetime in seconds,
// matching the shape Docker expects in /token responses ("expires_in").
func (i *TokenIssuer) ExpiresIn() int {
	return int(i.ttl / time.Second)
}

// IssuedAt returns the current wall-clock time as the /token response
// expects ("issued_at"). Callers typically format it in RFC 3339.
func (i *TokenIssuer) IssuedAt() time.Time { return i.now() }

// newJTI returns a 128-bit random identifier base64url-encoded.
// Used as JWT "jti" — the registry may (but doesn't have to) de-dup on it.
func newJTI() (string, error) {
	var buf [16]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return "", fmt.Errorf("token: jti: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(buf[:]), nil
}
