package data

import (
	"context"
	"fmt"

	"api/internal/biz"
	"api/internal/data/ent"
	"api/internal/data/ent/repopermission"
	"api/internal/data/ent/user"

	"github.com/go-kratos/kratos/v2/log"
)

type permissionRepo struct {
	data *Data
	log  *log.Helper
}

func NewPermissionRepo(d *Data, logger log.Logger) biz.PermissionRepo {
	return &permissionRepo{data: d, log: log.NewHelper(log.With(logger, "module", "data/permission"))}
}

// ListPatternsForUser returns just the repo_pattern strings — biz.scope
// matcher only needs those.
func (r *permissionRepo) ListPatternsForUser(ctx context.Context, userID int) ([]string, error) {
	rows, err := r.data.DB().RepoPermission.Query().
		Where(repopermission.HasUserWith(user.IDEQ(userID))).
		All(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(rows))
	for _, p := range rows {
		out = append(out, p.RepoPattern)
	}
	return out, nil
}

// ListForUser returns full permission rows for admin UI.
func (r *permissionRepo) ListForUser(ctx context.Context, userID int) ([]*biz.Permission, error) {
	rows, err := r.data.DB().RepoPermission.Query().
		Where(repopermission.HasUserWith(user.IDEQ(userID))).
		Order(ent.Asc(repopermission.FieldID)).
		All(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]*biz.Permission, 0, len(rows))
	for _, p := range rows {
		out = append(out, &biz.Permission{
			ID:          p.ID,
			UserID:      userID,
			RepoPattern: p.RepoPattern,
			CreatedAt:   p.CreatedAt.Unix(),
		})
	}
	return out, nil
}

// CreateOne inserts a single (user_id, repo_pattern) row. Duplicates
// collide with the unique index and are surfaced as biz.ErrDuplicate.
func (r *permissionRepo) CreateOne(ctx context.Context, userID int, pattern string) (*biz.Permission, error) {
	p, err := r.data.DB().RepoPermission.Create().
		SetUserID(userID).
		SetRepoPattern(pattern).
		Save(ctx)
	if err != nil {
		if ent.IsConstraintError(err) {
			return nil, biz.ErrDuplicate
		}
		return nil, fmt.Errorf("permission create: %w", err)
	}
	return &biz.Permission{
		ID:          p.ID,
		UserID:      userID,
		RepoPattern: p.RepoPattern,
		CreatedAt:   p.CreatedAt.Unix(),
	}, nil
}

func (r *permissionRepo) DeleteByID(ctx context.Context, id int) error {
	if err := r.data.DB().RepoPermission.DeleteOneID(id).Exec(ctx); err != nil {
		if ent.IsNotFound(err) {
			return biz.ErrPermissionNotFound
		}
		return err
	}
	return nil
}

func (r *permissionRepo) UpdatePattern(ctx context.Context, id int, pattern string) error {
	err := r.data.DB().RepoPermission.UpdateOneID(id).SetRepoPattern(pattern).Exec(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return biz.ErrPermissionNotFound
		}
		if ent.IsConstraintError(err) {
			return biz.ErrDuplicate
		}
		return err
	}
	return nil
}

var _ biz.PermissionRepo = (*permissionRepo)(nil)
