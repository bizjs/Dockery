package data

import (
	"context"
	"fmt"
	"time"

	"api/internal/biz"
	"api/internal/data/ent"
	"api/internal/data/ent/repometa"
	"api/internal/data/ent/schema"

	"github.com/go-kratos/kratos/v2/log"
)

type repoMetaRepo struct {
	data *Data
	log  *log.Helper
}

// NewRepoMetaRepo wires the ent-backed implementation of
// biz.RepoMetaRepo. Declared as the biz interface so wire's Bind picks
// up the correct slot.
func NewRepoMetaRepo(d *Data, logger log.Logger) biz.RepoMetaRepo {
	return &repoMetaRepo{
		data: d,
		log:  log.NewHelper(log.With(logger, "module", "data/repo_meta")),
	}
}

// Upsert writes a full snapshot for `repo`. Uses ent's unique-index
// collision detection: try Create, on constraint failure fall back to
// Update. Cheaper than a pre-check query for our write rate.
func (r *repoMetaRepo) Upsert(ctx context.Context, m *biz.RepoMeta) error {
	if m == nil || m.Repo == "" {
		return fmt.Errorf("repo_meta: Repo is required")
	}
	refreshedAt := unixToTime(m.RefreshedAt)
	platforms := m.Platforms
	if platforms == nil {
		platforms = []schema.PlatformInfo{}
	}

	create := r.data.DB().RepoMeta.Create().
		SetRepo(m.Repo).
		SetLatestTag(m.LatestTag).
		SetTagCount(m.TagCount).
		SetSize(m.Size).
		SetCreated(m.Created).
		SetPlatforms(platforms).
		SetPullCount(m.PullCount).
		SetRefreshedAt(refreshedAt)
	if m.LastPulledAt != nil {
		create = create.SetLastPulledAt(unixToTime(*m.LastPulledAt))
	}
	if _, err := create.Save(ctx); err == nil {
		return nil
	} else if !ent.IsConstraintError(err) {
		return fmt.Errorf("repo_meta create: %w", err)
	}

	// Row exists → update. Pull counters are preserved (we don't reset
	// them on every meta refresh) so only fields driven by the manifest
	// fetch are touched here.
	update := r.data.DB().RepoMeta.Update().
		Where(repometa.RepoEQ(m.Repo)).
		SetLatestTag(m.LatestTag).
		SetTagCount(m.TagCount).
		SetSize(m.Size).
		SetCreated(m.Created).
		SetPlatforms(platforms).
		SetRefreshedAt(refreshedAt)
	if _, err := update.Save(ctx); err != nil {
		return fmt.Errorf("repo_meta update: %w", err)
	}
	return nil
}

func (r *repoMetaRepo) Get(ctx context.Context, repo string) (*biz.RepoMeta, error) {
	row, err := r.data.DB().RepoMeta.Query().
		Where(repometa.RepoEQ(repo)).
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, biz.ErrRepoMetaNotFound
		}
		return nil, err
	}
	return toBizRepoMeta(row), nil
}

func (r *repoMetaRepo) Delete(ctx context.Context, repo string) error {
	_, err := r.data.DB().RepoMeta.Delete().
		Where(repometa.RepoEQ(repo)).
		Exec(ctx)
	return err
}

func (r *repoMetaRepo) List(ctx context.Context) ([]*biz.RepoMeta, error) {
	rows, err := r.data.DB().RepoMeta.Query().
		Order(ent.Asc(repometa.FieldRepo)).
		All(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]*biz.RepoMeta, 0, len(rows))
	for _, row := range rows {
		out = append(out, toBizRepoMeta(row))
	}
	return out, nil
}

func (r *repoMetaRepo) AllRepos(ctx context.Context) ([]string, error) {
	return r.data.DB().RepoMeta.Query().
		Select(repometa.FieldRepo).
		Strings(ctx)
}

// IncrementPull atomically bumps pull_count and sets last_pulled_at.
// Uses ent's AddPullCount so concurrent pulls don't clobber each other.
// Returns the number of rows affected — 0 means the repo isn't in the
// cache yet, letting the caller trigger a refresh.
func (r *repoMetaRepo) IncrementPull(ctx context.Context, repo string, at time.Time) (int, error) {
	n, err := r.data.DB().RepoMeta.Update().
		Where(repometa.RepoEQ(repo)).
		AddPullCount(1).
		SetLastPulledAt(at).
		Save(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return 0, nil
		}
		return 0, err
	}
	return n, nil
}

// --- conversions ---

func toBizRepoMeta(row *ent.RepoMeta) *biz.RepoMeta {
	m := &biz.RepoMeta{
		Repo:        row.Repo,
		LatestTag:   row.LatestTag,
		TagCount:    row.TagCount,
		Size:        row.Size,
		Created:     row.Created,
		Platforms:   row.Platforms,
		PullCount:   row.PullCount,
		RefreshedAt: row.RefreshedAt.Unix(),
	}
	if row.LastPulledAt != nil {
		ts := row.LastPulledAt.Unix()
		m.LastPulledAt = &ts
	}
	return m
}

// unixToTime converts a unix-second value to time.Time, falling back to
// time.Now() when the input is zero (callers that want "now" often just
// set RefreshedAt to 0 and let the layer stamp it).
func unixToTime(sec int64) time.Time {
	if sec == 0 {
		return time.Now()
	}
	return time.Unix(sec, 0)
}

var _ biz.RepoMetaRepo = (*repoMetaRepo)(nil)
