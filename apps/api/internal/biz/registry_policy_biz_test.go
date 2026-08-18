package biz

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"api/internal/data/ent"
	"api/internal/data/ent/systemsetting"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"

	_ "modernc.org/sqlite"
)

func newPolicyTestClient(t *testing.T) *ent.Client {
	t.Helper()
	dsn := "file:" + filepath.Join(t.TempDir(), "policy.db") + "?_pragma=foreign_keys(1)"
	stdDB, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	client := ent.NewClient(ent.Driver(entsql.OpenDB(dialect.SQLite, stdDB)))
	if err := client.Schema.Create(context.Background()); err != nil {
		t.Fatalf("create schema: %v", err)
	}
	t.Cleanup(func() { _ = client.Close() })
	return client
}

func TestRegistryPolicyInitializeAndUpdate(t *testing.T) {
	client := newPolicyTestClient(t)
	uc := NewRegistryPolicyBiz(client)
	if err := uc.Initialize(context.Background()); err != nil {
		t.Fatalf("initialize: %v", err)
	}
	initial, err := uc.Get()
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if initial.PreventTagOverwrite || initial.Version != 0 || initial.UpdatedBy != "system" {
		t.Fatalf("unexpected default policy: %+v", initial)
	}
	if len(initial.OverwriteExclusions) != 0 {
		t.Fatalf("default policy exclusions = %v, want empty", initial.OverwriteExclusions)
	}

	updated, err := uc.Update(context.Background(), 0, true, []string{"latest"}, "admin")
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if !updated.PreventTagOverwrite || !slices.Equal(updated.OverwriteExclusions, []string{"latest"}) ||
		updated.Version != 1 || updated.UpdatedBy != "admin" {
		t.Fatalf("unexpected updated policy: %+v", updated)
	}
	exclusionsSetting, err := client.SystemSetting.Query().
		Where(systemsetting.KeyEQ(SettingKeyOverwriteExclusions)).
		Only(context.Background())
	if err != nil {
		t.Fatalf("query persisted exclusions: %v", err)
	}
	if string(exclusionsSetting.Value) != `["latest"]` || exclusionsSetting.Version != 1 {
		t.Fatalf("unexpected persisted exclusions: %+v", exclusionsSetting)
	}

	idempotent, err := uc.Update(context.Background(), 1, true, []string{"latest"}, "other-admin")
	if err != nil {
		t.Fatalf("idempotent update: %v", err)
	}
	if idempotent.Version != 1 || idempotent.UpdatedBy != "admin" {
		t.Fatalf("idempotent update changed metadata: %+v", idempotent)
	}
	if _, err := uc.Update(context.Background(), 0, false, nil, "admin"); !errors.Is(err, ErrRegistryPolicyConflict) {
		t.Fatalf("stale update error = %v, want conflict", err)
	}
}

func TestRegistryPolicyLoadsLegacyBooleanAndNormalizesExclusions(t *testing.T) {
	client := newPolicyTestClient(t)
	if _, err := client.SystemSetting.Create().
		SetKey(SettingKeyPreventTagOverwrite).
		SetValue(json.RawMessage("true")).
		SetVersion(4).
		SetUpdatedBy("old-version").
		Save(context.Background()); err != nil {
		t.Fatalf("seed legacy boolean: %v", err)
	}
	uc := NewRegistryPolicyBiz(client)
	if err := uc.Initialize(context.Background()); err != nil {
		t.Fatalf("initialize legacy setting: %v", err)
	}
	loaded, err := uc.Get()
	if err != nil {
		t.Fatalf("get legacy setting: %v", err)
	}
	if !loaded.PreventTagOverwrite || loaded.Version != 4 || len(loaded.OverwriteExclusions) != 0 {
		t.Fatalf("unexpected legacy policy: %+v", loaded)
	}

	updated, err := uc.Update(context.Background(), 4, true, []string{"nightly", " latest ", "latest"}, "admin")
	if err != nil {
		t.Fatalf("add exclusions to legacy policy: %v", err)
	}
	if updated.Version != 5 || !slices.Equal(updated.OverwriteExclusions, []string{"latest", "nightly"}) {
		t.Fatalf("unexpected normalized policy: %+v", updated)
	}
	primary, err := client.SystemSetting.Query().
		Where(systemsetting.KeyEQ(SettingKeyPreventTagOverwrite)).
		Only(context.Background())
	if err != nil {
		t.Fatalf("reload primary setting: %v", err)
	}
	if string(primary.Value) != "true" {
		t.Fatalf("primary value changed format: %s", primary.Value)
	}
}

func TestRegistryPolicyRejectsInvalidOverwriteExclusion(t *testing.T) {
	uc := NewRegistryPolicyBiz(newPolicyTestClient(t))
	if err := uc.Initialize(context.Background()); err != nil {
		t.Fatalf("initialize: %v", err)
	}
	if _, err := uc.Update(context.Background(), 0, true, []string{"release/*"}, "admin"); !errors.Is(err, ErrRegistryPolicyInvalid) {
		t.Fatalf("invalid exclusion error = %v, want ErrRegistryPolicyInvalid", err)
	}
	current, err := uc.Get()
	if err != nil {
		t.Fatalf("get after invalid update: %v", err)
	}
	if current.Version != 0 {
		t.Fatalf("invalid update changed policy: %+v", current)
	}
}

func TestRegistryPolicyRejectsNonBooleanJSON(t *testing.T) {
	for _, value := range []string{"null", `"false"`, "0", `{}`} {
		t.Run(value, func(t *testing.T) {
			client := newPolicyTestClient(t)
			if _, err := client.SystemSetting.Create().
				SetKey(SettingKeyPreventTagOverwrite).
				SetValue(json.RawMessage(value)).
				SetVersion(1).
				SetUpdatedBy("system").
				Save(context.Background()); err != nil {
				t.Fatalf("seed setting: %v", err)
			}
			if err := NewRegistryPolicyBiz(client).Initialize(context.Background()); err == nil {
				t.Fatalf("Initialize accepted non-boolean JSON %s", value)
			}
		})
	}
}

func TestRegistryPolicyRejectsInvalidPersistedExclusions(t *testing.T) {
	for _, value := range []string{"null", `"latest"`, `["release/*"]`} {
		t.Run(value, func(t *testing.T) {
			client := newPolicyTestClient(t)
			if _, err := client.SystemSetting.Create().
				SetKey(SettingKeyPreventTagOverwrite).
				SetValue(json.RawMessage("true")).
				SetVersion(1).
				SetUpdatedBy("admin").
				Save(context.Background()); err != nil {
				t.Fatalf("seed primary setting: %v", err)
			}
			if _, err := client.SystemSetting.Create().
				SetKey(SettingKeyOverwriteExclusions).
				SetValue(json.RawMessage(value)).
				SetVersion(1).
				SetUpdatedBy("admin").
				Save(context.Background()); err != nil {
				t.Fatalf("seed exclusions setting: %v", err)
			}
			if err := NewRegistryPolicyBiz(client).Initialize(context.Background()); err == nil {
				t.Fatalf("Initialize accepted invalid exclusions JSON %s", value)
			}
		})
	}
}

func TestRegistryPolicySwitchWaitsForActivePut(t *testing.T) {
	uc := NewRegistryPolicyBiz(newPolicyTestClient(t))
	if err := uc.Initialize(context.Background()); err != nil {
		t.Fatalf("initialize: %v", err)
	}
	old, leave, err := uc.EnterManifestPut(context.Background())
	if err != nil || old.PreventTagOverwrite {
		t.Fatalf("enter old policy: policy=%+v err=%v", old, err)
	}

	updateDone := make(chan error, 1)
	go func() {
		_, updateErr := uc.Update(context.Background(), 0, true, nil, "admin")
		updateDone <- updateErr
	}()
	select {
	case err := <-updateDone:
		t.Fatalf("update passed active PUT barrier: %v", err)
	case <-time.After(30 * time.Millisecond):
	}

	newPut := make(chan *RegistryPolicy, 1)
	go func() {
		p, release, enterErr := uc.EnterManifestPut(context.Background())
		if enterErr != nil {
			newPut <- nil
			return
		}
		release()
		newPut <- p
	}()
	leave()

	select {
	case err := <-updateDone:
		if err != nil {
			t.Fatalf("update after release: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("update remained blocked after active PUT left")
	}
	select {
	case p := <-newPut:
		if p == nil || !p.PreventTagOverwrite {
			t.Fatalf("new PUT observed old policy: %+v", p)
		}
	case <-time.After(time.Second):
		t.Fatal("new PUT remained blocked")
	}
}
