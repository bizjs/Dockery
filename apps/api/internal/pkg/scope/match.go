package scope

import "strings"

// Role is the Dockery authorization role. Roles alone decide the set of
// actions a user is capable of — repo_permissions rows only gate which
// repositories those actions apply to.
type Role string

const (
	RoleAdmin Role = "admin"
	RoleWrite Role = "write"
	RoleView  Role = "view"
)

// roleActions maps each role to the set of actions it can perform on
// any repository its patterns match. admin's "*" is a marker — the
// match algorithm short-circuits admin in Match() rather than consulting
// this table, but the mapping keeps the data model complete.
var roleActions = map[Role][]string{
	RoleAdmin: {ActionPull, ActionPush, ActionDelete, ActionAll},
	RoleWrite: {ActionPull, ActionPush, ActionDelete},
	RoleView:  {ActionPull},
}

// ActionsFor returns a defensive copy of the action set a role grants.
func ActionsFor(r Role) []string {
	src := roleActions[r]
	if src == nil {
		return nil
	}
	out := make([]string, len(src))
	copy(out, src)
	return out
}

// Match computes the actions actually granted to a user for a single
// requested scope.
//
// Rules (design.md §7.3):
//  1. `registry:*` scopes (e.g. registry:catalog:*) are admin-only.
//  2. admin users bypass pattern matching — requested actions pass through.
//  3. write/view users must have at least one repo_pattern glob-matching
//     the requested Name; if so, granted = requested ∩ role_actions.
func Match(role Role, patterns []string, requested Scope) []string {
	// Rule 1: registry-scoped operations are admin-only.
	if requested.Type == TypeRegistry {
		if role == RoleAdmin {
			return dedupePreserveOrder(requested.Actions)
		}
		return nil
	}

	// Rule 2: admin on any repository scope — grant whatever was asked.
	if role == RoleAdmin {
		return dedupePreserveOrder(requested.Actions)
	}

	// Rule 3: non-admin must match at least one pattern.
	if !anyPatternMatches(patterns, requested.Name) {
		return nil
	}

	return intersect(requested.Actions, roleActions[role])
}

// MatchPattern reports whether a single glob pattern matches a
// repository name. Supported wildcards:
//
//	*          — matches any repository (whole name)
//	alice/*    — any repo whose *last segment* is under "alice/"
//	alice/app  — exact match
//
// Semantics: "*" substitutes exactly one path segment (i.e. it does
// not cross "/"). This mirrors the common intuition that "alice/*"
// means repos owned by alice but not sub-namespaces.
func MatchPattern(pattern, name string) bool {
	if pattern == "*" {
		return true
	}
	if !strings.Contains(pattern, "*") {
		return pattern == name
	}

	// Split both by "/" and compare segment by segment.
	pSegs := strings.Split(pattern, "/")
	nSegs := strings.Split(name, "/")
	if len(pSegs) != len(nSegs) {
		return false
	}
	for i, ps := range pSegs {
		if ps == "*" {
			continue
		}
		if ps != nSegs[i] {
			return false
		}
	}
	return true
}

func anyPatternMatches(patterns []string, name string) bool {
	for _, p := range patterns {
		if MatchPattern(p, name) {
			return true
		}
	}
	return false
}

// intersect returns the elements of a that also appear in b, preserving
// the order of a. Values are deduplicated. A "*" in either slice is
// treated as the full concrete action set (pull/push/delete).
func intersect(a, b []string) []string {
	if containsAll(a) {
		a = []string{ActionPull, ActionPush, ActionDelete}
	}
	if containsAll(b) {
		b = []string{ActionPull, ActionPush, ActionDelete}
	}
	bset := make(map[string]struct{}, len(b))
	for _, x := range b {
		bset[x] = struct{}{}
	}
	seen := make(map[string]struct{}, len(a))
	out := make([]string, 0, len(a))
	for _, x := range a {
		if _, ok := bset[x]; !ok {
			continue
		}
		if _, dup := seen[x]; dup {
			continue
		}
		seen[x] = struct{}{}
		out = append(out, x)
	}
	return out
}

func containsAll(xs []string) bool {
	for _, x := range xs {
		if x == ActionAll {
			return true
		}
	}
	return false
}

func dedupePreserveOrder(xs []string) []string {
	seen := make(map[string]struct{}, len(xs))
	out := make([]string, 0, len(xs))
	for _, x := range xs {
		if _, ok := seen[x]; ok {
			continue
		}
		seen[x] = struct{}{}
		out = append(out, x)
	}
	return out
}
