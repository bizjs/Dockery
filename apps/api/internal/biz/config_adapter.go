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
