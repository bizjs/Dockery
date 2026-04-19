package service

import (
	"errors"
	"net/http"

	"api/internal/biz"

	"github.com/bizjs/kratoscarf/response"
	"github.com/bizjs/kratoscarf/router"
)

// AdminService hosts endpoints only admins reach: maintenance
// operations (GC, key rotation — the latter still M4) and audit log
// queries.
type AdminService struct {
	audit *biz.AuditUsecase
	gc    *biz.GCRunner
}

func NewAdminService(audit *biz.AuditUsecase, gc *biz.GCRunner) *AdminService {
	return &AdminService{audit: audit, gc: gc}
}

// --- DTOs ---

// GCResponse describes one GC run. Success=false means the run was
// attempted but something in the stop / gc / restart sequence failed;
// Error holds a short human-readable summary and OutputTail carries
// whatever supervisorctl / registry printed so operators can diagnose
// without shelling into the container.
type GCResponse struct {
	Success    bool   `json:"success"`
	DurationMs int64  `json:"duration_ms"`
	Error      string `json:"error,omitempty"`
	OutputTail string `json:"output_tail,omitempty"`
}

type RotateKeyResponse struct {
	OldFingerprint string `json:"old_fingerprint"`
	NewFingerprint string `json:"new_fingerprint"`
}

type AuditEntryView struct {
	ID       int64          `json:"id"`
	Ts       int64          `json:"ts"`
	Actor    string         `json:"actor"`
	Action   string         `json:"action"`
	Target   string         `json:"target,omitempty"`
	Scope    string         `json:"scope,omitempty"`
	ClientIP string         `json:"client_ip,omitempty"`
	Success  bool           `json:"success"`
	Detail   map[string]any `json:"detail,omitempty"`
}

type AuditListView struct {
	Items []AuditEntryView `json:"items"`
	Total int              `json:"total"`
}

type AuditQuery struct {
	Actor  string `form:"actor" validate:"omitempty,max=64"`
	Action string `form:"action" validate:"omitempty,max=64"`
	Since  int64  `form:"since"` // unix seconds inclusive
	Until  int64  `form:"until"`
	Limit  int    `form:"limit" validate:"omitempty,min=1,max=500"`
	Offset int    `form:"offset" validate:"omitempty,min=0"`
}

// --- Handlers ---

// TriggerGC runs distribution's garbage-collect in a read-only window.
// Single-flight: concurrent callers get 409 Conflict. The handler
// blocks for the whole cycle (stop → gc → restart); operators should
// expect seconds-to-minutes depending on registry size.
//
// GC failures (supervisorctl missing, registry binary error, etc.) are
// surfaced as 200 with success=false + error + output_tail rather than
// a bare 500 — the underlying command output is the only useful
// diagnostic, and dropping it behind kratoscarf's generic
// "internal server error" frustrated the operator case this endpoint
// exists to serve.
//
// See biz/gc.go for the orchestration details.
func (s *AdminService) TriggerGC(ctx *router.Context) error {
	result, err := s.gc.Run(ctx.Context(), sessionUsername(ctx), ctx.ClientIP())
	if err != nil {
		if errors.Is(err, biz.ErrGCAlreadyRunning) {
			return response.NewBizError(http.StatusConflict, 40901, "gc already in progress")
		}
		resp := GCResponse{
			Success: false,
			Error:   err.Error(),
		}
		if result != nil {
			resp.DurationMs = result.Duration.Milliseconds()
			resp.OutputTail = tailLines(result.Output, 40)
		}
		return ctx.Success(resp)
	}
	return ctx.Success(GCResponse{
		Success:    true,
		DurationMs: result.Duration.Milliseconds(),
		OutputTail: tailLines(result.Output, 40),
	})
}

// RotateKey generates a new Ed25519 signing key, writes it to
// /data/config/jwt-*.pem, then restarts the registry process via
// supervisorctl so the new public key is loaded. All previously issued
// registry tokens become invalid immediately — users must re-login
// on both UI and docker CLI.
func (s *AdminService) RotateKey(ctx *router.Context) error {
	// TODO(M4)
	return errNotImplemented()
}

// Audit returns a filtered slice of the audit_log table, most recent
// first. Limit defaults to 100 and is capped at 500.
func (s *AdminService) Audit(ctx *router.Context) error {
	var q AuditQuery
	if err := ctx.BindQuery(&q); err != nil {
		return err
	}
	rows, total, err := s.audit.Query(ctx.Context(), biz.AuditFilter{
		Actor:  q.Actor,
		Action: q.Action,
		Since:  q.Since,
		Until:  q.Until,
		Limit:  q.Limit,
		Offset: q.Offset,
	})
	if err != nil {
		return response.ErrInternal.WithCause(err)
	}
	items := make([]AuditEntryView, 0, len(rows))
	for _, r := range rows {
		items = append(items, AuditEntryView{
			ID:       r.ID,
			Ts:       r.Ts,
			Actor:    r.Actor,
			Action:   r.Action,
			Target:   r.Target,
			Scope:    r.Scope,
			ClientIP: r.ClientIP,
			Success:  r.Success,
			Detail:   r.Detail,
		})
	}
	return ctx.Success(AuditListView{Items: items, Total: total})
}

// tailLines returns the last N lines of s. Used to keep GC response
// payloads bounded when registry prints thousands of swept blob lines.
func tailLines(s string, n int) string {
	if s == "" || n <= 0 {
		return ""
	}
	// Count newlines backwards from the end.
	count := 0
	for i := len(s) - 1; i >= 0; i-- {
		if s[i] == '\n' {
			count++
			if count == n+1 {
				return s[i+1:]
			}
		}
	}
	return s
}
