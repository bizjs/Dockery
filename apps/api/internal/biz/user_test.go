package biz_test

import (
	"context"
	"testing"

	"api/internal/biz"
)

// fakeUserRepo is a minimal in-memory UserRepo for testing biz logic
// without spinning up ent/SQLite.
type fakeUserRepo struct {
	users []*biz.User
	next  int
}

func (r *fakeUserRepo) Create(_ context.Context, username, hash, role string) (*biz.User, error) {
	r.next++
	u := &biz.User{ID: r.next, Username: username, PasswordHash: hash, Role: role}
	r.users = append(r.users, u)
	return u, nil
}
func (r *fakeUserRepo) GetByUsername(_ context.Context, name string) (*biz.User, error) {
	for _, u := range r.users {
		if u.Username == name {
			return u, nil
		}
	}
	return nil, biz.ErrUserNotFound
}
func (r *fakeUserRepo) GetByID(_ context.Context, id int) (*biz.User, error) {
	for _, u := range r.users {
		if u.ID == id {
			return u, nil
		}
	}
	return nil, biz.ErrUserNotFound
}
func (r *fakeUserRepo) List(_ context.Context) ([]*biz.User, error) { return r.users, nil }
func (r *fakeUserRepo) Count(_ context.Context) (int, error)        { return len(r.users), nil }
func (r *fakeUserRepo) CountByRole(_ context.Context, role string) (int, error) {
	n := 0
	for _, u := range r.users {
		if u.Role == role {
			n++
		}
	}
	return n, nil
}
func (r *fakeUserRepo) SetPassword(_ context.Context, id int, hash string) error {
	for _, u := range r.users {
		if u.ID == id {
			u.PasswordHash = hash
			return nil
		}
	}
	return biz.ErrUserNotFound
}
func (r *fakeUserRepo) SetRole(_ context.Context, id int, role string) error {
	for _, u := range r.users {
		if u.ID == id {
			u.Role = role
			return nil
		}
	}
	return biz.ErrUserNotFound
}
func (r *fakeUserRepo) SetDisabled(_ context.Context, id int, d bool) error {
	for _, u := range r.users {
		if u.ID == id {
			u.Disabled = d
			return nil
		}
	}
	return biz.ErrUserNotFound
}
func (r *fakeUserRepo) Delete(_ context.Context, id int) error {
	for i, u := range r.users {
		if u.ID == id {
			r.users = append(r.users[:i], r.users[i+1:]...)
			return nil
		}
	}
	return biz.ErrUserNotFound
}

func TestEnsureAdmin_CreatesOnce(t *testing.T) {
	repo := &fakeUserRepo{}
	u := biz.NewUserUsecase(repo)
	ctx := context.Background()

	if err := u.EnsureAdmin(ctx, "admin", "a-strong-password"); err != nil {
		t.Fatalf("first ensure: %v", err)
	}
	if n, _ := repo.Count(ctx); n != 1 {
		t.Fatalf("want 1 user, got %d", n)
	}
	// Second call is a no-op even if password changes.
	if err := u.EnsureAdmin(ctx, "admin", "something-else"); err != nil {
		t.Fatalf("second ensure: %v", err)
	}
	if n, _ := repo.Count(ctx); n != 1 {
		t.Fatalf("want still 1 user, got %d", n)
	}
}

func TestEnsureAdmin_RejectsEmptyPasswordOnEmptyDB(t *testing.T) {
	u := biz.NewUserUsecase(&fakeUserRepo{})
	err := u.EnsureAdmin(context.Background(), "admin", "")
	if err != biz.ErrAdminPasswordUnset {
		t.Fatalf("want ErrAdminPasswordUnset, got %v", err)
	}
}

func TestEnsureAdmin_RejectsWeakPassword(t *testing.T) {
	u := biz.NewUserUsecase(&fakeUserRepo{})
	err := u.EnsureAdmin(context.Background(), "admin", "short")
	if err != biz.ErrWeakPassword {
		t.Fatalf("want ErrWeakPassword, got %v", err)
	}
}

func TestVerifyCredentials_Success(t *testing.T) {
	repo := &fakeUserRepo{}
	u := biz.NewUserUsecase(repo)
	ctx := context.Background()

	if _, err := u.Create(ctx, "alice", "a-strong-password", "write"); err != nil {
		t.Fatalf("create: %v", err)
	}
	got, err := u.VerifyCredentials(ctx, "alice", "a-strong-password")
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if got.Username != "alice" || got.Role != "write" {
		t.Fatalf("got %+v", got)
	}
}

func TestVerifyCredentials_WrongPassword(t *testing.T) {
	repo := &fakeUserRepo{}
	u := biz.NewUserUsecase(repo)
	ctx := context.Background()
	_, _ = u.Create(ctx, "alice", "a-strong-password", "view")

	if _, err := u.VerifyCredentials(ctx, "alice", "nope"); err != biz.ErrInvalidCredentials {
		t.Fatalf("got %v", err)
	}
}

func TestVerifyCredentials_UnknownUserBehavesLikeWrongPassword(t *testing.T) {
	u := biz.NewUserUsecase(&fakeUserRepo{})
	// No user exists; must still return ErrInvalidCredentials (not "not found")
	// so callers cannot enumerate usernames.
	if _, err := u.VerifyCredentials(context.Background(), "ghost", "whatever"); err != biz.ErrInvalidCredentials {
		t.Fatalf("got %v", err)
	}
}

func TestVerifyCredentials_Disabled(t *testing.T) {
	repo := &fakeUserRepo{}
	u := biz.NewUserUsecase(repo)
	ctx := context.Background()
	user, _ := u.Create(ctx, "alice", "a-strong-password", "write")
	_ = u.SetDisabled(ctx, user.ID, true)

	if _, err := u.VerifyCredentials(ctx, "alice", "a-strong-password"); err != biz.ErrInvalidCredentials {
		t.Fatalf("disabled user should fail auth, got %v", err)
	}
}

func TestCreate_RejectsInvalidRole(t *testing.T) {
	u := biz.NewUserUsecase(&fakeUserRepo{})
	_, err := u.Create(context.Background(), "alice", "a-strong-password", "superuser")
	if err != biz.ErrInvalidRole {
		t.Fatalf("got %v", err)
	}
}

func TestSetRole_RefusesLastAdminDemotion(t *testing.T) {
	repo := &fakeUserRepo{}
	u := biz.NewUserUsecase(repo)
	ctx := context.Background()
	admin, _ := u.Create(ctx, "admin", "a-strong-password", "admin")

	if err := u.SetRole(ctx, admin.ID, "write"); err != biz.ErrLastAdmin {
		t.Fatalf("want ErrLastAdmin, got %v", err)
	}
	if admin.Role != "admin" {
		t.Fatalf("role changed despite refusal: %s", admin.Role)
	}
}

func TestSetRole_AllowsDemotionWhenOtherAdminExists(t *testing.T) {
	repo := &fakeUserRepo{}
	u := biz.NewUserUsecase(repo)
	ctx := context.Background()
	a1, _ := u.Create(ctx, "a1", "a-strong-password", "admin")
	_, _ = u.Create(ctx, "a2", "a-strong-password", "admin")

	if err := u.SetRole(ctx, a1.ID, "write"); err != nil {
		t.Fatalf("demote with >1 admin should succeed, got %v", err)
	}
	if a1.Role != "write" {
		t.Fatalf("role not updated: %s", a1.Role)
	}
}

func TestDelete_RefusesLastAdmin(t *testing.T) {
	repo := &fakeUserRepo{}
	u := biz.NewUserUsecase(repo)
	ctx := context.Background()
	admin, _ := u.Create(ctx, "admin", "a-strong-password", "admin")

	if err := u.Delete(ctx, admin.ID); err != biz.ErrLastAdmin {
		t.Fatalf("want ErrLastAdmin, got %v", err)
	}
	if n, _ := repo.Count(ctx); n != 1 {
		t.Fatalf("user was deleted despite refusal: count=%d", n)
	}
}

func TestDelete_AllowsNonAdmin(t *testing.T) {
	repo := &fakeUserRepo{}
	u := biz.NewUserUsecase(repo)
	ctx := context.Background()
	_, _ = u.Create(ctx, "admin", "a-strong-password", "admin")
	bob, _ := u.Create(ctx, "bob", "a-strong-password", "write")

	if err := u.Delete(ctx, bob.ID); err != nil {
		t.Fatalf("delete non-admin should succeed, got %v", err)
	}
}
