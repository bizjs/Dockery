package service

import (
	"errors"
	"strconv"

	"api/internal/biz"

	"github.com/bizjs/kratoscarf/response"
	"github.com/bizjs/kratoscarf/router"
)

// PermissionService manages repo_permissions rows for write/view users.
// Admins bypass this table entirely; attempting to grant a permission
// to an admin is an error.
type PermissionService struct {
	perms *biz.PermissionUsecase
	users *biz.UserUsecase
	audit *biz.AuditUsecase
}

func NewPermissionService(perms *biz.PermissionUsecase, users *biz.UserUsecase, audit *biz.AuditUsecase) *PermissionService {
	return &PermissionService{perms: perms, users: users, audit: audit}
}

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
	ID          int    `json:"id"`
	UserID      int    `json:"user_id"`
	RepoPattern string `json:"repo_pattern"`
	CreatedAt   int64  `json:"created_at"`
}

type PermissionListView struct {
	Items []PermissionView `json:"items"`
	Total int              `json:"total"`
}

func toPermissionView(p *biz.Permission) PermissionView {
	return PermissionView{
		ID:          p.ID,
		UserID:      p.UserID,
		RepoPattern: p.RepoPattern,
		CreatedAt:   p.CreatedAt,
	}
}

// --- Handlers ---

// ListForUser returns every permission row for a given user (admin-only
// at the route level). Non-existent user → 404.
func (s *PermissionService) ListForUser(ctx *router.Context) error {
	userID, err := userIDFromPath(ctx)
	if err != nil {
		return err
	}
	if _, err := s.users.GetByID(ctx.Context(), userID); err != nil {
		if errors.Is(err, biz.ErrUserNotFound) {
			return response.ErrNotFound
		}
		return response.ErrInternal.WithCause(err)
	}
	rows, err := s.perms.ListForUser(ctx.Context(), userID)
	if err != nil {
		return response.ErrInternal.WithCause(err)
	}
	items := make([]PermissionView, 0, len(rows))
	for _, p := range rows {
		items = append(items, toPermissionView(p))
	}
	return ctx.Success(PermissionListView{Items: items, Total: len(items)})
}

// GrantBatch inserts the given patterns for a user. Duplicates on the
// unique (user_id, repo_pattern) index are silently skipped; the
// response reports only freshly-inserted rows. Targeting an admin
// user is a 400 — admins don't use this table.
func (s *PermissionService) GrantBatch(ctx *router.Context) error {
	userID, err := userIDFromPath(ctx)
	if err != nil {
		return err
	}
	var req GrantPermissionsRequest
	if err := ctx.Bind(&req); err != nil {
		return err
	}
	rows, err := s.perms.GrantPatterns(ctx.Context(), userID, req.RepoPatterns)
	if err != nil {
		switch {
		case errors.Is(err, biz.ErrUserNotFound):
			return response.ErrNotFound
		case errors.Is(err, biz.ErrAdminNeedsNoPerm):
			return response.ErrBadRequest.WithMessage("admin users do not use repo permissions")
		}
		return response.ErrInternal.WithCause(err)
	}
	items := make([]PermissionView, 0, len(rows))
	patterns := make([]string, 0, len(rows))
	for _, p := range rows {
		items = append(items, toPermissionView(p))
		patterns = append(patterns, p.RepoPattern)
	}
	// Audit only the rows that were actually inserted (duplicates were
	// silently dropped; not worth auditing non-events).
	if len(rows) > 0 {
		target, _ := s.users.GetByID(ctx.Context(), userID)
		targetName := ""
		if target != nil {
			targetName = target.Username
		}
		s.audit.Write(ctx.Context(), biz.AuditEntry{
			Actor:    sessionUsername(ctx),
			Action:   biz.ActionPermissionGranted,
			Target:   "user:" + targetName,
			ClientIP: ctx.ClientIP(),
			Success:  true,
			Detail:   map[string]any{"patterns": patterns},
		})
	}
	return ctx.Success(PermissionListView{Items: items, Total: len(items)})
}

// Update changes the pattern of an existing row.
func (s *PermissionService) Update(ctx *router.Context) error {
	id, err := permissionIDFromPath(ctx)
	if err != nil {
		return err
	}
	var req UpdatePermissionRequest
	if err := ctx.Bind(&req); err != nil {
		return err
	}
	if err := s.perms.UpdatePattern(ctx.Context(), id, req.RepoPattern); err != nil {
		switch {
		case errors.Is(err, biz.ErrPermissionNotFound):
			return response.ErrNotFound
		case errors.Is(err, biz.ErrDuplicate):
			return response.ErrConflict.WithMessage("pattern already exists for this user")
		}
		return response.ErrInternal.WithCause(err)
	}
	s.audit.Write(ctx.Context(), biz.AuditEntry{
		Actor:    sessionUsername(ctx),
		Action:   biz.ActionPermissionUpdated,
		Target:   "permission:" + strconv.Itoa(id),
		ClientIP: ctx.ClientIP(),
		Success:  true,
		Detail:   map[string]any{"pattern": req.RepoPattern},
	})
	return ctx.Success(nil)
}

// Revoke removes a single permission row.
func (s *PermissionService) Revoke(ctx *router.Context) error {
	id, err := permissionIDFromPath(ctx)
	if err != nil {
		return err
	}
	if err := s.perms.Revoke(ctx.Context(), id); err != nil {
		if errors.Is(err, biz.ErrPermissionNotFound) {
			return response.ErrNotFound
		}
		return response.ErrInternal.WithCause(err)
	}
	s.audit.Write(ctx.Context(), biz.AuditEntry{
		Actor:    sessionUsername(ctx),
		Action:   biz.ActionPermissionRevoked,
		Target:   "permission:" + strconv.Itoa(id),
		ClientIP: ctx.ClientIP(),
		Success:  true,
	})
	return ctx.Success(nil)
}

// permissionIDFromPath parses the {id} path param as a positive int.
func permissionIDFromPath(ctx *router.Context) (int, error) {
	raw := ctx.Param("id")
	id, err := strconv.Atoi(raw)
	if err != nil || id <= 0 {
		return 0, response.ErrBadRequest.WithMessage("invalid permission id")
	}
	return id, nil
}
