package biz

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// WebhookSecret is the shared bearer token distribution registry uses
// when POSTing notification events into /api/internal/registry-events.
// Auto-generated (32 random bytes → 64 hex chars) on first boot and
// persisted to disk; the supervisord registry wrapper reads the same
// file and bakes the value into the registry's runtime config.yml, so
// both sides share a single source of truth.
type WebhookSecret struct {
	value string
}

// WebhookSecretConfig parameterises where the secret lives on disk.
type WebhookSecretConfig struct {
	// Path is the secret file location. Generated (mode 0600) if
	// missing. Required.
	Path string
}

// NewWebhookSecret loads the secret from disk, generating + writing it
// on first boot if absent.
func NewWebhookSecret(c WebhookSecretConfig) (*WebhookSecret, error) {
	if c.Path == "" {
		return nil, errors.New("webhook: Path is required")
	}
	v, err := loadOrGenerateSecret(c.Path)
	if err != nil {
		return nil, err
	}
	return &WebhookSecret{value: v}, nil
}

// Value returns the raw secret. Only service/webhook.go should read
// this to pass along in internal-only wiring (never to log / return).
func (s *WebhookSecret) Value() string { return s.value }

// Verify compares `token` (the bearer value, no "Bearer " prefix) to
// the persisted secret in constant time so timing attacks don't reveal
// partial matches.
func (s *WebhookSecret) Verify(token string) bool {
	a := []byte(strings.TrimSpace(token))
	b := []byte(s.value)
	if len(a) != len(b) {
		return false
	}
	return subtle.ConstantTimeCompare(a, b) == 1
}

// --- internals --------------------------------------------------------

func loadOrGenerateSecret(path string) (string, error) {
	if fileExists(path) {
		raw, err := os.ReadFile(path)
		if err != nil {
			return "", fmt.Errorf("webhook: read secret: %w", err)
		}
		s := strings.TrimSpace(string(raw))
		if s == "" {
			return "", fmt.Errorf("webhook: %s is empty", path)
		}
		return s, nil
	}
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("webhook: generate: %w", err)
	}
	s := hex.EncodeToString(buf)
	if err := writeSecret(path, s); err != nil {
		return "", err
	}
	return s, nil
}

func writeSecret(path, value string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("webhook: mkdir %s: %w", filepath.Dir(path), err)
	}
	tmp := path + ".tmp"
	// Trailing newline so `cat file` in shells renders cleanly and
	// `sed` substitution in the registry wrapper doesn't trip.
	if err := os.WriteFile(tmp, []byte(value+"\n"), 0o600); err != nil {
		return fmt.Errorf("webhook: write %s: %w", path, err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("webhook: rename %s: %w", path, err)
	}
	return nil
}
