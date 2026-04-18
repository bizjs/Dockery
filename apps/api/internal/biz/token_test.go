package biz

import (
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

func newTestIssuer(t *testing.T) (*TokenIssuer, *Keystore) {
	t.Helper()
	ks := newTestKeystore(t)
	iss, err := NewTokenIssuer(ks, TokenIssuerConfig{
		Issuer:   "dockery-api",
		Audience: "dockery",
		TTL:      5 * time.Minute,
	})
	if err != nil {
		t.Fatalf("NewTokenIssuer: %v", err)
	}
	return iss, ks
}

func TestNewTokenIssuer_Validation(t *testing.T) {
	ks := newTestKeystore(t)

	_, err := NewTokenIssuer(nil, TokenIssuerConfig{Issuer: "x", Audience: "y", TTL: time.Hour})
	if err == nil {
		t.Error("nil keystore should error")
	}
	_, err = NewTokenIssuer(ks, TokenIssuerConfig{Issuer: "", Audience: "y", TTL: time.Hour})
	if err == nil {
		t.Error("empty issuer should error")
	}
	_, err = NewTokenIssuer(ks, TokenIssuerConfig{Issuer: "x", Audience: "y", TTL: 30 * time.Second})
	if err == nil {
		t.Error("TTL < 60s should error")
	}
}

func TestIssueRegistryToken_VerifyRoundTrip(t *testing.T) {
	iss, ks := newTestIssuer(t)

	access := []RegistryAccess{
		{Type: "repository", Name: "alice/app", Actions: []string{"pull", "push"}},
	}
	tokenStr, err := iss.IssueRegistryToken("alice", access)
	if err != nil {
		t.Fatalf("issue: %v", err)
	}
	if tokenStr == "" {
		t.Fatal("empty token")
	}

	// Verify with the public half — exactly what the Registry will do.
	parsed, err := jwt.ParseWithClaims(tokenStr, &registryClaims{}, func(tok *jwt.Token) (any, error) {
		if tok.Method.Alg() != jwt.SigningMethodEdDSA.Alg() {
			t.Fatalf("unexpected alg %s", tok.Method.Alg())
		}
		return ks.Public(), nil
	})
	if err != nil {
		t.Fatalf("parse+verify: %v", err)
	}
	if !parsed.Valid {
		t.Fatal("token not valid")
	}
	claims := parsed.Claims.(*registryClaims)

	if claims.Issuer != "dockery-api" {
		t.Errorf("iss = %q", claims.Issuer)
	}
	if claims.Subject != "alice" {
		t.Errorf("sub = %q", claims.Subject)
	}
	if len(claims.Audience) != 1 || claims.Audience[0] != "dockery" {
		t.Errorf("aud = %v", claims.Audience)
	}
	if len(claims.Access) != 1 {
		t.Fatalf("access = %v", claims.Access)
	}
	a := claims.Access[0]
	if a.Type != "repository" || a.Name != "alice/app" {
		t.Errorf("access target = %+v", a)
	}
	if len(a.Actions) != 2 || a.Actions[0] != "pull" || a.Actions[1] != "push" {
		t.Errorf("access actions = %v", a.Actions)
	}
}

func TestIssueRegistryToken_KIDInHeader(t *testing.T) {
	iss, ks := newTestIssuer(t)
	tokenStr, err := iss.IssueRegistryToken("alice", nil)
	if err != nil {
		t.Fatalf("issue: %v", err)
	}

	parser := jwt.NewParser()
	parsed, _, err := parser.ParseUnverified(tokenStr, &registryClaims{})
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	kid, _ := parsed.Header["kid"].(string)
	if kid != ks.KID() {
		t.Errorf("kid header = %q, want %q", kid, ks.KID())
	}
}

func TestIssueRegistryToken_EmptyAccessAllowed(t *testing.T) {
	iss, _ := newTestIssuer(t)
	// Docker spec: an authenticated user who was granted NO requested
	// actions still receives a valid token with access: [].
	_, err := iss.IssueRegistryToken("alice", nil)
	if err != nil {
		t.Fatalf("issue: %v", err)
	}
}

func TestIssueRegistryToken_Expiry(t *testing.T) {
	ks := newTestKeystore(t)
	iss, err := NewTokenIssuer(ks, TokenIssuerConfig{
		Issuer: "dockery-api", Audience: "dockery", TTL: time.Minute,
	})
	if err != nil {
		t.Fatalf("issuer: %v", err)
	}
	// Pin clock to determinism.
	fixed := time.Unix(1_700_000_000, 0).UTC()
	iss.now = func() time.Time { return fixed }

	tokenStr, err := iss.IssueRegistryToken("alice", nil)
	if err != nil {
		t.Fatalf("issue: %v", err)
	}
	parsed, err := jwt.ParseWithClaims(tokenStr, &registryClaims{}, func(*jwt.Token) (any, error) {
		return ks.Public(), nil
	}, jwt.WithTimeFunc(func() time.Time { return fixed.Add(30 * time.Second) }))
	if err != nil || !parsed.Valid {
		t.Fatalf("should still be valid at 30s in: %v", err)
	}

	_, err = jwt.ParseWithClaims(tokenStr, &registryClaims{}, func(*jwt.Token) (any, error) {
		return ks.Public(), nil
	}, jwt.WithTimeFunc(func() time.Time { return fixed.Add(2 * time.Minute) }))
	if err == nil {
		t.Fatal("expected expiry error at 2m in, got nil")
	}
}

func TestExpiresIn(t *testing.T) {
	iss, _ := newTestIssuer(t)
	if got := iss.ExpiresIn(); got != 300 {
		t.Errorf("ExpiresIn = %d, want 300", got)
	}
}
