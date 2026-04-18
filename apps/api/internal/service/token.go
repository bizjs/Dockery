package service

import (
	"api/internal/data"

	"github.com/bizjs/kratoscarf/router"
)

// TokenService implements the Docker Registry token auth realm
// (https://distribution.github.io/distribution/spec/auth/token/).
//
// Contract — the docker CLI hits this endpoint when the registry
// returns 401 with a WWW-Authenticate: Bearer realm="…/token" header:
//
//	GET /token?service=dockery&scope=repository:alice/app:pull,push
//	Authorization: Basic base64(username:password)
//
// Response shape is fixed by the Docker spec (NOT the kratoscarf
// {code,message,data} envelope), so handlers here write with ctx.JSON
// directly rather than ctx.Success.
type TokenService struct {
	data *data.Data
}

func NewTokenService(d *data.Data) *TokenService { return &TokenService{data: d} }

// --- DTOs ---

// TokenResponse is the Docker-specified success payload.
type TokenResponse struct {
	Token       string `json:"token"`
	AccessToken string `json:"access_token"`
	ExpiresIn   int    `json:"expires_in"`
	IssuedAt    string `json:"issued_at"`
}

// --- Handlers ---

// Issue verifies Basic Auth credentials, resolves the caller's role and
// repo_permissions, intersects the requested scope with the permitted
// actions, and returns a short-lived Ed25519-signed JWT that the
// registry will validate.
func (s *TokenService) Issue(ctx *router.Context) error {
	// TODO(M2):
	//   1. parse Authorization: Basic; 401 on failure (Docker JSON shape, not kratoscarf).
	//   2. bcrypt compare against users.password_hash.
	//   3. parse every scope=... query param (may repeat).
	//   4. for each scope: glob-match user permissions, intersect with role actions.
	//   5. sign JWT with /data/config/jwt-private.pem (Ed25519).
	//   6. audit-log 'token.issued' or 'token.denied'.
	//   7. ctx.JSON(200, TokenResponse{...}).
	return errNotImplemented()
}
