package main

import (
	"context"
	"errors"
	"flag"
	"os"
	"time"

	"api/internal/biz"
	"api/internal/conf"

	"github.com/go-kratos/kratos/v2"
	"github.com/go-kratos/kratos/v2/config"
	"github.com/go-kratos/kratos/v2/config/file"
	"github.com/go-kratos/kratos/v2/log"
	"github.com/go-kratos/kratos/v2/middleware/tracing"
	"github.com/go-kratos/kratos/v2/transport/http"

	_ "go.uber.org/automaxprocs"
)

// go build -ldflags "-X main.Version=x.y.z"
var (
	Name     string
	Version  string
	flagconf string

	id, _ = os.Hostname()
)

func init() {
	flag.StringVar(&flagconf, "conf", "../../configs", "config path, eg: -conf config.yaml")
	if Name == "" {
		Name = "dockery-api"
	}
}

// newApp wires the kratos.App and performs first-boot admin bootstrap.
// The EnsureAdmin call here is intentional: if the users table is empty
// and no admin password is available, the process refuses to start.
// This is the only "side-effectful" provider in the wire graph; any
// other boot-time work belongs here — most notably, kicking off the
// reconciler so the repo_meta cache is self-healing from the first
// request onward.
func newApp(
	logger log.Logger,
	hs *http.Server,
	users *biz.UserUsecase,
	meta *biz.RepoMetaUsecase,
	reconciler *biz.Reconciler,
	dockery *conf.Dockery,
) *kratos.App {
	adminUser := dockery.Admin.Username
	adminPass := dockery.Admin.Password
	// env takes precedence over yaml so users can bootstrap without committing secrets.
	if v := os.Getenv("DOCKERY_ADMIN_USERNAME"); v != "" {
		adminUser = v
	}
	if v := os.Getenv("DOCKERY_ADMIN_PASSWORD"); v != "" {
		adminPass = v
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := users.EnsureAdmin(ctx, adminUser, adminPass); err != nil {
		if errors.Is(err, biz.ErrAdminPasswordUnset) {
			log.NewHelper(logger).Fatalf(
				"users table is empty and DOCKERY_ADMIN_PASSWORD is not set; " +
					"set the env (or dockery.admin.password in config) and retry")
		}
		log.NewHelper(logger).Fatalf("ensure admin: %v", err)
	}

	// Non-blocking: Start delays the first scan a few seconds so
	// distribution has a chance to finish startup, then runs on a
	// 30-min ticker. No effect on HTTP readiness.
	reconciler.Start()

	return kratos.New(
		kratos.ID(id),
		kratos.Name(Name),
		kratos.Version(Version),
		kratos.Metadata(map[string]string{}),
		kratos.Logger(logger),
		kratos.Server(hs),
		// Drain the background goroutines on shutdown. Reconciler first
		// so it doesn't enqueue new refreshes while the usecase worker
		// is draining.
		kratos.AfterStop(func(context.Context) error {
			reconciler.Stop()
			meta.Close()
			return nil
		}),
	)
}

func main() {
	flag.Parse()

	logger := log.With(log.NewStdLogger(os.Stdout),
		"ts", log.DefaultTimestamp,
		"caller", log.DefaultCaller,
		"service.id", id,
		"service.name", Name,
		"service.version", Version,
		"trace.id", tracing.TraceID(),
		"span.id", tracing.SpanID(),
	)

	c := config.New(config.WithSource(file.NewSource(flagconf)))
	defer c.Close()
	if err := c.Load(); err != nil {
		panic(err)
	}

	var bc conf.Bootstrap
	if err := c.Scan(&bc); err != nil {
		panic(err)
	}
	var dockeryConf conf.Dockery
	if err := c.Value("dockery").Scan(&dockeryConf); err != nil {
		// Missing dockery section is a hard error — Dockery cannot run
		// without keystore/token config.
		panic("config missing 'dockery' section: " + err.Error())
	}

	// Subcommand dispatch. `dockery-api [-conf …] user <verb> [args...]`.
	// Subcommands boot the biz layer but never start the HTTP server.
	args := flag.Args()
	if len(args) > 0 && args[0] == "user" {
		os.Exit(runUserCommand(args[1:], bc.Data, logger))
	}

	app, cleanup, err := wireApp(bc.Server, bc.Data, &dockeryConf, logger)
	if err != nil {
		panic(err)
	}
	defer cleanup()

	if err := app.Run(); err != nil {
		panic(err)
	}
}
