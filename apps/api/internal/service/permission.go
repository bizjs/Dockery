package service

import (
	"api/internal/data"

	"github.com/bizjs/kratoscarf/router"
)

// PermissionService manages repo_permissions rows for write/view users.
// Admins bypass this table entirely; attempting to grant a permission
// to an admin is an error.
type PermissionService struct {
	data *data.Data
}

func NewPermissionService(d *data.Data) *PermissionService { return &PermissionService{data: d} }

// --- DTOs ---

// GrantPermissionsRequest allows an admin to add multiple patterns in a
// single round-trip. The server splits the list into one row per pattern
// and returns the rows that were actually inserted (duplicates on the
// unique (user_id, repo_pattern) index are silently skipped).
type GrantPermissionsRequest struct {
	RepoPatterns []string `json:"repo_patterns" validate:"required,min=1,max=64,dive,required,min=1,max=256"`
}

type UpdatePermissionRequest struct {
	RepoPattern string `json:"repo_pattern" validate:"required,min=1,max=256"`
}

type PermissionView struct {
	ID          int64  `json:"id"`
	UserID      int64  `json:"user_id"`
	RepoPattern string `json:"repo_pattern"`
	CreatedAt   int64  `json:"created_at"`
}

type PermissionListView struct {
	Items []PermissionView `json:"items"`
	Total int              `json:"total"`
}

// --- Handlers ---

// ListForUser returns every permission row for a given user (admin-only).
func (s *PermissionService) ListForUser(ctx *router.Context) error {
	// TODO(M3)
	return errNotImplemented()
}

// GrantBatch inserts the given patterns for a user.
// Admin users cannot be targets (rejected with 400).
func (s *PermissionService) GrantBatch(ctx *router.Context) error {
	var req GrantPermissionsRequest
	if err := ctx.Bind(&req); err != nil {
		return err
	}
	// TODO(M3): split patterns → bulk insert; ignore duplicates.
	return errNotImplemented()
}

// Update changes the pattern of an existing row (admin-only).
func (s *PermissionService) Update(ctx *router.Context) error {
	var req UpdatePermissionRequest
	if err := ctx.Bind(&req); err != nil {
		return err
	}
	// TODO(M3)
	return errNotImplemented()
}

// Revoke removes a single permission row (admin-only).
func (s *PermissionService) Revoke(ctx *router.Context) error {
	// TODO(M3)
	return errNotImplemented()
}
