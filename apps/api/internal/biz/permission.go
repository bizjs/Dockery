package biz

import (
	"context"
	"errors"
	"fmt"

	"api/internal/pkg/scope"
)

// Permission is the biz-layer representation of a repo_permissions row.
type Permission struct {
	ID          int
	UserID      int
	RepoPattern string
	CreatedAt   int64
}

// PermissionRepo is the data-layer contract for repo_permissions.
type PermissionRepo interface {
	ListForUser(ctx context.Context, userID int) ([]*Permission, error)
	ListPatternsForUser(ctx context.Context, userID int) ([]string, error)
	CreateOne(ctx context.Context, userID int, pattern string) (*Permission, error)
	DeleteByID(ctx context.Context, id int) error
	UpdatePattern(ctx context.Context, id int, pattern string) error
}

var (
	ErrPermissionNotFound = errors.New("permission: not found")
	ErrDuplicate          = errors.New("permission: (user, pattern) already exists")
	ErrAdminNeedsNoPerm   = errors.New("permission: admin users do not use repo_permissions")
)

// PermissionUsecase owns repo_permissions CRUD and the scope-matching
// decision that backs every /token issuance.
type PermissionUsecase struct {
	permRepo PermissionRepo
	userRepo UserRepo
}

func NewPermissionUsecase(permRepo PermissionRepo, userRepo UserRepo) *PermissionUsecase {
	return &PermissionUsecase{permRepo: permRepo, userRepo: userRepo}
}

// GrantPatterns adds multiple patterns for a user in one call. Duplicates
// (same user + pattern) are silently skipped so the operation is
// idempotent from the caller's perspective. Returns the rows that were
// actually inserted.
//
// Granting patterns to an admin is rejected — admins bypass this table.
func (u *PermissionUsecase) GrantPatterns(ctx context.Context, userID int, patterns []string) ([]*Permission, error) {
	target, err := u.userRepo.GetByID(ctx, userID)
	if err != nil {
		return nil, err
	}
	if target.Role == "admin" {
		return nil, ErrAdminNeedsNoPerm
	}
	inserted := make([]*Permission, 0, len(patterns))
	for _, p := range dedupe(patterns) {
		p = trimSpace(p)
		if p == "" {
			continue
		}
		row, err := u.permRepo.CreateOne(ctx, userID, p)
		if err != nil {
			if errors.Is(err, ErrDuplicate) {
				continue
			}
			return inserted, fmt.Errorf("grant %q: %w", p, err)
		}
		inserted = append(inserted, row)
	}
	return inserted, nil
}

func (u *PermissionUsecase) ListForUser(ctx context.Context, userID int) ([]*Permission, error) {
	return u.permRepo.ListForUser(ctx, userID)
}

func (u *PermissionUsecase) Revoke(ctx context.Context, permissionID int) error {
	return u.permRepo.DeleteByID(ctx, permissionID)
}

func (u *PermissionUsecase) UpdatePattern(ctx context.Context, permissionID int, pattern string) error {
	return u.permRepo.UpdatePattern(ctx, permissionID, pattern)
}

// ResolveAccess is the hot path taken by /token for every request:
// load the user's patterns once, then intersect each requested scope
// with the role's permitted actions. Admin users short-circuit the DB
// lookup entirely.
func (u *PermissionUsecase) ResolveAccess(ctx context.Context, user *User, requested []scope.Scope) ([]scope.Scope, error) {
	var patterns []string
	if user.Role != "admin" {
		var err error
		patterns, err = u.permRepo.ListPatternsForUser(ctx, user.ID)
		if err != nil {
			return nil, err
		}
	}
	out := make([]scope.Scope, 0, len(requested))
	for _, r := range requested {
		granted := scope.Match(scope.Role(user.Role), patterns, r)
		out = append(out, scope.Scope{Type: r.Type, Name: r.Name, Actions: granted})
	}
	return out, nil
}

func dedupe(xs []string) []string {
	seen := make(map[string]struct{}, len(xs))
	out := make([]string, 0, len(xs))
	for _, x := range xs {
		if _, ok := seen[x]; ok {
			continue
		}
		seen[x] = struct{}{}
		out = append(out, x)
	}
	return out
}

func trimSpace(s string) string {
	for len(s) > 0 && (s[0] == ' ' || s[0] == '\t') {
		s = s[1:]
	}
	for len(s) > 0 && (s[len(s)-1] == ' ' || s[len(s)-1] == '\t') {
		s = s[:len(s)-1]
	}
	return s
}
