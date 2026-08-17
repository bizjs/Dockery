package data

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"api/internal/conf"
	"api/internal/data/ent"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	"github.com/go-kratos/kratos/v2/log"
	"github.com/google/wire"

	_ "modernc.org/sqlite"
)

// ProviderSet is data providers.
var ProviderSet = wire.NewSet(NewData, ProvideEntClient, NewUserRepo, NewPermissionRepo, NewAuditRepo, NewRepoMetaRepo)

// Data wraps shared data-layer resources.
type Data struct {
	db  *ent.Client
	log *log.Helper
}

// DB returns the ent client.
func (d *Data) DB() *ent.Client { return d.db }

// ProvideEntClient exposes the generated client for the registry policy,
// whose persistence logic is intentionally kept directly in biz. Existing
// repositories continue to use Data unchanged.
func ProvideEntClient(d *Data) *ent.Client { return d.db }

// NewData opens the SQLite database via modernc.org/sqlite, wires an ent
// client and auto-migrates all schemas.
//
// Production deployments wanting auditable DDL should instead generate
// migrations via `atlas migrate diff` against the ent schemas and apply
// them out-of-band; the auto-migrate here is intended for single-node
// self-hosted installs where ease of setup matters more than change
// control.
func NewData(c *conf.Data, logger log.Logger) (*Data, func(), error) {
	helper := log.NewHelper(log.With(logger, "module", "data"))

	stdDB, err := sql.Open(c.Database.Driver, c.Database.Source)
	if err != nil {
		return nil, nil, fmt.Errorf("open %s: %w", c.Database.Driver, err)
	}

	drv := entsql.OpenDB(dialect.SQLite, stdDB)
	client := ent.NewClient(ent.Driver(drv))

	migrationCtx, cancelMigration := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancelMigration()
	if err := client.Schema.Create(migrationCtx); err != nil {
		_ = client.Close()
		return nil, nil, fmt.Errorf("ent schema create: %w", err)
	}
	helper.Infof("ent client ready, driver=%s", c.Database.Driver)

	d := &Data{db: client, log: helper}
	cleanup := func() {
		helper.Info("closing ent client")
		_ = client.Close()
	}
	return d, cleanup, nil
}
