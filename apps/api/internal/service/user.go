package service

import (
	"api/internal/data"

	"github.com/bizjs/kratoscarf/router"
)

// UserService owns account CRUD for the Dockery identity system.
// Role is one of admin | write | view; actions on repositories are
// entirely driven by the role, not stored per-row.
type UserService struct {
	data *data.Data
}

func NewUserService(d *data.Data) *UserService { return &UserService{data: d} }

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

// SetPasswordRequest supports both self-service ("old_password" required)
// and admin-reset ("old_password" omitted). The handler decides which
// rule to enforce based on whether caller.ID == target.ID.
type SetPasswordRequest struct {
	OldPassword string `json:"old_password,omitempty"`
	NewPassword string `json:"new_password" validate:"required,min=8,max=256"`
}

type UserView struct {
	ID        int64  `json:"id"`
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

// --- Handlers ---

// List returns every user (admin-only).
func (s *UserService) List(ctx *router.Context) error {
	// TODO(M3): ent query with pagination.
	return errNotImplemented()
}

// Create provisions a new user account (admin-only).
func (s *UserService) Create(ctx *router.Context) error {
	var req CreateUserRequest
	if err := ctx.Bind(&req); err != nil {
		return err
	}
	// TODO(M2): bcrypt hash → users.Create
	return errNotImplemented()
}

// Get returns a single user by id (admin-only).
func (s *UserService) Get(ctx *router.Context) error {
	// TODO(M3)
	return errNotImplemented()
}

// Update mutates role / disabled flag (admin-only).
func (s *UserService) Update(ctx *router.Context) error {
	var req UpdateUserRequest
	if err := ctx.Bind(&req); err != nil {
		return err
	}
	// TODO(M3)
	return errNotImplemented()
}

// Delete removes a user and cascades their repo_permissions rows.
// (admin-only; cannot delete the last admin — enforced in biz layer.)
func (s *UserService) Delete(ctx *router.Context) error {
	// TODO(M3)
	return errNotImplemented()
}

// SetPassword changes the password for a user.
// Session required. If caller.ID == target.ID, old_password must match.
// If caller.Role == admin, old_password is optional.
func (s *UserService) SetPassword(ctx *router.Context) error {
	var req SetPasswordRequest
	if err := ctx.Bind(&req); err != nil {
		return err
	}
	// TODO(M3)
	return errNotImplemented()
}
