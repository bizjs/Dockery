package biz

import (
	"time"

	"api/internal/conf"

	"github.com/bizjs/kratoscarf/auth/session"
)

// NewKeystoreConfigFromConf maps the protobuf-adjacent Dockery config
// into biz.KeystoreConfig so the keystore constructor stays
// config-shape-agnostic (easier to unit-test without pulling in conf).
func NewKeystoreConfigFromConf(c *conf.Dockery) KeystoreConfig {
	return KeystoreConfig{
		PrivatePath: c.Keystore.PrivatePath,
		JWKSPath:    c.Keystore.JWKSPath,
	}
}

// NewWebhookSecretConfigFromConf maps the yaml webhook section. Falls
// back to the container-baked default path so operators can leave the
// section out entirely.
func NewWebhookSecretConfigFromConf(c *conf.Dockery) WebhookSecretConfig {
	path := c.Webhook.SecretPath
	if path == "" {
		path = "/data/config/webhook-secret"
	}
	return WebhookSecretConfig{Path: path}
}

// RegistryUpstreamURL is the base URL other biz components (reconciler,
// meta refresher) use for /v2/ calls against the local distribution
// registry. Defaults to 127.0.0.1:5001 (the in-container nginx-less
// port); override via `dockery.registry.upstream_url`.
type RegistryUpstreamURL string

// NewRegistryUpstreamURL resolves the upstream base URL from conf with
// a sane container default.
func NewRegistryUpstreamURL(c *conf.Dockery) RegistryUpstreamURL {
	u := c.Registry.UpstreamURL
	if u == "" {
		u = "http://127.0.0.1:5001"
	}
	return RegistryUpstreamURL(u)
}

// NewTokenIssuerConfigFromConf derives a TokenIssuerConfig from the
// yaml-loaded Dockery section. TTL is stored as seconds in config so
// yaml stays human-readable; we convert to time.Duration here.
func NewTokenIssuerConfigFromConf(c *conf.Dockery) TokenIssuerConfig {
	ttl := time.Duration(c.Token.TTLSeconds) * time.Second
	if ttl <= 0 {
		ttl = 5 * time.Minute
	}
	return TokenIssuerConfig{
		Issuer:   c.Token.Issuer,
		Audience: c.Token.Audience,
		TTL:      ttl,
	}
}

// NewGCConfigFromConf converts the yaml DockeryGC section into biz.GCConfig.
// Empty fields fall back to the container-baked defaults so operators
// can override only what they need (or leave the section out entirely).
func NewGCConfigFromConf(c *conf.Dockery) GCConfig {
	cfg := defaultGCConfig()
	g := c.GC
	if g.SupervisorctlBin != "" {
		cfg.SupervisorctlBin = g.SupervisorctlBin
	}
	if g.SupervisordConf != "" {
		cfg.SupervisordConf = g.SupervisordConf
	}
	if g.RegistryBin != "" {
		cfg.RegistryBin = g.RegistryBin
	}
	if g.RegistryConf != "" {
		cfg.RegistryConf = g.RegistryConf
	}
	if g.ServiceName != "" {
		cfg.ServiceName = g.ServiceName
	}
	if g.DeleteUntagged != nil {
		cfg.DeleteUntagged = *g.DeleteUntagged
	}
	if g.RegistryRootDir != "" {
		cfg.RegistryRootDir = g.RegistryRootDir
	}
	if g.PruneEmptyRepos != nil {
		cfg.PruneEmptyRepos = *g.PruneEmptyRepos
	}
	if g.TimeoutSeconds > 0 {
		cfg.Timeout = time.Duration(g.TimeoutSeconds) * time.Second
	}
	return cfg
}

// NewSessionManager is a variadic-free wrapper around
// session.NewManager so wire can provide a *Manager without needing to
// materialize a (usually empty) []session.Option. We never pass
// functional options today; if that changes, extend this wrapper.
func NewSessionManager(store session.Store, cfg session.Config) *session.Manager {
	return session.NewManager(store, cfg)
}

// NewSessionConfigFromConf maps the yaml Session section into a
// kratoscarf session.Config. The kratoscarf struct uses camelCase yaml
// tags internally, but Dockery exposes snake_case through its own conf
// struct for style consistency — this adapter is the bridge.
func NewSessionConfigFromConf(c *conf.Dockery) session.Config {
	maxAge := time.Duration(c.Session.TTLHours) * time.Hour
	if maxAge <= 0 {
		maxAge = 7 * 24 * time.Hour
	}
	name := c.Session.CookieName
	if name == "" {
		name = "dockery_session"
	}
	return session.Config{
		MaxAge:     maxAge,
		CookieName: name,
		CookiePath: "/",
		Secure:     c.Session.CookieSecure,
		HTTPOnly:   true,
		SameSite:   "lax",
	}
}
