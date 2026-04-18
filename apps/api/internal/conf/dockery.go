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
}

// DockeryKeystore is the filesystem location of the Ed25519 signing
// keypair Dockery API uses to sign registry JWTs and Distribution
// Registry uses to verify them.
type DockeryKeystore struct {
	PrivatePath string `json:"private_path" yaml:"private_path"`
	PublicPath  string `json:"public_path"  yaml:"public_path"`
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
