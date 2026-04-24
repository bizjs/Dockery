package biz

import (
	"context"

	"github.com/go-kratos/kratos/v2/log"
)

// Canonical audit action values. Keeping them as constants stops us
// from drifting across handlers ("login.failed" vs "login.failure").
const (
	ActionTokenIssued         = "token.issued"
	ActionTokenDenied         = "token.denied"
	ActionAuthLoginSuccess    = "auth.login.success"
	ActionAuthLoginFailure    = "auth.login.failure"
	ActionUserCreated         = "user.created"
	ActionUserRoleChanged     = "user.role_changed"
	ActionUserDisabled        = "user.disabled"
	ActionUserEnabled         = "user.enabled"
	ActionUserDeleted         = "user.deleted"
	ActionUserPasswordChanged = "user.password_changed"
	ActionPermissionGranted   = "permission.granted"
	ActionPermissionUpdated   = "permission.updated"
	ActionPermissionRevoked   = "permission.revoked"
	ActionImageDeleted        = "image.deleted"
	ActionGCStarted           = "gc.started"
	ActionGCCompleted         = "gc.completed"
	ActionCacheResynced       = "registry.cache.resynced"
	ActionKeyRotated          = "key.rotated"
	// Reconciler discrepancies — triggered when the repo_meta cache
	// drifts from upstream /v2/_catalog (missed webhook event, etc.).
	// Writing these means something was wrong at the moment of check;
	// the row is already being fixed when the audit fires.
	ActionReconcileAdded   = "registry.reconcile.added"
	ActionReconcileRemoved = "registry.reconcile.removed"
)

// AuditEntry is the biz-layer view of one audit_log row. Ts is unix
// seconds (data layer converts to/from time.Time).
type AuditEntry struct {
	ID       int64
	Ts       int64
	Actor    string
	Action   string
	Target   string
	Scope    string
	ClientIP string
	Success  bool
	Detail   map[string]any
}

// AuditFilter narrows a Query. Zero values mean "no filter".
type AuditFilter struct {
	Actor  string
	Action string
	Since  int64 // unix seconds, inclusive
	Until  int64 // unix seconds, inclusive
	Limit  int   // 0 → default 100; capped in biz
	Offset int
}

// AuditRepo is the data-layer contract. Implemented in
// internal/data/audit.go; mocked in biz tests.
type AuditRepo interface {
	Create(ctx context.Context, e *AuditEntry) error
	Query(ctx context.Context, f AuditFilter) (items []*AuditEntry, total int, err error)
}

// AuditUsecase owns audit writes + reads. Write failures are logged but
// NOT propagated to callers — an audit hiccup must never break the
// business operation that produced it. Query failures do surface.
type AuditUsecase struct {
	repo AuditRepo
	log  *log.Helper
}

func NewAuditUsecase(repo AuditRepo, logger log.Logger) *AuditUsecase {
	return &AuditUsecase{
		repo: repo,
		log:  log.NewHelper(log.With(logger, "module", "biz/audit")),
	}
}

// Write appends one audit row. Errors are logged but swallowed so the
// caller never has to wrap each call in an error branch.
func (u *AuditUsecase) Write(ctx context.Context, e AuditEntry) {
	if u == nil {
		return
	}
	if err := u.repo.Create(ctx, &e); err != nil {
		u.log.Errorf("audit write failed: action=%s actor=%s err=%v", e.Action, e.Actor, err)
	}
}

// Query pulls rows with filter + pagination. Limit is clamped to [1, 500].
func (u *AuditUsecase) Query(ctx context.Context, f AuditFilter) ([]*AuditEntry, int, error) {
	if f.Limit <= 0 {
		f.Limit = 100
	}
	if f.Limit > 500 {
		f.Limit = 500
	}
	if f.Offset < 0 {
		f.Offset = 0
	}
	return u.repo.Query(ctx, f)
}
