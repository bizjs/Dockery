package service

import (
	"api/internal/data"

	"github.com/bizjs/kratoscarf/router"
)

// AdminService hosts endpoints that only an admin should reach:
// maintenance operations and audit log queries.
type AdminService struct {
	data *data.Data
}

func NewAdminService(d *data.Data) *AdminService { return &AdminService{data: d} }

// --- DTOs ---

type GCResponse struct {
	Started bool   `json:"started"`
	JobID   string `json:"job_id,omitempty"`
}

type RotateKeyResponse struct {
	OldFingerprint string `json:"old_fingerprint"`
	NewFingerprint string `json:"new_fingerprint"`
}

type AuditEntryView struct {
	ID       int64  `json:"id"`
	Ts       int64  `json:"ts"`
	Actor    string `json:"actor"`
	Action   string `json:"action"`
	Target   string `json:"target,omitempty"`
	Scope    string `json:"scope,omitempty"`
	ClientIP string `json:"client_ip,omitempty"`
	Success  bool   `json:"success"`
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
// The flow: set maintenance flag (reject writes) → stop registry → run
// `registry garbage-collect …` → restart registry → clear flag.
func (s *AdminService) TriggerGC(ctx *router.Context) error {
	// TODO(M4)
	return errNotImplemented()
}

// RotateKey generates a new Ed25519 signing key, writes it to
// /data/config/jwt-*.pem, then restarts the registry process via
// s6-svc -r so the new public key is loaded. All previously issued
// registry tokens become invalid immediately — users must re-login
// on both UI and docker CLI.
func (s *AdminService) RotateKey(ctx *router.Context) error {
	// TODO(M4)
	return errNotImplemented()
}

// Audit returns a filtered slice of the audit_log table.
func (s *AdminService) Audit(ctx *router.Context) error {
	var q AuditQuery
	if err := ctx.BindQuery(&q); err != nil {
		return err
	}
	// TODO(M4)
	return errNotImplemented()
}
