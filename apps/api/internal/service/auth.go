package service

import (
	"net/http"

	"api/internal/biz"

	"github.com/bizjs/kratoscarf/response"
	"github.com/bizjs/kratoscarf/router"
)

// AuthService handles Web UI session login/logout and "who am I".
// Docker CLI token issuance lives in TokenService.
type AuthService struct {
	users *biz.UserUsecase
}

func NewAuthService(users *biz.UserUsecase) *AuthService {
	return &AuthService{users: users}
}

// --- DTOs ---

type LoginRequest struct {
	Username string `json:"username" validate:"required,min=1,max=64"`
	Password string `json:"password" validate:"required,min=1,max=256"`
}

type LoginResponse struct {
	Username string `json:"username"`
	Role     string `json:"role"`
}

type MeResponse struct {
	Username string `json:"username"`
	Role     string `json:"role"`
}

// --- Handlers ---

// Login verifies credentials. Session-cookie issuance is deferred to
// M3; for now the client can still discover whether credentials are
// valid and what role the user has (useful for the CLI and for UI
// prototyping against the same endpoint).
func (s *AuthService) Login(ctx *router.Context) error {
	var req LoginRequest
	if err := ctx.Bind(&req); err != nil {
		return err
	}
	user, err := s.users.VerifyCredentials(ctx.Context(), req.Username, req.Password)
	if err != nil {
		return response.NewBizError(http.StatusUnauthorized, 40101, "invalid credentials")
	}
	return ctx.Success(LoginResponse{Username: user.Username, Role: user.Role})
}

// Logout — M3: clear session cookie.
func (s *AuthService) Logout(ctx *router.Context) error {
	return errNotImplemented()
}

// Me — M3: read user from session-gated context.
func (s *AuthService) Me(ctx *router.Context) error {
	return errNotImplemented()
}
