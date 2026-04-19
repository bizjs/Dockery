package biz

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/go-kratos/kratos/v2/log"
)

// GCConfig pins the external-command paths GCRunner shells out to. The
// defaults target the paths baked into the Dockery container image;
// tests override via the constructor.
type GCConfig struct {
	SupervisorctlBin string        // e.g. /usr/bin/supervisorctl
	SupervisordConf  string        // e.g. /etc/supervisord.conf
	RegistryBin      string        // e.g. /usr/local/bin/registry
	RegistryConf     string        // e.g. /etc/docker/registry/config.yml
	ServiceName      string        // supervisord program name, e.g. "registry"
	DeleteUntagged   bool          // pass --delete-untagged to garbage-collect
	Timeout          time.Duration // hard cap on the whole op; default 30 min
}

// defaultGCConfig returns container-baked-in paths. Still used when the
// yaml section is entirely absent.
func defaultGCConfig() GCConfig {
	return GCConfig{
		SupervisorctlBin: "/usr/bin/supervisorctl",
		SupervisordConf:  "/etc/supervisord.conf",
		RegistryBin:      "/usr/local/bin/registry",
		RegistryConf:     "/etc/docker/registry/config.yml",
		ServiceName:      "registry",
		DeleteUntagged:   true,
		Timeout:          30 * time.Minute,
	}
}

// ErrGCAlreadyRunning is returned when a Run() arrives while a previous
// one is still in flight. Callers should translate to HTTP 409.
var ErrGCAlreadyRunning = errors.New("gc: already in progress")

// GCResult summarises a successful run for the API response / audit row.
type GCResult struct {
	Duration time.Duration
	Output   string // CombinedOutput from `registry garbage-collect`
}

// runner abstracts exec.CommandContext so tests can inject fakes without
// shelling out to real binaries.
type runner func(ctx context.Context, name string, args ...string) (string, error)

// defaultRunner shells out and returns the combined stdout+stderr.
func defaultRunner(ctx context.Context, name string, args ...string) (string, error) {
	out, err := exec.CommandContext(ctx, name, args...).CombinedOutput()
	return string(out), err
}

// GCRunner orchestrates: set maintenance flag → stop registry →
// `registry garbage-collect` → start registry → clear flag.
// At most one run at a time (single-flight via mu.TryLock).
type GCRunner struct {
	cfg   GCConfig
	mode  *Maintenance
	audit *AuditUsecase
	log   *log.Helper

	run runner     // swappable for tests
	mu  sync.Mutex // single-flight lock
}

// NewGCRunner constructs a runner with default exec behaviour. Tests
// should instead use newGCRunnerWithRunner to inject a fake.
func NewGCRunner(cfg GCConfig, mode *Maintenance, audit *AuditUsecase, logger log.Logger) *GCRunner {
	return newGCRunnerWithRunner(cfg, mode, audit, logger, defaultRunner)
}

func newGCRunnerWithRunner(cfg GCConfig, mode *Maintenance, audit *AuditUsecase, logger log.Logger, run runner) *GCRunner {
	if cfg.Timeout <= 0 {
		cfg.Timeout = 30 * time.Minute
	}
	return &GCRunner{
		cfg:   cfg,
		mode:  mode,
		audit: audit,
		log:   log.NewHelper(log.With(logger, "module", "biz/gc")),
		run:   run,
	}
}

// Run executes one GC cycle. Single-flight: concurrent callers get
// ErrGCAlreadyRunning. The maintenance flag is set for the entire
// duration (including the restart phase) so the UI keeps rejecting
// writes until the registry is back up.
func (r *GCRunner) Run(ctx context.Context, actor, clientIP string) (*GCResult, error) {
	if !r.mu.TryLock() {
		return nil, ErrGCAlreadyRunning
	}
	defer r.mu.Unlock()

	r.mode.Enter()
	defer r.mode.Exit()

	r.audit.Write(ctx, AuditEntry{
		Actor:    actor,
		Action:   ActionGCStarted,
		ClientIP: clientIP,
		Success:  true,
	})

	// Detach from the caller's request ctx: if the admin navigates away
	// mid-run, we must NOT abort halfway (registry would stay stopped).
	runCtx, cancel := context.WithTimeout(context.Background(), r.cfg.Timeout)
	defer cancel()

	started := time.Now()
	output, err := r.doRun(runCtx)
	duration := time.Since(started)

	entry := AuditEntry{
		Actor:    actor,
		Action:   ActionGCCompleted,
		ClientIP: clientIP,
		Success:  err == nil,
		Detail: map[string]any{
			"duration_ms": duration.Milliseconds(),
		},
	}
	if err != nil {
		entry.Detail["error"] = err.Error()
	}
	r.audit.Write(ctx, entry)

	if err != nil {
		return nil, err
	}
	return &GCResult{Duration: duration, Output: output}, nil
}

// doRun is the stop / GC / restart sequence. The restart is attempted
// even when GC fails so we don't leave a stopped registry behind; any
// restart error is appended to the primary error, not substituted.
func (r *GCRunner) doRun(ctx context.Context) (string, error) {
	if out, err := r.supervisor(ctx, "stop", r.cfg.ServiceName); err != nil {
		return out, fmt.Errorf("supervisorctl stop: %w (output: %s)", err, strings.TrimSpace(out))
	}

	gcOut, gcErr := r.garbageCollect(ctx)

	startOut, startErr := r.supervisor(ctx, "start", r.cfg.ServiceName)

	switch {
	case gcErr != nil && startErr != nil:
		return gcOut + "\n\n--- restart also failed ---\n" + startOut,
			fmt.Errorf("gc failed (%w) AND restart failed: %v", gcErr, startErr)
	case gcErr != nil:
		// GC failed but registry is back up — the more useful signal.
		return gcOut, fmt.Errorf("garbage-collect: %w", gcErr)
	case startErr != nil:
		// GC succeeded but we couldn't restart — critical: registry is offline.
		r.log.Errorf("GC succeeded but registry restart failed: %v; output: %s", startErr, startOut)
		return gcOut + "\n\n--- restart failed (registry is OFFLINE) ---\n" + startOut,
			fmt.Errorf("restart registry: %w", startErr)
	}
	return gcOut, nil
}

func (r *GCRunner) supervisor(ctx context.Context, action, name string) (string, error) {
	return r.run(ctx, r.cfg.SupervisorctlBin, "-c", r.cfg.SupervisordConf, action, name)
}

func (r *GCRunner) garbageCollect(ctx context.Context) (string, error) {
	args := []string{"garbage-collect"}
	if r.cfg.DeleteUntagged {
		args = append(args, "--delete-untagged")
	}
	args = append(args, r.cfg.RegistryConf)
	return r.run(ctx, r.cfg.RegistryBin, args...)
}
