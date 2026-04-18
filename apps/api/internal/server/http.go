package server

import (
	"api/internal/conf"
	"api/internal/service"

	"github.com/bizjs/kratoscarf/auth/session"
	"github.com/bizjs/kratoscarf/middleware"
	"github.com/bizjs/kratoscarf/response"
	"github.com/bizjs/kratoscarf/router"
	"github.com/bizjs/kratoscarf/validation"

	"github.com/go-kratos/kratos/v2/log"
	"github.com/go-kratos/kratos/v2/middleware/recovery"
	"github.com/go-kratos/kratos/v2/transport/http"
)

// NewHTTPServer wires a Kratos HTTP server that exposes the Dockery API
// through kratoscarf's router. All routes go through:
//
//   - Filter: CORS + Secure headers (HTTP-level)
//   - Middleware: Recovery + RequestID (Kratos-level)
//   - ErrorEncoder: kratoscarf {code,message,data} envelope
//   - Validator: auto-validates on ctx.Bind / ctx.BindQuery
//   - Response wrapper: ctx.Success wraps payloads in the envelope
//
// Routes are grouped by authentication requirement; the /token endpoint
// bypasses the envelope (Docker spec requires its own JSON shape) by
// writing via ctx.JSON directly.
func NewHTTPServer(c *conf.Server, svcs *service.Services, sm *session.Manager, logger log.Logger) *http.Server {
	opts := []http.ServerOption{
		// Route HTTP transport logs (e.g. "server listening on …")
		// through the injected logger; tests can feed io.Discard to
		// stay quiet, production keeps them on stdout.
		http.Logger(logger),
		http.ErrorEncoder(response.NewHTTPErrorEncoder()),
		http.Filter(
			middleware.CORS(),
			middleware.Secure(middleware.SecureConfig{}),
		),
		http.Middleware(
			recovery.Recovery(),
			middleware.RequestID(),
		),
	}
	if c.Http.Network != "" {
		opts = append(opts, http.Network(c.Http.Network))
	}
	if c.Http.Addr != "" {
		opts = append(opts, http.Address(c.Http.Addr))
	}
	if c.Http.Timeout != nil {
		opts = append(opts, http.Timeout(c.Http.Timeout.AsDuration()))
	}

	srv := http.NewServer(opts...)

	r := router.NewRouter(srv,
		router.WithValidator(validation.New()),
		router.WithResponseWrapper(response.Wrap),
	)

	registerRoutes(r, svcs, sm)
	return srv
}
