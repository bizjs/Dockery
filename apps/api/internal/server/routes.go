package server

import (
	"api/internal/service"

	"github.com/bizjs/kratoscarf/auth/session"
	"github.com/bizjs/kratoscarf/router"
)

// registerRoutes wires every Dockery endpoint onto the router.
//
// Three-tier grouping layered via kratoscarf router.Group (middleware
// accumulates across nested groups):
//
//   - public          — /healthz /readyz /ping /token   (no session)
//   - /api + session  — /api/auth/login, other /api endpoints gain
//     session context so handlers can populate it.
//   - /api + session + RequireSession             — logged-in users.
//   - /api + session + RequireSession + RequireAdmin — admin only.
func registerRoutes(r *router.Router, svcs *service.Services, sm *session.Manager) {
	registerPublicRoutes(r, svcs)

	// All /api/* requests load a session so Login can write into it
	// and middleware further down can read it.
	api := r.Group("/api", session.Middleware(sm))
	registerAuthRoutes(api, svcs)
	registerSessionRoutes(api, svcs)
	registerAdminRoutes(api, svcs)
}

// registerPublicRoutes — no authentication, no session loading.
// The /token endpoint parses its own Basic Auth header and returns
// the Docker-spec JSON shape via ctx.JSON.
func registerPublicRoutes(r *router.Router, svcs *service.Services) {
	svcs.System.Register(r)
	r.GET("/token", svcs.Token.Issue)
	// Webhook callback from the upstream distribution registry. Auth'd
	// via a shared bearer secret (see biz/webhook_secret.go), not session.
	// Loopback-only in the container image; in remote deployments, the
	// route should be restricted at the nginx layer too.
	r.POST("/api/internal/registry-events", svcs.Webhook.Handle)
}

// registerAuthRoutes — endpoints that need session context but not
// a valid authenticated session (login is where the session is
// populated for the first time).
func registerAuthRoutes(api *router.Router, svcs *service.Services) {
	api.POST("/auth/login", svcs.Auth.Login)
}

// registerSessionRoutes — must have an authenticated session but no
// role restriction.
func registerSessionRoutes(api *router.Router, svcs *service.Services) {
	g := api.Group("", RequireSession())

	g.POST("/auth/logout", svcs.Auth.Logout)
	g.GET("/auth/me", svcs.Auth.Me)

	// Self-service password change — handler enforces self-or-admin.
	g.PUT("/users/{id}/password", svcs.User.SetPassword)

	// UI → upstream-registry proxy.
	// {name:.+} accepts multi-segment Docker image names like
	// "demo/hello" or "org/team/svc". gorilla/mux backtracks the greedy
	// `.+` against the trailing literal (/tags, /manifests, /blobs) so
	// the match unambiguously resolves.
	g.GET("/registry/catalog", svcs.Registry.Catalog)
	g.GET("/registry/overview", svcs.Registry.Overview)
	g.GET("/registry/{name:.+}/tags", svcs.Registry.Tags)
	g.GET("/registry/{name:.+}/manifests/{ref}", svcs.Registry.GetManifest)
	g.DELETE("/registry/{name:.+}/manifests/{ref}", svcs.Registry.DeleteManifest)
	g.GET("/registry/{name:.+}/blobs/{digest}", svcs.Registry.GetBlob)
}

// registerAdminRoutes — authenticated AND role=admin.
func registerAdminRoutes(api *router.Router, svcs *service.Services) {
	g := api.Group("", RequireSession(), RequireAdmin())

	g.GET("/users", svcs.User.List)
	g.POST("/users", svcs.User.Create)
	g.GET("/users/{id}", svcs.User.Get)
	g.PATCH("/users/{id}", svcs.User.Update)
	g.DELETE("/users/{id}", svcs.User.Delete)

	g.GET("/users/{id}/permissions", svcs.Permission.ListForUser)
	g.POST("/users/{id}/permissions", svcs.Permission.GrantBatch)
	g.PATCH("/permissions/{id}", svcs.Permission.Update)
	g.DELETE("/permissions/{id}", svcs.Permission.Revoke)

	g.POST("/admin/gc", svcs.Admin.TriggerGC)
	g.POST("/admin/rotate-signing-key", svcs.Admin.RotateKey)
	g.GET("/audit", svcs.Admin.Audit)
}
