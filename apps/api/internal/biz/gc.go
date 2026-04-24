package biz

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
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
	// RegistryRootDir is the filesystem root used by distribution's
	// filesystem driver (`storage.filesystem.rootdirectory` in its
	// config.yml, default /data/registry). GCRunner reads this to
	// prune empty repo directories after GC — distribution's
	// garbage-collect leaves those dirs behind, so `/v2/_catalog` keeps
	// listing repos that have no tags.
	RegistryRootDir string
	// PruneEmptyRepos enables the post-GC sweep that removes repo
	// directories with no tags (and their now-empty namespace parents).
	// Default: true; set false only when using a non-filesystem
	// storage driver (S3 etc.) where the layout differs.
	PruneEmptyRepos bool
	Timeout         time.Duration // hard cap on the whole op; default 30 min
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
		RegistryRootDir:  "/data/registry",
		PruneEmptyRepos:  true,
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
// `registry garbage-collect` → start registry → clear flag →
// resync the repo_meta cache.
// At most one run at a time (single-flight via mu.TryLock).
type GCRunner struct {
	cfg        GCConfig
	mode       *Maintenance
	audit      *AuditUsecase
	meta       *RepoMetaUsecase
	reconciler *Reconciler
	log        *log.Helper

	run runner     // swappable for tests
	mu  sync.Mutex // single-flight lock
}

// NewGCRunner constructs a runner with default exec behaviour. Tests
// should instead use newGCRunnerWithRunner to inject a fake.
func NewGCRunner(cfg GCConfig, mode *Maintenance, audit *AuditUsecase, meta *RepoMetaUsecase, reconciler *Reconciler, logger log.Logger) *GCRunner {
	return newGCRunnerWithRunner(cfg, mode, audit, meta, reconciler, logger, defaultRunner)
}

func newGCRunnerWithRunner(cfg GCConfig, mode *Maintenance, audit *AuditUsecase, meta *RepoMetaUsecase, reconciler *Reconciler, logger log.Logger, run runner) *GCRunner {
	if cfg.Timeout <= 0 {
		cfg.Timeout = 30 * time.Minute
	}
	return &GCRunner{
		cfg:        cfg,
		mode:       mode,
		audit:      audit,
		meta:       meta,
		reconciler: reconciler,
		log:        log.NewHelper(log.With(logger, "module", "biz/gc")),
		run:        run,
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

	// Always return the partial result so callers can surface the
	// supervisorctl / registry output even when the run fails — that
	// output is the single most useful piece of debugging info for the
	// operator.
	result := &GCResult{Duration: duration, Output: output}
	if err != nil {
		return result, err
	}
	// GC bypasses the HTTP API so distribution never emits
	// push/delete webhooks for blobs/repos it sweeps. Re-sync the
	// cache explicitly:
	//   1. ReconcileOnce diffs /v2/_catalog → deletes rows for repos
	//      that vanished entirely;
	//   2. EnqueueRefresh for every remaining cached repo → the
	//      refresh worker updates size/tag_count for those whose
	//      blobs shrank but that still have tags.
	r.resyncCache(runCtx, actor, clientIP)
	return result, nil
}

// resyncCache is the post-GC cache reconcile. Idempotent and
// best-effort — any error is logged but doesn't fail the GC response,
// because the user's real action (GC) already succeeded. The next
// periodic reconcile will catch anything this missed.
func (r *GCRunner) resyncCache(ctx context.Context, actor, clientIP string) {
	if r.reconciler == nil || r.meta == nil {
		return
	}
	r.reconciler.ReconcileOnce(ctx)
	repos, err := r.meta.AllRepos(ctx)
	if err != nil {
		r.log.Warnf("post-gc cache resync: list repos: %v", err)
	}
	for _, repo := range repos {
		r.meta.EnqueueRefresh(repo)
	}
	r.audit.Write(ctx, AuditEntry{
		Actor:    actor,
		Action:   ActionCacheResynced,
		ClientIP: clientIP,
		Success:  true,
		Detail:   map[string]any{"reason": "post-gc", "enqueued": len(repos)},
	})
}

// doRun is the stop / GC / prune / restart sequence. The restart is
// attempted even when GC fails so we don't leave a stopped registry
// behind; any restart error is appended to the primary error, not
// substituted.
//
// The prune step (post-GC) walks the filesystem and removes repository
// directories that have no tags — distribution's own garbage-collect
// leaves those behind, causing /v2/_catalog to list "zombie" repos
// with zero tags forever.
func (r *GCRunner) doRun(ctx context.Context) (string, error) {
	if out, err := r.supervisor(ctx, "stop", r.cfg.ServiceName); err != nil {
		return out, fmt.Errorf("supervisorctl stop: %w (output: %s)", err, strings.TrimSpace(out))
	}

	gcOut, gcErr := r.garbageCollect(ctx)

	// Prune only runs when GC succeeded. A failed GC could leave the
	// layout in a half-swept state; we'd rather leave repos alone than
	// accidentally wipe data.
	var pruneOut string
	if gcErr == nil && r.cfg.PruneEmptyRepos {
		pruned, err := r.pruneEmptyRepos()
		if err != nil {
			// Non-fatal: log + surface in output but don't fail the whole run.
			r.log.Errorf("prune empty repos: %v", err)
			pruneOut = "\n\nprune warning: " + err.Error()
		}
		if len(pruned) > 0 {
			pruneOut = fmt.Sprintf("\n\npruned %d empty repositor%s:\n  %s",
				len(pruned),
				pluralY(len(pruned)),
				strings.Join(pruned, "\n  "))
		}
	}

	startOut, startErr := r.supervisor(ctx, "start", r.cfg.ServiceName)

	combined := gcOut + pruneOut

	switch {
	case gcErr != nil && startErr != nil:
		return combined + "\n\n--- restart also failed ---\n" + startOut,
			fmt.Errorf("gc failed (%w) AND restart failed: %v", gcErr, startErr)
	case gcErr != nil:
		// GC failed but registry is back up — the more useful signal.
		return combined, fmt.Errorf("garbage-collect: %w", gcErr)
	case startErr != nil:
		// GC succeeded but we couldn't restart — critical: registry is offline.
		r.log.Errorf("GC succeeded but registry restart failed: %v; output: %s", startErr, startOut)
		return combined + "\n\n--- restart failed (registry is OFFLINE) ---\n" + startOut,
			fmt.Errorf("restart registry: %w", startErr)
	}
	return combined, nil
}

// pruneEmptyRepos walks <RegistryRootDir>/docker/registry/v2/repositories
// and removes any repo whose _manifests/tags/ directory is empty, then
// cleans up now-empty namespace parents. Returns the repo names
// (relative to the repositories root) that were removed.
//
// Definition of a "repo" directory: any dir that contains a
// _manifests/ child. This matches distribution's on-disk layout and
// correctly handles nested names like alice/team/app (only the leaf
// is a repo; the ancestors are namespace dirs).
func (r *GCRunner) pruneEmptyRepos() ([]string, error) {
	if r.cfg.RegistryRootDir == "" {
		return nil, nil
	}
	reposRoot := filepath.Join(r.cfg.RegistryRootDir, "docker", "registry", "v2", "repositories")
	info, err := os.Stat(reposRoot)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("stat repositories root: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("%s: not a directory", reposRoot)
	}

	var emptyRepos []string
	walkErr := filepath.WalkDir(reposRoot, func(path string, d fs.DirEntry, werr error) error {
		if werr != nil {
			return werr
		}
		if !d.IsDir() || path == reposRoot {
			return nil
		}
		manifestsDir := filepath.Join(path, "_manifests")
		if _, err := os.Stat(manifestsDir); err != nil {
			if os.IsNotExist(err) {
				return nil // not a repo, keep walking into sub-namespaces
			}
			return err
		}
		// It's a repo — check for any tag.
		tagsDir := filepath.Join(manifestsDir, "tags")
		entries, err := os.ReadDir(tagsDir)
		if err != nil && !os.IsNotExist(err) {
			return err
		}
		if len(entries) > 0 {
			// Still has tags; don't descend into repo internals.
			return fs.SkipDir
		}
		rel, err := filepath.Rel(reposRoot, path)
		if err != nil {
			return err
		}
		emptyRepos = append(emptyRepos, filepath.ToSlash(rel))
		// Skip descent so _manifests/_layers/_uploads aren't traversed.
		return fs.SkipDir
	})
	if walkErr != nil {
		return nil, walkErr
	}

	for _, rel := range emptyRepos {
		if err := os.RemoveAll(filepath.Join(reposRoot, rel)); err != nil {
			return emptyRepos, fmt.Errorf("remove %s: %w", rel, err)
		}
	}
	// Walk up each removed repo's namespace parents and rmdir any that
	// are now empty. os.Remove only succeeds on empty dirs, so we get
	// the "stop at the first non-empty ancestor" semantics for free.
	for _, rel := range emptyRepos {
		dir := filepath.Dir(rel)
		for dir != "." && dir != "" && dir != "/" {
			if err := os.Remove(filepath.Join(reposRoot, dir)); err != nil {
				break // non-empty or already gone
			}
			dir = filepath.Dir(dir)
		}
	}
	return emptyRepos, nil
}

func pluralY(n int) string {
	if n == 1 {
		return "y"
	}
	return "ies"
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
