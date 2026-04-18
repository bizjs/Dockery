package server

import (
	"api/internal/service"

	"github.com/bizjs/kratoscarf/router"
)

// registerRoutes wires every Dockery endpoint onto the router, grouped
// by authentication layer. Keeping the full surface area in one file
// makes it straightforward to audit what is public vs. session-gated
// vs. admin-only without chasing per-domain helpers.
func registerRoutes(r *router.Router, svcs *service.Services) {
	registerPublicRoutes(r, svcs)
	registerSessionRoutes(r, svcs)
	registerAdminRoutes(r, svcs)
}

// registerPublicRoutes mounts endpoints reachable without authentication.
// The /token endpoint parses Basic Auth inside its handler and replies in
// the Docker-spec response shape rather than the kratoscarf envelope.
func registerPublicRoutes(r *router.Router, svcs *service.Services) {
	svcs.System.Register(r) // /healthz, /readyz, /ping

	r.GET("/token", svcs.Token.Issue)
	r.POST("/api/auth/login", svcs.Auth.Login)
}

// registerSessionRoutes mounts endpoints that require a valid UI session
// (HttpOnly cookie + JWT) but do not require admin role.
func registerSessionRoutes(r *router.Router, svcs *service.Services) {
	g := r.Group("/api", RequireSession())

	g.POST("/auth/logout", svcs.Auth.Logout)
	g.GET("/auth/me", svcs.Auth.Me)

	// Password change is session-only; the handler enforces
	// "id==caller OR caller.role==admin".
	g.PUT("/users/{id}/password", svcs.User.SetPassword)

	// UI → upstream-registry proxy (JWT injected server-side).
	// NOTE: {name} defaults to [^/]+ in gorilla/mux. Docker image
	// names commonly contain slashes (e.g. "alice/app"); M2/M3 will
	// switch to {name:.+} with careful trailing-literal regexes when
	// the proxy is actually implemented.
	g.GET("/registry/catalog", svcs.Registry.Catalog)
	g.GET("/registry/{name}/tags", svcs.Registry.Tags)
	g.GET("/registry/{name}/manifests/{ref}", svcs.Registry.GetManifest)
	g.DELETE("/registry/{name}/manifests/{ref}", svcs.Registry.DeleteManifest)
	g.GET("/registry/{name}/blobs/{digest}", svcs.Registry.GetBlob)
}

// registerAdminRoutes mounts endpoints that additionally require
// role == admin. RequireSession must run before RequireAdmin so that
// the user identity is available when checking the role.
func registerAdminRoutes(r *router.Router, svcs *service.Services) {
	g := r.Group("/api", RequireSession(), RequireAdmin())

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
