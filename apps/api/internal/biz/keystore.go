package biz

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// Keystore owns the Ed25519 signing keypair used to issue Dockery
// registry tokens. The registry validates those tokens by reading the
// derived JWKS file (auth.token.jwks).
//
// The private key is the single source of truth: the public half is
// derived on load via priv.Public(), and a JWKS view is (re)written
// to disk on every boot so registry rotation requires nothing more
// than restarting dockery-api (which regenerates the JWKS from the
// same private key, or a new one if absent).
type Keystore struct {
	priv ed25519.PrivateKey
	pub  ed25519.PublicKey
	kid  string
}

// KeystoreConfig is the subset of Dockery config that Keystore needs.
type KeystoreConfig struct {
	// PrivatePath is where the Ed25519 private key lives. Generated
	// (mode 0600) if missing. The public half is derived from this
	// file; there is no separate on-disk public key.
	PrivatePath string
	// JWKSPath is the path Keystore writes the RFC 7517 JSON Web Key
	// Set to. distribution registry reads this via auth.token.jwks.
	// Rewritten on every boot so rotation propagates without a
	// separate step. Required.
	JWKSPath string
}

// NewKeystore loads the Ed25519 private key from disk (generating one
// on first run), derives the public key, and writes a fresh JWKS file
// for registry consumption. The JWKS overwrite is idempotent.
func NewKeystore(c KeystoreConfig) (*Keystore, error) {
	if c.PrivatePath == "" {
		return nil, errors.New("keystore: PrivatePath is required")
	}
	if c.JWKSPath == "" {
		return nil, errors.New("keystore: JWKSPath is required (registry consumes it via auth.token.jwks)")
	}

	priv, err := loadOrGeneratePrivate(c.PrivatePath)
	if err != nil {
		return nil, err
	}
	pub := priv.Public().(ed25519.PublicKey)

	kid := JWKThumbprint(pub)
	if err := writeJWKS(c.JWKSPath, pub, kid); err != nil {
		return nil, err
	}

	return &Keystore{
		priv: priv,
		pub:  pub,
		kid:  kid,
	}, nil
}

// Private returns the Ed25519 private key for JWT signing.
func (k *Keystore) Private() ed25519.PrivateKey { return k.priv }

// Public returns the Ed25519 public key.
func (k *Keystore) Public() ed25519.PublicKey { return k.pub }

// KID returns the RFC 8037 JWK thumbprint used as the JWT "kid" header;
// the registry uses this to look up the right verification key.
func (k *Keystore) KID() string { return k.kid }

// --- internals --------------------------------------------------------

func loadOrGeneratePrivate(path string) (ed25519.PrivateKey, error) {
	if fileExists(path) {
		priv, err := readPrivatePEM(path)
		if err != nil {
			return nil, fmt.Errorf("keystore: read private key: %w", err)
		}
		return priv, nil
	}
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("keystore: generate: %w", err)
	}
	if err := writePrivatePEM(path, priv); err != nil {
		return nil, err
	}
	return priv, nil
}

func writePrivatePEM(path string, priv ed25519.PrivateKey) error {
	der, err := x509.MarshalPKCS8PrivateKey(priv)
	if err != nil {
		return fmt.Errorf("keystore: marshal private: %w", err)
	}
	return writePEM(path, "PRIVATE KEY", der, 0o600)
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

// jwk is the RFC 7517 JSON Web Key representation of an Ed25519 public
// key per RFC 8037. Only fields distribution registry actually reads
// are emitted.
type jwk struct {
	Kty string `json:"kty"`
	Crv string `json:"crv"`
	X   string `json:"x"`
	Kid string `json:"kid"`
	Use string `json:"use,omitempty"`
	Alg string `json:"alg,omitempty"`
}

type jwks struct {
	Keys []jwk `json:"keys"`
}

// writeJWKS emits a single-key JWK Set file for the registry.
// Written via tmp+rename so a concurrent registry reload cannot see a
// truncated file. Overwritten on every Keystore boot so rotations
// propagate to registry on its next restart.
func writeJWKS(path string, pub ed25519.PublicKey, kid string) error {
	doc := jwks{Keys: []jwk{{
		Kty: "OKP",
		Crv: "Ed25519",
		X:   base64.RawURLEncoding.EncodeToString(pub),
		Kid: kid,
		Use: "sig",
		Alg: "EdDSA",
	}}}
	data, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return fmt.Errorf("keystore: marshal jwks: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("keystore: mkdir %s: %w", filepath.Dir(path), err)
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return fmt.Errorf("keystore: write jwks %s: %w", path, err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("keystore: rename jwks %s: %w", path, err)
	}
	return nil
}
