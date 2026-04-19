package data

import (
	"context"
	"database/sql"
	"fmt"

	"api/internal/conf"
	"api/internal/data/ent"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	"github.com/go-kratos/kratos/v2/log"
	"github.com/google/wire"

	_ "modernc.org/sqlite"
)

// ProviderSet is data providers.
var ProviderSet = wire.NewSet(NewData, NewUserRepo, NewPermissionRepo, NewAuditRepo)

// Data wraps shared data-layer resources. Biz-level repos reach the ent
// client through DB() rather than importing the ent package themselves,
// so the biz layer stays testable without spinning up a real database.
type Data struct {
	db  *ent.Client
	log *log.Helper
}

// DB returns the ent client.
func (d *Data) DB() *ent.Client { return d.db }

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

	if err := client.Schema.Create(context.Background()); err != nil {
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
