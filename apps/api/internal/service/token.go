package service

import (
	"encoding/base64"
	"errors"
	"net/http"
	"strings"
	"time"

	"api/internal/biz"
	"api/internal/pkg/scope"

	"github.com/bizjs/kratoscarf/router"
)

// TokenService implements the Docker Registry token auth realm.
// The response shape is fixed by the Docker spec — we bypass the
// kratoscarf envelope by writing through ctx.JSON directly.
type TokenService struct {
	users *biz.UserUsecase
	perms *biz.PermissionUsecase
	iss   *biz.TokenIssuer
	audit *biz.AuditUsecase
}

func NewTokenService(users *biz.UserUsecase, perms *biz.PermissionUsecase, iss *biz.TokenIssuer, audit *biz.AuditUsecase) *TokenService {
	return &TokenService{users: users, perms: perms, iss: iss, audit: audit}
}

// TokenResponse is the Docker-spec success payload.
type TokenResponse struct {
	Token       string `json:"token"`
	AccessToken string `json:"access_token"`
	ExpiresIn   int    `json:"expires_in"`
	IssuedAt    string `json:"issued_at"`
}

// dockerError is the error shape Docker clients expect on 4xx responses.
type dockerError struct {
	Errors []dockerErrorItem `json:"errors"`
}
type dockerErrorItem struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Detail  any    `json:"detail,omitempty"`
}

func writeDockerError(ctx *router.Context, status int, code, msg string) error {
	// Challenge the client in a Docker-compatible way.
	// The handler returns nil because we've fully written the response;
	// kratoscarf must not wrap it in the {code,message,data} envelope.
	ctx.SetHeader("WWW-Authenticate", `Basic realm="dockery"`)
	return ctx.JSON(status, dockerError{
		Errors: []dockerErrorItem{{Code: code, Message: msg}},
	})
}

// Issue is hit by the docker CLI on the 401 / WWW-Authenticate bounce.
func (s *TokenService) Issue(ctx *router.Context) error {
	username, password, ok := parseBasicAuth(ctx.Header("Authorization"))
	if !ok {
		// Probe request (no creds) — Docker login uses this to detect the realm.
		// Return a valid but-empty-access token so docker treats the login as OK.
		// This matches Docker Hub's behaviour for the initial handshake.
		return s.issueAnonymous(ctx)
	}

	clientIP := ctx.ClientIP()
	requestedScopes := ctx.QueryArray("scope")

	user, err := s.users.VerifyCredentials(ctx.Context(), username, password)
	if err != nil {
		s.audit.Write(ctx.Context(), biz.AuditEntry{
			Actor:    username,
			Action:   biz.ActionTokenDenied,
			Scope:    joinScopes(requestedScopes),
			ClientIP: clientIP,
			Success:  false,
			Detail:   map[string]any{"reason": "invalid credentials"},
		})
		return writeDockerError(ctx, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
	}

	// Parse requested scopes from query. The Docker CLI passes "scope"
	// possibly multiple times.
	requested, _ := scope.ParseMany(requestedScopes)

	// Resolve what the user is actually allowed to do.
	resolved, err := s.perms.ResolveAccess(ctx.Context(), user, requested)
	if err != nil {
		return writeDockerError(ctx, http.StatusInternalServerError, "DENIED", "authorization lookup failed")
	}

	access := make([]biz.RegistryAccess, 0, len(resolved))
	for _, r := range resolved {
		access = append(access, biz.RegistryAccess{
			Type: r.Type, Name: r.Name, Actions: r.Actions,
		})
	}

	tok, err := s.iss.IssueRegistryToken(user.Username, access)
	if err != nil {
		return writeDockerError(ctx, http.StatusInternalServerError, "DENIED", "token signing failed")
	}

	s.audit.Write(ctx.Context(), biz.AuditEntry{
		Actor:    user.Username,
		Action:   biz.ActionTokenIssued,
		Scope:    describeAccess(access),
		ClientIP: clientIP,
		Success:  true,
	})

	return ctx.JSON(http.StatusOK, TokenResponse{
		Token:       tok,
		AccessToken: tok,
		ExpiresIn:   s.iss.ExpiresIn(),
		IssuedAt:    time.Now().UTC().Format(time.RFC3339),
	})
}

// describeAccess renders the granted scopes as the Docker-style
// "type:name:actions" CSV (matches what the CLI sends in via `scope=`).
// Used for the audit row so operators can grep for suspicious grants.
func describeAccess(access []biz.RegistryAccess) string {
	if len(access) == 0 {
		return ""
	}
	parts := make([]string, 0, len(access))
	for _, a := range access {
		parts = append(parts, a.Type+":"+a.Name+":"+strings.Join(a.Actions, ","))
	}
	return strings.Join(parts, " ")
}

// joinScopes concatenates the raw incoming `scope=` params. Used in the
// token.denied audit row when we don't have a parsed access list.
func joinScopes(ss []string) string {
	return strings.Join(ss, " ")
}

// issueAnonymous returns a valid but access-empty token so the Docker
// client's initial `docker login` (which sends no credentials on the
// probe) succeeds with "Login Succeeded". Subsequent push/pull will
// re-challenge with real scopes; this time Basic Auth is attached.
func (s *TokenService) issueAnonymous(ctx *router.Context) error {
	tok, err := s.iss.IssueRegistryToken("", nil)
	if err != nil {
		return writeDockerError(ctx, http.StatusInternalServerError, "DENIED", "token signing failed")
	}
	return ctx.JSON(http.StatusOK, TokenResponse{
		Token:       tok,
		AccessToken: tok,
		ExpiresIn:   s.iss.ExpiresIn(),
		IssuedAt:    time.Now().UTC().Format(time.RFC3339),
	})
}

// parseBasicAuth decodes "Basic base64(user:pass)".
func parseBasicAuth(header string) (user, pass string, ok bool) {
	const prefix = "Basic "
	if !strings.HasPrefix(header, prefix) {
		return "", "", false
	}
	raw, err := base64.StdEncoding.DecodeString(header[len(prefix):])
	if err != nil {
		return "", "", false
	}
	s := string(raw)
	idx := strings.IndexByte(s, ':')
	if idx < 0 {
		return "", "", false
	}
	return s[:idx], s[idx+1:], true
}

// Silence unused import in early builds.
var _ = errors.New
