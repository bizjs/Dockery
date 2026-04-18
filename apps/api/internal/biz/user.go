package biz

import (
	"context"
	"errors"
	"fmt"

	"golang.org/x/crypto/bcrypt"
)

// User is the biz-layer representation of a Dockery account.
// Fields match the ent schema (internal/data/ent/schema/user.go) but
// biz never imports ent directly — data/ is the adapter.
type User struct {
	ID           int
	Username     string
	PasswordHash string
	Role         string // "admin" | "write" | "view"
	Disabled     bool
	CreatedAt    int64
	UpdatedAt    int64
}

// UserRepo is the data-layer contract UserUsecase depends on.
// Implemented in internal/data/user.go.
type UserRepo interface {
	Create(ctx context.Context, username, passwordHash, role string) (*User, error)
	GetByUsername(ctx context.Context, username string) (*User, error)
	GetByID(ctx context.Context, id int) (*User, error)
	List(ctx context.Context) ([]*User, error)
	Count(ctx context.Context) (int, error)
	SetPassword(ctx context.Context, id int, passwordHash string) error
	SetRole(ctx context.Context, id int, role string) error
	SetDisabled(ctx context.Context, id int, disabled bool) error
	Delete(ctx context.Context, id int) error
}

// Sentinel errors.
var (
	ErrUserNotFound       = errors.New("user: not found")
	ErrInvalidCredentials = errors.New("user: invalid credentials")
	ErrAdminPasswordUnset = errors.New("user: admin password required on first launch (set DOCKERY_ADMIN_PASSWORD env or dockery.admin.password in config)")
	ErrWeakPassword       = errors.New("user: password must be at least 8 characters")
	ErrInvalidRole        = errors.New("user: role must be one of admin/write/view")
)

// UserUsecase owns password hashing, credential verification, and the
// first-boot admin bootstrap.
type UserUsecase struct {
	repo UserRepo
}

func NewUserUsecase(repo UserRepo) *UserUsecase {
	return &UserUsecase{repo: repo}
}

// EnsureAdmin creates the first admin account if the users table is
// empty. Called exactly once on each container start, before the HTTP
// server accepts traffic. A no-op on subsequent boots.
func (u *UserUsecase) EnsureAdmin(ctx context.Context, username, password string) error {
	if username == "" {
		username = "admin"
	}
	n, err := u.repo.Count(ctx)
	if err != nil {
		return fmt.Errorf("ensure admin: count: %w", err)
	}
	if n > 0 {
		return nil
	}
	if password == "" {
		return ErrAdminPasswordUnset
	}
	if len(password) < 8 {
		return ErrWeakPassword
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("bcrypt: %w", err)
	}
	if _, err := u.repo.Create(ctx, username, string(hash), "admin"); err != nil {
		return fmt.Errorf("ensure admin: create: %w", err)
	}
	return nil
}

// Create provisions a new user; hashes password via bcrypt.
func (u *UserUsecase) Create(ctx context.Context, username, password, role string) (*User, error) {
	if !isValidRole(role) {
		return nil, ErrInvalidRole
	}
	if len(password) < 8 {
		return nil, ErrWeakPassword
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}
	return u.repo.Create(ctx, username, string(hash), role)
}

// VerifyCredentials checks username+password via bcrypt. Returns the
// user on success, ErrInvalidCredentials on any failure (wrong user,
// wrong password, or disabled account) — the caller cannot distinguish
// these cases, foiling username enumeration.
func (u *UserUsecase) VerifyCredentials(ctx context.Context, username, password string) (*User, error) {
	user, err := u.repo.GetByUsername(ctx, username)
	if err != nil {
		// Run bcrypt against a dummy hash to keep timing flat; otherwise
		// missing-user would respond noticeably faster than wrong-pass.
		_ = bcrypt.CompareHashAndPassword([]byte(dummyHash), []byte(password))
		return nil, ErrInvalidCredentials
	}
	if user.Disabled {
		return nil, ErrInvalidCredentials
	}
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)); err != nil {
		return nil, ErrInvalidCredentials
	}
	return user, nil
}

// GetByID / List / Count are pass-throughs used by CLI and admin endpoints.
func (u *UserUsecase) GetByID(ctx context.Context, id int) (*User, error) {
	return u.repo.GetByID(ctx, id)
}

func (u *UserUsecase) GetByUsername(ctx context.Context, username string) (*User, error) {
	return u.repo.GetByUsername(ctx, username)
}

func (u *UserUsecase) List(ctx context.Context) ([]*User, error) {
	return u.repo.List(ctx)
}

// SetPassword replaces a user's password (admin-reset flow — no old
// password check. Self-service password change is handled at the
// service layer with an extra VerifyCredentials step).
func (u *UserUsecase) SetPassword(ctx context.Context, id int, newPassword string) error {
	if len(newPassword) < 8 {
		return ErrWeakPassword
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	return u.repo.SetPassword(ctx, id, string(hash))
}

func (u *UserUsecase) SetRole(ctx context.Context, id int, role string) error {
	if !isValidRole(role) {
		return ErrInvalidRole
	}
	return u.repo.SetRole(ctx, id, role)
}

func (u *UserUsecase) SetDisabled(ctx context.Context, id int, disabled bool) error {
	return u.repo.SetDisabled(ctx, id, disabled)
}

func (u *UserUsecase) Delete(ctx context.Context, id int) error {
	return u.repo.Delete(ctx, id)
}

func isValidRole(r string) bool {
	switch r {
	case "admin", "write", "view":
		return true
	}
	return false
}

// dummyHash is a valid bcrypt hash used only to equalise timing when
// the target username does not exist. The underlying plaintext ("") is
// not relevant because CompareHashAndPassword runs the full cost
// regardless; what matters is that the hash parses and triggers real
// bcrypt work.
const dummyHash = "$2a$10$CwTycUXWue0Thq9StjUM0uJ8yB8KQJLOmfoAZM7iOk9oQlNVcCvqO"
