package server

import (
	"context"

	kratosmiddleware "github.com/go-kratos/kratos/v2/middleware"
)

// RequireSession ensures a valid Dockery UI session is attached to the
// request (HttpOnly cookie + signed JWT). On success, it injects the
// authenticated user into the request context for downstream handlers.
//
// M1 stub: pass-through. M3 replaces with kratoscarf auth/session logic.
func RequireSession() kratosmiddleware.Middleware {
	return func(next kratosmiddleware.Handler) kratosmiddleware.Handler {
		return func(ctx context.Context, req any) (any, error) {
			// TODO(M3): parse session cookie → verify JWT → load user → inject into ctx.
			return next(ctx, req)
		}
	}
}

// RequireAdmin rejects requests whose session user is not role=admin.
// Must be stacked AFTER RequireSession in the middleware chain.
//
// M1 stub: pass-through. M3 replaces with real role check.
func RequireAdmin() kratosmiddleware.Middleware {
	return func(next kratosmiddleware.Handler) kratosmiddleware.Handler {
		return func(ctx context.Context, req any) (any, error) {
			// TODO(M3): require role == "admin" on user from ctx.
			return next(ctx, req)
		}
	}
}
