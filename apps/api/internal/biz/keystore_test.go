package biz

import (
	"crypto/ed25519"
	"encoding/base64"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
)

func newTestKeystore(t *testing.T) *Keystore {
	t.Helper()
	dir := t.TempDir()
	ks, err := NewKeystore(KeystoreConfig{
		PrivatePath: filepath.Join(dir, "priv.pem"),
		PublicPath:  filepath.Join(dir, "pub.pem"),
	})
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

func TestKeystore_FilePermissions(t *testing.T) {
	dir := t.TempDir()
	priv := filepath.Join(dir, "priv.pem")
	pub := filepath.Join(dir, "pub.pem")
	_, err := NewKeystore(KeystoreConfig{PrivatePath: priv, PublicPath: pub})
	if err != nil {
		t.Fatalf("NewKeystore: %v", err)
	}

	st, err := os.Stat(priv)
	if err != nil {
		t.Fatalf("stat priv: %v", err)
	}
	if m := st.Mode().Perm(); m != fs.FileMode(0o600) {
		t.Errorf("priv perm = %o, want 0600", m)
	}
	st, err = os.Stat(pub)
	if err != nil {
		t.Fatalf("stat pub: %v", err)
	}
	if m := st.Mode().Perm(); m != fs.FileMode(0o644) {
		t.Errorf("pub perm = %o, want 0644", m)
	}
}

func TestKeystore_Reloads(t *testing.T) {
	dir := t.TempDir()
	cfg := KeystoreConfig{
		PrivatePath: filepath.Join(dir, "priv.pem"),
		PublicPath:  filepath.Join(dir, "pub.pem"),
	}
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
	if !bytesEq(first.Public(), second.Public()) {
		t.Fatalf("public key changed on reload")
	}
}

func TestKeystore_DetectsKeyMismatch(t *testing.T) {
	dir := t.TempDir()
	priv := filepath.Join(dir, "priv.pem")
	pub := filepath.Join(dir, "pub.pem")

	// Seed a valid pair.
	if _, err := NewKeystore(KeystoreConfig{PrivatePath: priv, PublicPath: pub}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	// Overwrite the public key with a different one.
	pub2, _, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("gen: %v", err)
	}
	if err := writePublicPEM(pub, pub2); err != nil {
		t.Fatalf("overwrite pub: %v", err)
	}

	if _, err := NewKeystore(KeystoreConfig{PrivatePath: priv, PublicPath: pub}); err == nil {
		t.Fatal("expected mismatch error, got nil")
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
