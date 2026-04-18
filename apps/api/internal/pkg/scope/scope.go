// Package scope parses and matches Docker Registry token-auth scope
// strings as specified at
// https://distribution.github.io/distribution/spec/auth/scope/.
//
// A scope has the form:
//
//	<type>:<name>:<action>[,<action>]*
//
// Examples:
//
//	repository:alice/app:pull,push
//	repository:alice/app:*
//	registry:catalog:*
package scope

import (
	"errors"
	"strings"
)

// Type enumerates the resource categories Dockery recognises.
const (
	TypeRepository = "repository"
	TypeRegistry   = "registry"
)

// Common action constants (string) — we keep these untyped because
// Docker CLI and the distribution server pass them as plain strings.
const (
	ActionPull   = "pull"
	ActionPush   = "push"
	ActionDelete = "delete"
	ActionAll    = "*"
)

// ErrMalformed is returned by Parse when the input does not match
// the canonical <type>:<name>:<actions> shape.
var ErrMalformed = errors.New("scope: malformed scope string")

// Scope is the parsed form of a single scope string.
type Scope struct {
	Type    string
	Name    string
	Actions []string
}

// String renders the scope back into its canonical wire form.
func (s Scope) String() string {
	return s.Type + ":" + s.Name + ":" + strings.Join(s.Actions, ",")
}

// Parse decodes one scope string. Whitespace around fields is trimmed.
// Empty action lists are allowed (a token server may legitimately return
// a scope with zero granted actions).
//
// Note: <name> may itself contain colons (image tags include ":" and
// digests use "sha256:..."), so we split into at most three parts from
// the LEFT and keep the trailing portion as actions. For registry
// scopes the shape is e.g. "registry:catalog:*", which still parses
// cleanly because the first two colons are the delimiters.
func Parse(s string) (Scope, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return Scope{}, ErrMalformed
	}

	// Split into exactly three sections: type, name, actions.
	// We use SplitN(s, ":", 3) then treat everything after the 2nd
	// colon as the actions list. But that breaks names containing
	// colons. Registry tokens never put ":" in a scope name (names are
	// image refs like "alice/app" — colons only appear in tags which
	// aren't part of the scope name). Simple three-part split is fine.
	parts := strings.SplitN(s, ":", 3)
	if len(parts) != 3 {
		return Scope{}, ErrMalformed
	}
	typ := strings.TrimSpace(parts[0])
	name := strings.TrimSpace(parts[1])
	actionsField := strings.TrimSpace(parts[2])
	if typ == "" || name == "" {
		return Scope{}, ErrMalformed
	}

	var actions []string
	if actionsField != "" {
		raw := strings.Split(actionsField, ",")
		actions = make([]string, 0, len(raw))
		for _, a := range raw {
			a = strings.TrimSpace(a)
			if a != "" {
				actions = append(actions, a)
			}
		}
	}

	return Scope{Type: typ, Name: name, Actions: actions}, nil
}

// ParseMany parses a space-separated list or a slice of scope strings.
// Malformed entries are returned in the second slice so the caller can
// decide whether to reject the request or ignore bad ones.
func ParseMany(scopes []string) (ok []Scope, bad []string) {
	for _, raw := range scopes {
		// Scopes can also arrive space-joined in a single string.
		for _, s := range strings.Fields(raw) {
			parsed, err := Parse(s)
			if err != nil {
				bad = append(bad, s)
				continue
			}
			ok = append(ok, parsed)
		}
	}
	return
}
