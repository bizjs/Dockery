package server

import (
	v1 "api/api/helloworld/v1"
	"api/internal/conf"
	"api/internal/service"

	"github.com/bizjs/kratoscarf/middleware"
	"github.com/bizjs/kratoscarf/response"
	"github.com/bizjs/kratoscarf/router"
	"github.com/bizjs/kratoscarf/validation"

	"github.com/go-kratos/kratos/v2/log"
	"github.com/go-kratos/kratos/v2/middleware/recovery"
	"github.com/go-kratos/kratos/v2/transport/http"
)

// NewHTTPServer wires a Kratos HTTP server with kratoscarf conventions:
//   - ErrorEncoder: all errors emit the {code, message, data} envelope
//   - CORS + Secure headers as HTTP filters
//   - RequestID + Recovery as Kratos middleware
//   - Router group with auto-validate + auto-wrap for ctx.Success
//
// ResponseEncoder is intentionally NOT overridden, so proto-generated
// greeter handlers keep their native JSON shape; only kratoscarf
// ctx.Success() calls go through response.Wrap.
func NewHTTPServer(
	c *conf.Server,
	greeter *service.GreeterService,
	system *service.SystemService,
	logger log.Logger,
) *http.Server {
	opts := []http.ServerOption{
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

	// Proto-generated routes (greeter demo; replaced in M2).
	v1.RegisterGreeterHTTPServer(srv, greeter)

	// Kratoscarf-managed routes: health, ping, and (in M2) business APIs.
	r := router.NewRouter(srv,
		router.WithValidator(validation.New()),
		router.WithResponseWrapper(response.Wrap),
	)
	system.Register(r)

	return srv
}
