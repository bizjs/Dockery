package data

import (
	"context"
	"database/sql"
	"encoding/json"
	"path/filepath"
	"testing"

	"api/internal/biz"
	"api/internal/data/ent"
	"api/internal/data/ent/migrate"
	"api/internal/data/ent/systemsetting"
	"api/internal/data/ent/user"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	"entgo.io/ent/dialect/sql/schema"

	_ "modernc.org/sqlite"
)

func TestSystemSettingSchemaAutoMigratesOldDatabase(t *testing.T) {
	ctx := context.Background()
	dsn := "file:" + filepath.Join(t.TempDir(), "dockery.db") +
		"?_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)"
	stdDB, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}

	drv := entsql.OpenDB(dialect.SQLite, stdDB)
	client := ent.NewClient(ent.Driver(drv))
	t.Cleanup(func() {
		if err := client.Close(); err != nil {
			t.Errorf("close ent client: %v", err)
		}
	})

	// Recreate the schema from immediately before SystemSetting existed.
	tables, err := schema.CopyTables(migrate.Tables)
	if err != nil {
		t.Fatalf("copy migration tables: %v", err)
	}
	oldTables := tables[:0]
	for _, table := range tables {
		if table.Name != systemsetting.Table {
			oldTables = append(oldTables, table)
		}
	}
	if err := migrate.Create(ctx, client.Schema, oldTables); err != nil {
		t.Fatalf("create old schema: %v", err)
	}

	before, err := client.User.Create().
		SetUsername("before-upgrade").
		SetPasswordHash("not-a-real-password-hash").
		SetRole(user.RoleAdmin).
		Save(ctx)
	if err != nil {
		t.Fatalf("seed old-schema user: %v", err)
	}

	if err := client.Schema.Create(ctx); err != nil {
		t.Fatalf("auto-migrate new schema: %v", err)
	}
	after, err := client.User.Get(ctx, before.ID)
	if err != nil {
		t.Fatalf("read pre-upgrade user: %v", err)
	}
	if after.Username != before.Username || after.Role != before.Role {
		t.Fatalf("pre-upgrade user changed: before=%+v after=%+v", before, after)
	}

	count, err := client.SystemSetting.Query().Count(ctx)
	if err != nil {
		t.Fatalf("query new system_settings table: %v", err)
	}
	if count != 0 {
		t.Fatalf("schema migration must not invent setting rows: got %d", count)
	}

	// Startup reads a missing key as a virtual default and must leave the
	// table empty. Only an administrator's actual change creates the row.
	policy := biz.NewRegistryPolicyBiz(client)
	if err := policy.Initialize(ctx); err != nil {
		t.Fatalf("initialize virtual default: %v", err)
	}
	initial, err := policy.Get()
	if err != nil {
		t.Fatalf("get virtual default: %v", err)
	}
	if initial.PreventTagOverwrite || initial.Version != 0 {
		t.Fatalf("unexpected virtual default: %+v", initial)
	}
	count, err = client.SystemSetting.Query().Count(ctx)
	if err != nil || count != 0 {
		t.Fatalf("startup wrote a default row: count=%d err=%v", count, err)
	}

	updated, err := policy.Update(ctx, 0, true, nil, "admin")
	if err != nil {
		t.Fatalf("first administrator update: %v", err)
	}
	if !updated.PreventTagOverwrite || updated.Version != 1 || updated.UpdatedBy != "admin" {
		t.Fatalf("unexpected first persisted policy: %+v", updated)
	}
	setting, err := client.SystemSetting.Query().
		Where(systemsetting.KeyEQ(biz.SettingKeyPreventTagOverwrite)).
		Only(ctx)
	if err != nil {
		t.Fatalf("read first persisted system setting: %v", err)
	}
	if setting.Version != 1 || setting.UpdatedBy != "admin" {
		t.Fatalf("unexpected setting defaults: %+v", setting)
	}
	if string(setting.Value) != "true" {
		t.Fatalf("setting value = %s, want true", setting.Value)
	}

	if _, err := client.SystemSetting.Create().
		SetKey(setting.Key).
		SetValue(json.RawMessage("true")).
		Save(ctx); err == nil {
		t.Fatal("duplicate system setting key unexpectedly succeeded")
	}
	if _, err := client.SystemSetting.Create().
		SetKey("registry.example_future_setting").
		SetValue(json.RawMessage(`{"enabled":true}`)).
		Save(ctx); err != nil {
		t.Fatalf("second distinct setting failed: %v", err)
	}
	if _, err := stdDB.ExecContext(ctx,
		"UPDATE system_settings SET version = 0 WHERE key = ?", setting.Key); err == nil {
		t.Fatal("non-positive setting version unexpectedly succeeded")
	}

	if err := client.Schema.Create(ctx); err != nil {
		t.Fatalf("repeat auto-migration: %v", err)
	}
	reloaded, err := client.SystemSetting.Get(ctx, setting.ID)
	if err != nil {
		t.Fatalf("reload system setting: %v", err)
	}
	if string(reloaded.Value) != string(setting.Value) ||
		reloaded.Version != setting.Version || reloaded.UpdatedBy != setting.UpdatedBy {
		t.Fatalf("setting changed after repeat migration: before=%+v after=%+v", setting, reloaded)
	}
}
