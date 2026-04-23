package conf

// Dockery is the Dockery-specific configuration section, layered
// alongside the protobuf-generated Bootstrap.
//
// It is loaded in cmd/api/main.go via:
//
//	c.Value("dockery").Scan(&dockeryConf)
//
// Keeping this out of conf.proto lets us iterate on application-level
// knobs without touching the Kratos-managed protobuf schema.
type Dockery struct {
	Keystore DockeryKeystore `json:"keystore" yaml:"keystore"`
	Token    DockeryToken    `json:"token"    yaml:"token"`
	Admin    DockeryAdmin    `json:"admin"    yaml:"admin"`
	Session  DockerySession  `json:"session"  yaml:"session"`
	GC       DockeryGC       `json:"gc"       yaml:"gc"`
	Webhook  DockeryWebhook  `json:"webhook"  yaml:"webhook"`
	Registry DockeryRegistry `json:"registry" yaml:"registry"`
}

// DockeryKeystore is the filesystem location of the Ed25519 signing
// key. The private key is the single source of truth; the public key
// is derived at runtime and exposed to distribution registry as a
// JWKS file (RFC 7517 / RFC 8037), not a raw PEM.
//
// JWKS is used instead of rootcertbundle because distribution v3
// accepts only X.509 CERTIFICATE PEM blocks there, silently ignoring
// bare PUBLIC KEY blocks.
type DockeryKeystore struct {
	PrivatePath string `json:"private_path" yaml:"private_path"`
	// JWKSPath is where Keystore writes the JSON Web Key Set that the
	// distribution registry reads as `auth.token.jwks`.
	JWKSPath string `json:"jwks_path" yaml:"jwks_path"`
}

// DockeryToken pins the JWT issuance parameters. Issuer + Audience MUST
// match the Distribution Registry's auth.token.issuer / auth.token.service.
type DockeryToken struct {
	Issuer     string `json:"issuer"      yaml:"issuer"`
	Audience   string `json:"audience"    yaml:"audience"`
	TTLSeconds int    `json:"ttl_seconds" yaml:"ttl_seconds"`
	// PublicURL is the externally-reachable URL the registry advertises
	// in its WWW-Authenticate challenge (<PublicURL>/token). Leave empty
	// to derive from the request host; explicit override is needed when
	// behind a reverse proxy with a different hostname.
	PublicURL string `json:"public_url" yaml:"public_url"`
}

// DockeryAdmin bootstraps the first admin account on an empty database.
// Password MUST be provided via env DOCKERY_ADMIN_PASSWORD (preferred)
// or the yaml field; the latter is discouraged for anything but dev.
type DockeryAdmin struct {
	Username string `json:"username" yaml:"username"`
	Password string `json:"password" yaml:"password"`
}

// DockeryGC configures how dockery-api drives garbage collection of
// the upstream distribution registry. All paths default to the baked
// container layout (supervisord + alpine); override when running
// dockery-api outside the provided image.
type DockeryGC struct {
	SupervisorctlBin string `json:"supervisorctl_bin" yaml:"supervisorctl_bin"` // /usr/bin/supervisorctl
	SupervisordConf  string `json:"supervisord_conf"  yaml:"supervisord_conf"`  // /etc/supervisord.conf
	RegistryBin      string `json:"registry_bin"      yaml:"registry_bin"`      // /usr/local/bin/registry
	RegistryConf     string `json:"registry_conf"     yaml:"registry_conf"`     // /etc/docker/registry/config.yml
	ServiceName      string `json:"service_name"      yaml:"service_name"`      // supervisord [program:<name>]
	// DeleteUntagged passes --delete-untagged to `registry garbage-collect`
	// so manifests with no remaining tag are removed too. Default: true.
	// Only disable if you actively rely on digest-only references after
	// tag deletion (unusual).
	DeleteUntagged *bool `json:"delete_untagged" yaml:"delete_untagged"`
	// RegistryRootDir is the filesystem root that matches the
	// distribution registry's `storage.filesystem.rootdirectory`
	// (default /data/registry). Dockery walks this to prune repo
	// directories that have no tags after GC.
	RegistryRootDir string `json:"registry_root_dir" yaml:"registry_root_dir"`
	// PruneEmptyRepos controls the post-GC sweep of empty repo dirs.
	// Default: true. Set false when using a non-filesystem storage
	// driver (S3 etc.) where the on-disk layout doesn't apply.
	PruneEmptyRepos *bool `json:"prune_empty_repos" yaml:"prune_empty_repos"`
	// TimeoutSeconds is the hard cap on the full stop/gc/restart cycle.
	// Default: 1800 (30 min). Raise for very large registries.
	TimeoutSeconds int `json:"timeout_seconds" yaml:"timeout_seconds"`
}

// DockeryWebhook configures inbound notifications from distribution
// registry. The secret file is read-or-generated on boot and the same
// value is templated into the registry's `notifications.endpoints[].headers.Authorization`
// by the supervisord registry wrapper — both sides then share one token.
type DockeryWebhook struct {
	// SecretPath is the file holding the 32-byte hex shared secret.
	// Auto-generated on first boot if missing. Default:
	// /data/config/webhook-secret.
	SecretPath string `json:"secret_path" yaml:"secret_path"`
}

// DockeryRegistry tells dockery-api where the local distribution
// registry lives. Everything that used to be hardcoded 127.0.0.1:5001
// flows through here so dev setups (different ports) and container
// (loopback + private port) share the same code path.
type DockeryRegistry struct {
	// UpstreamURL is the base URL dockery-api uses for its own calls
	// into /v2/ (reconciler, meta refresh, proxy). Default:
	// http://127.0.0.1:5001.
	UpstreamURL string `json:"upstream_url" yaml:"upstream_url"`
}

// DockerySession configures the Web UI cookie session, backed by
// kratoscarf's auth/session package (server-side store, random session
// ID in cookie). Distinct from the Ed25519 registry token.
//
// Session data lives in an in-memory store for M3; M4 can swap in a
// SQLite-backed Store so sessions survive dockery-api restarts.
type DockerySession struct {
	// Session lifetime in hours. Default: 168 (7 days).
	TTLHours int `json:"ttl_hours" yaml:"ttl_hours"`
	// HTTP cookie name. Default: "dockery_session".
	CookieName string `json:"cookie_name" yaml:"cookie_name"`
	// Set to true when serving over HTTPS (production). Marks cookie Secure.
	CookieSecure bool `json:"cookie_secure" yaml:"cookie_secure"`
}
