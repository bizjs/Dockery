package service

import (
	"errors"
	"net/http"
	"strconv"

	"api/internal/biz"

	"github.com/bizjs/kratoscarf/response"
	"github.com/bizjs/kratoscarf/router"
)

// UserService owns account CRUD for the Dockery identity system.
// Role is one of admin | write | view; actions on repositories are
// entirely driven by the role, not stored per-row.
type UserService struct {
	users *biz.UserUsecase
	perms *biz.PermissionUsecase
}

func NewUserService(users *biz.UserUsecase, perms *biz.PermissionUsecase) *UserService {
	return &UserService{users: users, perms: perms}
}

// --- DTOs ---

type CreateUserRequest struct {
	Username string `json:"username" validate:"required,min=1,max=64"`
	Password string `json:"password" validate:"required,min=8,max=256"`
	Role     string `json:"role" validate:"required,oneof=admin write view"`
}

type UpdateUserRequest struct {
	Role     *string `json:"role,omitempty" validate:"omitempty,oneof=admin write view"`
	Disabled *bool   `json:"disabled,omitempty"`
}

type SetPasswordRequest struct {
	OldPassword string `json:"old_password,omitempty"`
	NewPassword string `json:"new_password" validate:"required,min=8,max=256"`
}

type UserView struct {
	ID        int    `json:"id"`
	Username  string `json:"username"`
	Role      string `json:"role"`
	Disabled  bool   `json:"disabled"`
	CreatedAt int64  `json:"created_at"`
	UpdatedAt int64  `json:"updated_at"`
}

type UserListView struct {
	Items []UserView `json:"items"`
	Total int        `json:"total"`
}

func toUserView(u *biz.User) UserView {
	return UserView{
		ID: u.ID, Username: u.Username, Role: u.Role, Disabled: u.Disabled,
		CreatedAt: u.CreatedAt, UpdatedAt: u.UpdatedAt,
	}
}

// --- Handlers ---

// List — admin-only; middleware enforces. Returns every account.
func (s *UserService) List(ctx *router.Context) error {
	users, err := s.users.List(ctx.Context())
	if err != nil {
		return response.ErrInternal.WithCause(err)
	}
	items := make([]UserView, 0, len(users))
	for _, u := range users {
		items = append(items, toUserView(u))
	}
	return ctx.Success(UserListView{Items: items, Total: len(items)})
}

// Create — admin-only. Username uniqueness is enforced by the DB;
// we surface it as a 409 Conflict.
func (s *UserService) Create(ctx *router.Context) error {
	var req CreateUserRequest
	if err := ctx.Bind(&req); err != nil {
		return err
	}
	u, err := s.users.Create(ctx.Context(), req.Username, req.Password, req.Role)
	if err != nil {
		switch {
		case errors.Is(err, biz.ErrWeakPassword):
			return response.ErrBadRequest.WithMessage("password too weak")
		case errors.Is(err, biz.ErrInvalidRole):
			return response.ErrBadRequest.WithMessage("invalid role")
		}
		// Likely unique constraint violation.
		return response.ErrConflict.WithMessage(err.Error())
	}
	return ctx.Success(toUserView(u))
}

// Get — admin-only.
func (s *UserService) Get(ctx *router.Context) error {
	id, err := userIDFromPath(ctx)
	if err != nil {
		return err
	}
	u, err := s.users.GetByID(ctx.Context(), id)
	if err != nil {
		if errors.Is(err, biz.ErrUserNotFound) {
			return response.ErrNotFound
		}
		return response.ErrInternal.WithCause(err)
	}
	return ctx.Success(toUserView(u))
}

// Update — admin-only. Mutates role and/or disabled flag.
func (s *UserService) Update(ctx *router.Context) error {
	id, err := userIDFromPath(ctx)
	if err != nil {
		return err
	}
	var req UpdateUserRequest
	if err := ctx.Bind(&req); err != nil {
		return err
	}
	if req.Role != nil {
		if err := s.users.SetRole(ctx.Context(), id, *req.Role); err != nil {
			return response.ErrBadRequest.WithMessage(err.Error())
		}
	}
	if req.Disabled != nil {
		if err := s.users.SetDisabled(ctx.Context(), id, *req.Disabled); err != nil {
			return response.ErrInternal.WithCause(err)
		}
	}
	u, err := s.users.GetByID(ctx.Context(), id)
	if err != nil {
		return response.ErrInternal.WithCause(err)
	}
	return ctx.Success(toUserView(u))
}

// Delete — admin-only. Cascades repo_permissions (ent edge).
// TODO(M3): refuse to delete the last admin so the system can't lock itself out.
func (s *UserService) Delete(ctx *router.Context) error {
	id, err := userIDFromPath(ctx)
	if err != nil {
		return err
	}
	// Prevent admins from deleting themselves — avoids the common
	// footgun of losing the only admin account via a double-click.
	if id == sessionUserID(ctx) {
		return response.ErrBadRequest.WithMessage("cannot delete your own account")
	}
	if err := s.users.Delete(ctx.Context(), id); err != nil {
		if errors.Is(err, biz.ErrUserNotFound) {
			return response.ErrNotFound
		}
		return response.ErrInternal.WithCause(err)
	}
	return ctx.Success(nil)
}

// SetPassword — session required (not admin-only). Enforces
// "target == caller OR caller.role == admin". Self-service additionally
// requires old_password to match.
func (s *UserService) SetPassword(ctx *router.Context) error {
	targetID, err := userIDFromPath(ctx)
	if err != nil {
		return err
	}
	var req SetPasswordRequest
	if err := ctx.Bind(&req); err != nil {
		return err
	}

	callerID := sessionUserID(ctx)
	isAdmin := sessionRole(ctx) == "admin"
	if targetID != callerID && !isAdmin {
		return response.ErrForbidden
	}

	// Self-service must prove the old password.
	if targetID == callerID && !isAdmin {
		if req.OldPassword == "" {
			return response.ErrBadRequest.WithMessage("old_password required for self-service change")
		}
		caller, err := s.users.GetByID(ctx.Context(), callerID)
		if err != nil {
			return response.ErrInternal.WithCause(err)
		}
		if _, err := s.users.VerifyCredentials(ctx.Context(), caller.Username, req.OldPassword); err != nil {
			return response.NewBizError(http.StatusUnauthorized, 40102, "old password incorrect")
		}
	}

	if err := s.users.SetPassword(ctx.Context(), targetID, req.NewPassword); err != nil {
		if errors.Is(err, biz.ErrWeakPassword) {
			return response.ErrBadRequest.WithMessage("password too weak")
		}
		return response.ErrInternal.WithCause(err)
	}
	return ctx.Success(nil)
}

// userIDFromPath parses {id} as a positive integer.
func userIDFromPath(ctx *router.Context) (int, error) {
	raw := ctx.Param("id")
	id, err := strconv.Atoi(raw)
	if err != nil || id <= 0 {
		return 0, response.ErrBadRequest.WithMessage("invalid user id")
	}
	return id, nil
}
