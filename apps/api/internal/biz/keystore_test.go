package biz

import (
	"crypto/ed25519"
	"encoding/base64"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
)

func newTestKeystoreConfig(t *testing.T) KeystoreConfig {
	t.Helper()
	dir := t.TempDir()
	return KeystoreConfig{
		PrivatePath: filepath.Join(dir, "priv.pem"),
		JWKSPath:    filepath.Join(dir, "jwks.json"),
	}
}

func newTestKeystore(t *testing.T) *Keystore {
	t.Helper()
	ks, err := NewKeystore(newTestKeystoreConfig(t))
	if err != nil {
		t.Fatalf("NewKeystore: %v", err)
	}
	return ks
}

func TestKeystore_GeneratesOnFirstRun(t *testing.T) {
	ks := newTestKeystore(t)
	if ks.Private() == nil || ks.Public() == nil {
		t.Fatal("keys not populated")
	}
	if ks.KID() == "" {
		t.Fatal("KID empty")
	}
	// Signature round-trip
	msg := []byte("hello dockery")
	sig := ed25519.Sign(ks.Private(), msg)
	if !ed25519.Verify(ks.Public(), msg, sig) {
		t.Fatal("signature did not verify")
	}
}

func TestKeystore_PrivateFilePermissions(t *testing.T) {
	cfg := newTestKeystoreConfig(t)
	if _, err := NewKeystore(cfg); err != nil {
		t.Fatalf("NewKeystore: %v", err)
	}
	st, err := os.Stat(cfg.PrivatePath)
	if err != nil {
		t.Fatalf("stat priv: %v", err)
	}
	if m := st.Mode().Perm(); m != fs.FileMode(0o600) {
		t.Errorf("priv perm = %o, want 0600", m)
	}
}

func TestKeystore_Reloads(t *testing.T) {
	cfg := newTestKeystoreConfig(t)
	first, err := NewKeystore(cfg)
	if err != nil {
		t.Fatalf("first NewKeystore: %v", err)
	}
	second, err := NewKeystore(cfg)
	if err != nil {
		t.Fatalf("second NewKeystore: %v", err)
	}
	if first.KID() != second.KID() {
		t.Fatalf("KID changed on reload: %s != %s", first.KID(), second.KID())
	}
	// Public derives from the same private → must be byte-identical.
	a, b := first.Public(), second.Public()
	if len(a) != len(b) {
		t.Fatalf("public length drift: %d vs %d", len(a), len(b))
	}
	for i := range a {
		if a[i] != b[i] {
			t.Fatalf("public key byte drift at %d", i)
		}
	}
}

func TestJWKThumbprint_Stable(t *testing.T) {
	// Two Keystores built from the same on-disk pair must have the same KID.
	ks := newTestKeystore(t)
	got := JWKThumbprint(ks.Public())
	if got != ks.KID() {
		t.Errorf("KID drift: %s vs %s", got, ks.KID())
	}
	// KID must be valid base64url SHA-256 (43 chars without padding).
	if _, err := base64.RawURLEncoding.DecodeString(got); err != nil {
		t.Errorf("KID not base64url: %v", err)
	}
	if len(got) != 43 {
		t.Errorf("KID length = %d, want 43", len(got))
	}
}
