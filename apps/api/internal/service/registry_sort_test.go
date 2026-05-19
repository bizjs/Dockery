package service

import (
	"testing"

	"api/internal/biz"
)

// TestSortMeta_TiebreakAlwaysAsc locks the contract that an equal-key
// tie always resolves by repo ASC, regardless of whether the primary
// sort is asc or desc. Without this, the in-memory path (used for
// pattern-restricted users) drifts from the SQL path (used for admins
// + unrestricted users), which means the same query returns a different
// page slice depending on who's logged in.
func TestSortMeta_TiebreakAlwaysAsc(t *testing.T) {
	// Three repos, all the same Size — only the repo name should
	// distinguish them. A correct implementation puts a, b, c in that
	// order whether direction is asc or desc.
	mk := func(repo string, size int64) *biz.RepoMeta {
		return &biz.RepoMeta{Repo: repo, Size: size}
	}
	cases := []struct {
		name      string
		direction string
	}{
		{"asc", "asc"},
		{"desc", "desc"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			items := []*biz.RepoMeta{mk("c/3", 100), mk("a/1", 100), mk("b/2", 100)}
			sortMeta(items, "size", c.direction)
			want := []string{"a/1", "b/2", "c/3"}
			for i, w := range want {
				if items[i].Repo != w {
					t.Errorf("[%s] items[%d] = %q, want %q (full: %v)",
						c.direction, i, items[i].Repo, w, repoNames(items))
				}
			}
		})
	}
}

// TestSortMeta_PrimaryDirectionRespected makes sure the tiebreak fix
// didn't accidentally lock the primary key to one direction.
func TestSortMeta_PrimaryDirectionRespected(t *testing.T) {
	items := []*biz.RepoMeta{
		{Repo: "a", Size: 100},
		{Repo: "b", Size: 200},
		{Repo: "c", Size: 50},
	}
	sortMeta(items, "size", "desc")
	wantDesc := []int64{200, 100, 50}
	for i, w := range wantDesc {
		if items[i].Size != w {
			t.Errorf("desc items[%d].Size = %d, want %d", i, items[i].Size, w)
		}
	}
	sortMeta(items, "size", "asc")
	wantAsc := []int64{50, 100, 200}
	for i, w := range wantAsc {
		if items[i].Size != w {
			t.Errorf("asc items[%d].Size = %d, want %d", i, items[i].Size, w)
		}
	}
}

// TestSortMeta_NameDirection guards against a regression where
// sort=name&direction=desc silently returned ASC because the primary
// comparator for "name" defaulted to "always equal" and only the
// repo-ASC tiebreak fired. Must mirror the SQL path's
// `ORDER BY repo DESC` behavior.
func TestSortMeta_NameDirection(t *testing.T) {
	mk := func(repo string) *biz.RepoMeta { return &biz.RepoMeta{Repo: repo} }
	items := []*biz.RepoMeta{mk("b"), mk("a"), mk("c")}

	sortMeta(items, "name", "desc")
	wantDesc := []string{"c", "b", "a"}
	for i, w := range wantDesc {
		if items[i].Repo != w {
			t.Errorf("name desc items[%d] = %q, want %q (full: %v)",
				i, items[i].Repo, w, repoNames(items))
		}
	}

	sortMeta(items, "name", "asc")
	wantAsc := []string{"a", "b", "c"}
	for i, w := range wantAsc {
		if items[i].Repo != w {
			t.Errorf("name asc items[%d] = %q, want %q", i, items[i].Repo, w)
		}
	}
}

// TestSortMeta_UpdatedField confirms ISO-8601 string ordering still
// reads as chronological ordering after the comparator refactor.
func TestSortMeta_UpdatedField(t *testing.T) {
	items := []*biz.RepoMeta{
		{Repo: "a", Created: "2026-04-20T00:00:00Z"},
		{Repo: "b", Created: "2026-04-22T00:00:00Z"},
		{Repo: "c", Created: "2026-04-21T00:00:00Z"},
	}
	sortMeta(items, "updated", "desc")
	if items[0].Repo != "b" || items[1].Repo != "c" || items[2].Repo != "a" {
		t.Errorf("desc by updated = %v, want [b c a]", repoNames(items))
	}
}

func repoNames(items []*biz.RepoMeta) []string {
	out := make([]string, len(items))
	for i, m := range items {
		out[i] = m.Repo
	}
	return out
}
