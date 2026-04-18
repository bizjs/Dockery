package biz

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// Keystore owns the Ed25519 signing keypair used to issue Dockery
// registry tokens. The registry validates those tokens against the
// public key pinned in its config (auth.token.rootcertbundle).
//
// The pair is persisted at the two paths passed to NewKeystore; if
// either file is absent on startup a fresh pair is generated atomically.
// Rotation (M4) writes new files and signals the registry process to
// reload.
type Keystore struct {
	priv    ed25519.PrivateKey
	pub     ed25519.PublicKey
	kid     string
	pubPath string
}

// KeystoreConfig is the subset of Dockery config that Keystore needs.
type KeystoreConfig struct {
	PrivatePath string
	PublicPath  string
}

// NewKeystore loads an existing Ed25519 keypair from the configured
// paths, or generates one on first run. Private key is written with
// mode 0600, public key with 0644. The directory is created if missing.
func NewKeystore(c KeystoreConfig) (*Keystore, error) {
	if c.PrivatePath == "" || c.PublicPath == "" {
		return nil, errors.New("keystore: PrivatePath and PublicPath are required")
	}

	priv, pub, err := loadOrGenerate(c.PrivatePath, c.PublicPath)
	if err != nil {
		return nil, err
	}

	return &Keystore{
		priv:    priv,
		pub:     pub,
		kid:     JWKThumbprint(pub),
		pubPath: c.PublicPath,
	}, nil
}

// Private returns the Ed25519 private key for JWT signing.
func (k *Keystore) Private() ed25519.PrivateKey { return k.priv }

// Public returns the Ed25519 public key.
func (k *Keystore) Public() ed25519.PublicKey { return k.pub }

// KID returns the RFC 8037 JWK thumbprint used as the JWT "kid" header;
// the registry uses this to look up the right verification key.
func (k *Keystore) KID() string { return k.kid }

// PublicPath is the filesystem location of the PEM-encoded public key.
// The registry process reads this same file.
func (k *Keystore) PublicPath() string { return k.pubPath }

// --- internals --------------------------------------------------------

func loadOrGenerate(privPath, pubPath string) (ed25519.PrivateKey, ed25519.PublicKey, error) {
	// Fast path: both files exist → load.
	if fileExists(privPath) && fileExists(pubPath) {
		priv, err := readPrivatePEM(privPath)
		if err != nil {
			return nil, nil, fmt.Errorf("keystore: read private key: %w", err)
		}
		pub, err := readPublicPEM(pubPath)
		if err != nil {
			return nil, nil, fmt.Errorf("keystore: read public key: %w", err)
		}
		// Cross-check: derive public from private and compare.
		derived := priv.Public().(ed25519.PublicKey)
		if !bytesEq(derived, pub) {
			return nil, nil, errors.New("keystore: private/public key mismatch on disk")
		}
		return priv, pub, nil
	}

	// Generate fresh pair.
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, nil, fmt.Errorf("keystore: generate: %w", err)
	}
	if err := writePrivatePEM(privPath, priv); err != nil {
		return nil, nil, err
	}
	if err := writePublicPEM(pubPath, pub); err != nil {
		return nil, nil, err
	}
	return priv, pub, nil
}

func writePrivatePEM(path string, priv ed25519.PrivateKey) error {
	der, err := x509.MarshalPKCS8PrivateKey(priv)
	if err != nil {
		return fmt.Errorf("keystore: marshal private: %w", err)
	}
	return writePEM(path, "PRIVATE KEY", der, 0o600)
}

func writePublicPEM(path string, pub ed25519.PublicKey) error {
	der, err := x509.MarshalPKIXPublicKey(pub)
	if err != nil {
		return fmt.Errorf("keystore: marshal public: %w", err)
	}
	return writePEM(path, "PUBLIC KEY", der, 0o644)
}

func writePEM(path, blockType string, der []byte, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("keystore: mkdir %s: %w", filepath.Dir(path), err)
	}
	pemBytes := pem.EncodeToMemory(&pem.Block{Type: blockType, Bytes: der})
	// Write via temp file + rename to avoid torn writes.
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, pemBytes, mode); err != nil {
		return fmt.Errorf("keystore: write %s: %w", path, err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("keystore: rename %s: %w", path, err)
	}
	return nil
}

func readPrivatePEM(path string) (ed25519.PrivateKey, error) {
	der, err := decodePEM(path, "PRIVATE KEY")
	if err != nil {
		return nil, err
	}
	key, err := x509.ParsePKCS8PrivateKey(der)
	if err != nil {
		return nil, fmt.Errorf("parse pkcs8: %w", err)
	}
	priv, ok := key.(ed25519.PrivateKey)
	if !ok {
		return nil, fmt.Errorf("unexpected key type %T", key)
	}
	return priv, nil
}

func readPublicPEM(path string) (ed25519.PublicKey, error) {
	der, err := decodePEM(path, "PUBLIC KEY")
	if err != nil {
		return nil, err
	}
	key, err := x509.ParsePKIXPublicKey(der)
	if err != nil {
		return nil, fmt.Errorf("parse pkix: %w", err)
	}
	pub, ok := key.(ed25519.PublicKey)
	if !ok {
		return nil, fmt.Errorf("unexpected key type %T", key)
	}
	return pub, nil
}

func decodePEM(path, wantType string) ([]byte, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	block, _ := pem.Decode(raw)
	if block == nil {
		return nil, fmt.Errorf("no PEM block in %s", path)
	}
	if block.Type != wantType {
		return nil, fmt.Errorf("unexpected PEM type %q (want %q) in %s", block.Type, wantType, path)
	}
	return block.Bytes, nil
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func bytesEq(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// JWKThumbprint computes the RFC 7638 / RFC 8037 JSON Web Key thumbprint
// for an Ed25519 public key. The thumbprint is a stable, key-specific
// identifier suitable for the JWT "kid" header; Registry uses this to
// select the right verification key when multiple are configured.
//
// For OKP / Ed25519, the canonical JWK is:
//
//	{"crv":"Ed25519","kty":"OKP","x":"<base64url-raw-key>"}
//
// The thumbprint is base64url(SHA-256(canonical-json)).
func JWKThumbprint(pub ed25519.PublicKey) string {
	x := base64.RawURLEncoding.EncodeToString(pub)
	canonical := `{"crv":"Ed25519","kty":"OKP","x":"` + x + `"}`
	sum := sha256.Sum256([]byte(canonical))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}
