package biz

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-kratos/kratos/v2/log"
)

// fakeAuditRepo captures writes for assertion without touching SQLite.
type fakeAuditRepo struct {
	mu      sync.Mutex
	entries []AuditEntry
}

func (f *fakeAuditRepo) Create(_ context.Context, e *AuditEntry) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.entries = append(f.entries, *e)
	return nil
}

func (f *fakeAuditRepo) Query(_ context.Context, _ AuditFilter) ([]*AuditEntry, int, error) {
	return nil, 0, nil
}

// actions returns captured audit action names in order.
func (f *fakeAuditRepo) actions() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]string, 0, len(f.entries))
	for _, e := range f.entries {
		out = append(out, e.Action)
	}
	return out
}

func newGCTestHarness(t *testing.T, run runner) (*GCRunner, *Maintenance, *fakeAuditRepo) {
	t.Helper()
	mode := NewMaintenance()
	audit := &fakeAuditRepo{}
	uc := NewAuditUsecase(audit, log.NewStdLogger(discard{}))
	r := newGCRunnerWithRunner(GCConfig{
		SupervisorctlBin: "/bin/supervisorctl",
		SupervisordConf:  "/etc/supervisord.conf",
		RegistryBin:      "/bin/registry",
		RegistryConf:     "/etc/docker/registry/config.yml",
		ServiceName:      "registry",
		DeleteUntagged:   true,
		Timeout:          5 * time.Second,
	}, mode, uc, log.NewStdLogger(discard{}), run)
	return r, mode, audit
}

// discard is an io.Writer that swallows kratos log output in tests.
type discard struct{}

func (discard) Write(p []byte) (int, error) { return len(p), nil }

func TestGCRun_HappyPath(t *testing.T) {
	var calls []string
	run := func(_ context.Context, name string, args ...string) (string, error) {
		calls = append(calls, name+" "+strings.Join(args, " "))
		// Return sample output for the garbage-collect command.
		if strings.Contains(strings.Join(args, " "), "garbage-collect") {
			return "blob eligible for deletion: sha256:abc\n", nil
		}
		return "", nil
	}
	r, mode, audit := newGCTestHarness(t, run)

	result, err := r.Run(context.Background(), "alice", "127.0.0.1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil || !strings.Contains(result.Output, "sha256:abc") {
		t.Fatalf("missing gc output: %+v", result)
	}
	// stop → garbage-collect → start
	if len(calls) != 3 {
		t.Fatalf("expected 3 commands, got %d: %v", len(calls), calls)
	}
	if !strings.Contains(calls[0], "stop registry") {
		t.Fatalf("first call should be stop: %s", calls[0])
	}
	if !strings.Contains(calls[1], "garbage-collect") || !strings.Contains(calls[1], "--delete-untagged") {
		t.Fatalf("second call should be garbage-collect --delete-untagged: %s", calls[1])
	}
	if !strings.Contains(calls[2], "start registry") {
		t.Fatalf("third call should be start: %s", calls[2])
	}
	// Maintenance flag must be cleared after Run returns.
	if mode.Active() {
		t.Fatalf("maintenance flag still active after Run")
	}
	// Two audit entries: gc.started and gc.completed (success=true).
	gotActions := audit.actions()
	if len(gotActions) != 2 || gotActions[0] != ActionGCStarted || gotActions[1] != ActionGCCompleted {
		t.Fatalf("unexpected audit trail: %v", gotActions)
	}
	if !audit.entries[1].Success {
		t.Fatalf("gc.completed should be success=true")
	}
}

func TestGCRun_SingleFlight(t *testing.T) {
	// First run holds the lock in a blocking channel; second run should
	// fast-fail with ErrGCAlreadyRunning.
	block := make(chan struct{})
	started := make(chan struct{})
	run := func(_ context.Context, _ string, args ...string) (string, error) {
		if strings.Contains(strings.Join(args, " "), "garbage-collect") {
			close(started)
			<-block
		}
		return "", nil
	}
	r, _, _ := newGCTestHarness(t, run)

	done := make(chan error, 1)
	go func() {
		_, err := r.Run(context.Background(), "alice", "")
		done <- err
	}()
	<-started

	// Second call should be rejected while the first is mid-flight.
	if _, err := r.Run(context.Background(), "bob", ""); !errors.Is(err, ErrGCAlreadyRunning) {
		t.Fatalf("expected ErrGCAlreadyRunning, got %v", err)
	}

	close(block)
	if err := <-done; err != nil {
		t.Fatalf("first run should succeed: %v", err)
	}
}

func TestGCRun_MaintenanceFlagFlips(t *testing.T) {
	// We need access to the Maintenance flag inside the run callback to
	// assert it's active mid-flight; build the harness first, then close
	// over mode in a runner.
	var r *GCRunner
	var mode *Maintenance
	seen := false
	run := func(_ context.Context, _ string, args ...string) (string, error) {
		if strings.Contains(strings.Join(args, " "), "garbage-collect") {
			seen = true
			if !mode.Active() {
				t.Errorf("maintenance should be active during garbage-collect")
			}
		}
		return "", nil
	}
	r, mode, _ = newGCTestHarness(t, run)

	if _, err := r.Run(context.Background(), "alice", ""); err != nil {
		t.Fatalf("run: %v", err)
	}
	if !seen {
		t.Fatalf("garbage-collect never invoked")
	}
	if mode.Active() {
		t.Fatalf("maintenance flag should be cleared after Run returns")
	}
}

func TestGCRun_GCFailureStillRestartsRegistry(t *testing.T) {
	var calls []string
	run := func(_ context.Context, _ string, args ...string) (string, error) {
		joined := strings.Join(args, " ")
		calls = append(calls, joined)
		if strings.Contains(joined, "garbage-collect") {
			return "boom", errors.New("gc exploded")
		}
		return "", nil
	}
	r, mode, audit := newGCTestHarness(t, run)

	_, err := r.Run(context.Background(), "alice", "")
	if err == nil {
		t.Fatalf("expected GC failure to propagate")
	}
	if !strings.Contains(err.Error(), "garbage-collect") {
		t.Fatalf("error should mention garbage-collect: %v", err)
	}
	// Must still have called start-registry after failed GC.
	foundStart := false
	for _, c := range calls {
		if strings.Contains(c, "start registry") {
			foundStart = true
			break
		}
	}
	if !foundStart {
		t.Fatalf("registry was not restarted after GC failure: %v", calls)
	}
	// gc.completed audit must be written with success=false.
	if len(audit.entries) != 2 || audit.entries[1].Action != ActionGCCompleted || audit.entries[1].Success {
		t.Fatalf("audit trail wrong: %+v", audit.entries)
	}
	if mode.Active() {
		t.Fatalf("maintenance flag still active after failed Run")
	}
}

// buildFakeRegistryRoot creates a distribution-shaped directory tree
// under root for testing pruneEmptyRepos. For each entry in repos, it
// creates `<root>/docker/registry/v2/repositories/<name>/_manifests/`
// and, if tagged, a dummy tag subdir so the repo looks occupied.
func buildFakeRegistryRoot(t *testing.T, root string, repos map[string]bool) {
	t.Helper()
	base := filepath.Join(root, "docker", "registry", "v2", "repositories")
	for name, hasTag := range repos {
		repo := filepath.Join(base, name)
		if err := os.MkdirAll(filepath.Join(repo, "_manifests", "tags"), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", repo, err)
		}
		if hasTag {
			if err := os.MkdirAll(filepath.Join(repo, "_manifests", "tags", "latest", "current"), 0o755); err != nil {
				t.Fatalf("mkdir tag: %v", err)
			}
		}
	}
}

func TestPruneEmptyRepos_RemovesEmptyAndKeepsTagged(t *testing.T) {
	root := t.TempDir()
	buildFakeRegistryRoot(t, root, map[string]bool{
		"demo/hello":     false, // empty → prune
		"demo/keep":      true,  // tagged → keep
		"alice/team/app": false, // empty nested → prune (+ rm namespace parents)
		"alice/other":    true,  // tagged → keep, so alice/ must stay
	})

	r := newGCRunnerWithRunner(GCConfig{
		RegistryRootDir: root,
		PruneEmptyRepos: true,
	}, NewMaintenance(), NewAuditUsecase(&fakeAuditRepo{}, log.NewStdLogger(discard{})),
		log.NewStdLogger(discard{}), func(context.Context, string, ...string) (string, error) { return "", nil })

	pruned, err := r.pruneEmptyRepos()
	if err != nil {
		t.Fatalf("prune: %v", err)
	}
	sort.Strings(pruned)
	want := []string{"alice/team/app", "demo/hello"}
	if strings.Join(pruned, ",") != strings.Join(want, ",") {
		t.Fatalf("pruned=%v want=%v", pruned, want)
	}

	base := filepath.Join(root, "docker", "registry", "v2", "repositories")
	// Removed repos are gone.
	for _, rel := range want {
		if _, err := os.Stat(filepath.Join(base, rel)); !os.IsNotExist(err) {
			t.Errorf("%s should be removed, err=%v", rel, err)
		}
	}
	// demo/ is now empty (only demo/keep survived which has tags ... wait)
	// Actually demo/keep has tags so it stays; demo/ keeps demo/keep so
	// demo/ is NOT empty. Must still exist.
	if _, err := os.Stat(filepath.Join(base, "demo")); err != nil {
		t.Errorf("demo/ should survive: %v", err)
	}
	if _, err := os.Stat(filepath.Join(base, "demo", "keep")); err != nil {
		t.Errorf("demo/keep should survive: %v", err)
	}
	// alice/team/ was the sole child of alice/team after pruning the
	// leaf repo, so team should be rmdir'd. alice/other survives so
	// alice/ stays.
	if _, err := os.Stat(filepath.Join(base, "alice", "team")); !os.IsNotExist(err) {
		t.Errorf("alice/team should be cleaned up, err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(base, "alice", "other")); err != nil {
		t.Errorf("alice/other should survive: %v", err)
	}
}

func TestPruneEmptyRepos_MissingRootIsNoOp(t *testing.T) {
	r := newGCRunnerWithRunner(GCConfig{
		RegistryRootDir: filepath.Join(t.TempDir(), "nonexistent"),
		PruneEmptyRepos: true,
	}, NewMaintenance(), NewAuditUsecase(&fakeAuditRepo{}, log.NewStdLogger(discard{})),
		log.NewStdLogger(discard{}), func(context.Context, string, ...string) (string, error) { return "", nil })

	pruned, err := r.pruneEmptyRepos()
	if err != nil {
		t.Fatalf("missing root should be no-op, got err=%v", err)
	}
	if len(pruned) != 0 {
		t.Fatalf("expected no pruning, got %v", pruned)
	}
}

func TestGCRun_StopFailureShortCircuits(t *testing.T) {
	var calls []string
	run := func(_ context.Context, _ string, args ...string) (string, error) {
		joined := strings.Join(args, " ")
		calls = append(calls, joined)
		if strings.Contains(joined, "stop registry") {
			return "permission denied", errors.New("stop failed")
		}
		return "", nil
	}
	r, _, _ := newGCTestHarness(t, run)

	_, err := r.Run(context.Background(), "alice", "")
	if err == nil {
		t.Fatalf("expected stop failure to propagate")
	}
	// GC should not have been attempted if stop failed.
	for _, c := range calls {
		if strings.Contains(c, "garbage-collect") {
			t.Fatalf("garbage-collect should not run after stop failure, calls: %v", calls)
		}
	}
}
