package service

import (
	"github.com/bizjs/kratoscarf/router"
)

// SystemService exposes platform-level endpoints (health, ping).
// It also serves as the canonical demonstration that the kratoscarf
// three-layer convention (bind → validate → wrap) is wired correctly:
//
//	GET /healthz       → liveness
//	GET /readyz        → readiness (M2 will gate on DB + keys)
//	GET /ping?name=foo → echo with validation; verifies Bind/Validate/Wrap
type SystemService struct{}

func NewSystemService() *SystemService { return &SystemService{} }

// Register mounts the system routes onto the given router.
func (s *SystemService) Register(r *router.Router) {
	r.GET("/healthz", s.Liveness)
	r.GET("/readyz", s.Readiness)
	r.GET("/ping", s.Ping)
}

type healthStatus struct {
	Status string `json:"status"`
}

// Liveness returns 200 as long as the process is running.
func (s *SystemService) Liveness(ctx *router.Context) error {
	return ctx.Success(healthStatus{Status: "ok"})
}

// Readiness returns 200 once all downstream dependencies are ready.
// M2 will extend this to check the ent client and the JWT signing key
// files; for now liveness implies readiness.
func (s *SystemService) Readiness(ctx *router.Context) error {
	return ctx.Success(healthStatus{Status: "ok"})
}

type pingReq struct {
	Name string `form:"name" json:"name" validate:"required,min=1,max=32"`
}

type pingResp struct {
	Pong string `json:"pong"`
}

// Ping demonstrates the three-layer convention end-to-end:
//   - Bind: BindQuery populates the struct
//   - Validate: WithValidator auto-enforces the `validate` tag (missing/too-long → 422)
//   - Wrap: ctx.Success wraps the payload in {code, message, data}
func (s *SystemService) Ping(ctx *router.Context) error {
	var req pingReq
	if err := ctx.BindQuery(&req); err != nil {
		return err
	}
	return ctx.Success(pingResp{Pong: req.Name})
}
