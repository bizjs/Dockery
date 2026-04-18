package server

import (
	"context"
	"net/http"

	"github.com/bizjs/kratoscarf/auth/session"
	"github.com/bizjs/kratoscarf/response"
	kratosmiddleware "github.com/go-kratos/kratos/v2/middleware"
)

// Session keys we stash into the kratoscarf session.Values map on login.
const (
	sessionKeyUserID   = "user_id"
	sessionKeyUsername = "username"
	sessionKeyRole     = "role"
)

// RequireSession rejects requests without an authenticated session. It
// assumes session.Middleware has already run upstream in the router
// group so session.FromContext returns the loaded (possibly empty)
// session.
//
// A session is considered authenticated iff user_id was set by Login.
func RequireSession() kratosmiddleware.Middleware {
	return func(next kratosmiddleware.Handler) kratosmiddleware.Handler {
		return func(ctx context.Context, req any) (any, error) {
			sess := session.FromContext(ctx)
			if sess == nil || sess.IsNew {
				return nil, errUnauthorized
			}
			if _, ok := sess.Get(sessionKeyUserID); !ok {
				return nil, errUnauthorized
			}
			return next(ctx, req)
		}
	}
}

// RequireAdmin layers on top of RequireSession: the authenticated user
// must have role == admin. Stack AFTER RequireSession so the Get below
// is guaranteed populated.
func RequireAdmin() kratosmiddleware.Middleware {
	return func(next kratosmiddleware.Handler) kratosmiddleware.Handler {
		return func(ctx context.Context, req any) (any, error) {
			sess := session.FromContext(ctx)
			if sess == nil {
				return nil, errUnauthorized
			}
			role, _ := sess.Get(sessionKeyRole)
			if role != "admin" {
				return nil, errForbidden
			}
			return next(ctx, req)
		}
	}
}

// Common error envelopes returned by auth middleware. ErrorEncoder in
// http.go translates the BizError HTTPCode into the response status.
var (
	errUnauthorized = response.NewBizError(http.StatusUnauthorized, 40100, "authentication required")
	errForbidden    = response.NewBizError(http.StatusForbidden, 40300, "admin role required")
)
