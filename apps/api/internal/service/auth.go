package service

import (
	"net/http"

	"api/internal/biz"

	"github.com/bizjs/kratoscarf/auth/session"
	"github.com/bizjs/kratoscarf/response"
	"github.com/bizjs/kratoscarf/router"
)

// Keys mirrored from internal/server/middleware.go. Keeping them
// duplicated (rather than exporting) scopes session-shape knowledge
// to the two files that need it.
const (
	sessKeyUserID   = "user_id"
	sessKeyUsername = "username"
	sessKeyRole     = "role"
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

// Login verifies credentials and, on success, populates the
// kratoscarf session. kratoscarf's session middleware will persist
// the session to the store and emit the Set-Cookie header after the
// handler returns because Set() marks the session as Modified.
func (s *AuthService) Login(ctx *router.Context) error {
	var req LoginRequest
	if err := ctx.Bind(&req); err != nil {
		return err
	}
	user, err := s.users.VerifyCredentials(ctx.Context(), req.Username, req.Password)
	if err != nil {
		return response.NewBizError(http.StatusUnauthorized, 40101, "invalid credentials")
	}

	sess := session.FromContext(ctx.Context())
	if sess == nil {
		return response.ErrInternal.WithMessage("session middleware not attached")
	}
	sess.Set(sessKeyUserID, user.ID)
	sess.Set(sessKeyUsername, user.Username)
	sess.Set(sessKeyRole, user.Role)

	return ctx.Success(LoginResponse{Username: user.Username, Role: user.Role})
}

// Logout clears the session values. The cookie itself lives on until
// its TTL; a subsequent request with the cookie will load an empty
// session and the RequireSession middleware will reject it as
// unauthenticated. (Proper DestroySession via Manager requires the
// raw ResponseWriter, which we don't expose to biz — M4 can switch
// to a SQLite-backed Store with explicit revocation if needed.)
func (s *AuthService) Logout(ctx *router.Context) error {
	sess := session.FromContext(ctx.Context())
	if sess != nil {
		sess.Delete(sessKeyUserID)
		sess.Delete(sessKeyUsername)
		sess.Delete(sessKeyRole)
	}
	return ctx.Success(nil)
}

// Me returns the currently authenticated user's profile.
// RequireSession middleware guarantees user_id is present when we
// reach this handler.
func (s *AuthService) Me(ctx *router.Context) error {
	sess := session.FromContext(ctx.Context())
	if sess == nil {
		return response.ErrUnauthorized
	}
	name, _ := sess.Get(sessKeyUsername)
	role, _ := sess.Get(sessKeyRole)
	nameStr, _ := name.(string)
	roleStr, _ := role.(string)
	return ctx.Success(MeResponse{Username: nameStr, Role: roleStr})
}

// sessionUserID returns the authenticated user's id from the session,
// or 0 if the session is empty. Handlers that must enforce
// self-or-admin semantics call this to compare against the URL id.
func sessionUserID(ctx *router.Context) int {
	sess := session.FromContext(ctx.Context())
	if sess == nil {
		return 0
	}
	v, _ := sess.Get(sessKeyUserID)
	id, _ := v.(int)
	return id
}

// sessionRole returns the authenticated user's role, or empty string.
func sessionRole(ctx *router.Context) string {
	sess := session.FromContext(ctx.Context())
	if sess == nil {
		return ""
	}
	v, _ := sess.Get(sessKeyRole)
	s2, _ := v.(string)
	return s2
}
