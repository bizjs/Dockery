package service

import (
	"api/internal/data"

	"github.com/bizjs/kratoscarf/router"
)

// AuthService handles Web UI session login/logout and "who am I" queries.
// Token issuance for the docker CLI lives in TokenService (/token).
type AuthService struct {
	data *data.Data
}

func NewAuthService(d *data.Data) *AuthService { return &AuthService{data: d} }

// --- DTOs ---

type LoginRequest struct {
	Username string `json:"username" validate:"required,min=1,max=64"`
	Password string `json:"password" validate:"required,min=1,max=256"`
}

type LoginResponse struct {
	Username  string `json:"username"`
	Role      string `json:"role"`
	ExpiresAt int64  `json:"expires_at"`
}

type MeResponse struct {
	Username string `json:"username"`
	Role     string `json:"role"`
}

// --- Handlers ---

// Login verifies the credentials against the users table and, on success,
// issues a session JWT delivered as a HttpOnly cookie.
func (s *AuthService) Login(ctx *router.Context) error {
	var req LoginRequest
	if err := ctx.Bind(&req); err != nil {
		return err
	}
	// TODO(M2): bcrypt compare → IssueSessionToken → Set-Cookie
	return errNotImplemented()
}

// Logout clears the session cookie.
func (s *AuthService) Logout(ctx *router.Context) error {
	// TODO(M3): clear Set-Cookie.
	return errNotImplemented()
}

// Me returns the current session user profile.
func (s *AuthService) Me(ctx *router.Context) error {
	// TODO(M3): read user from session context populated by RequireSession middleware.
	return errNotImplemented()
}
