package biz

import (
	"time"

	"api/internal/conf"
)

// NewKeystoreConfigFromConf maps the protobuf-adjacent Dockery config
// into biz.KeystoreConfig so the keystore constructor stays
// config-shape-agnostic (easier to unit-test without pulling in conf).
func NewKeystoreConfigFromConf(c *conf.Dockery) KeystoreConfig {
	return KeystoreConfig{
		PrivatePath: c.Keystore.PrivatePath,
		PublicPath:  c.Keystore.PublicPath,
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
