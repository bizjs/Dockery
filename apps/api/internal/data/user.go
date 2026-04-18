package data

import (
	"context"
	"errors"
	"fmt"

	"api/internal/biz"
	"api/internal/data/ent"
	"api/internal/data/ent/user"

	"github.com/go-kratos/kratos/v2/log"
)

type userRepo struct {
	data *Data
	log  *log.Helper
}

// NewUserRepo adapts ent's generated User client to the biz.UserRepo
// interface so biz/ never imports ent directly.
func NewUserRepo(d *Data, logger log.Logger) biz.UserRepo {
	return &userRepo{data: d, log: log.NewHelper(log.With(logger, "module", "data/user"))}
}

func (r *userRepo) Create(ctx context.Context, username, passwordHash, role string) (*biz.User, error) {
	u, err := r.data.DB().User.Create().
		SetUsername(username).
		SetPasswordHash(passwordHash).
		SetRole(user.Role(role)).
		Save(ctx)
	if err != nil {
		return nil, fmt.Errorf("user create: %w", err)
	}
	return toBizUser(u), nil
}

func (r *userRepo) GetByUsername(ctx context.Context, username string) (*biz.User, error) {
	u, err := r.data.DB().User.Query().Where(user.UsernameEQ(username)).Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, biz.ErrUserNotFound
		}
		return nil, fmt.Errorf("user query: %w", err)
	}
	return toBizUser(u), nil
}

func (r *userRepo) GetByID(ctx context.Context, id int) (*biz.User, error) {
	u, err := r.data.DB().User.Get(ctx, id)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, biz.ErrUserNotFound
		}
		return nil, err
	}
	return toBizUser(u), nil
}

func (r *userRepo) List(ctx context.Context) ([]*biz.User, error) {
	us, err := r.data.DB().User.Query().Order(ent.Asc(user.FieldID)).All(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]*biz.User, 0, len(us))
	for _, u := range us {
		out = append(out, toBizUser(u))
	}
	return out, nil
}

func (r *userRepo) Count(ctx context.Context) (int, error) {
	return r.data.DB().User.Query().Count(ctx)
}

func (r *userRepo) SetPassword(ctx context.Context, id int, passwordHash string) error {
	return r.data.DB().User.UpdateOneID(id).SetPasswordHash(passwordHash).Exec(ctx)
}

func (r *userRepo) SetRole(ctx context.Context, id int, role string) error {
	return r.data.DB().User.UpdateOneID(id).SetRole(user.Role(role)).Exec(ctx)
}

func (r *userRepo) SetDisabled(ctx context.Context, id int, disabled bool) error {
	return r.data.DB().User.UpdateOneID(id).SetDisabled(disabled).Exec(ctx)
}

func (r *userRepo) Delete(ctx context.Context, id int) error {
	if err := r.data.DB().User.DeleteOneID(id).Exec(ctx); err != nil {
		if ent.IsNotFound(err) {
			return biz.ErrUserNotFound
		}
		return err
	}
	return nil
}

func toBizUser(u *ent.User) *biz.User {
	if u == nil {
		return nil
	}
	return &biz.User{
		ID:           u.ID,
		Username:     u.Username,
		PasswordHash: u.PasswordHash,
		Role:         string(u.Role),
		Disabled:     u.Disabled,
		CreatedAt:    u.CreatedAt.Unix(),
		UpdatedAt:    u.UpdatedAt.Unix(),
	}
}

// Compile-time check that *userRepo implements biz.UserRepo.
var _ biz.UserRepo = (*userRepo)(nil)

// errUnused silences the unused import warning for errors during
// partial edits; remove if not needed.
var _ = errors.New
