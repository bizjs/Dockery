package scope

import (
	"reflect"
	"testing"
)

func TestParse(t *testing.T) {
	cases := []struct {
		in      string
		want    Scope
		wantErr bool
	}{
		{"repository:alice/app:pull,push", Scope{TypeRepository, "alice/app", []string{"pull", "push"}}, false},
		{"repository:alice/app:*", Scope{TypeRepository, "alice/app", []string{"*"}}, false},
		{"registry:catalog:*", Scope{TypeRegistry, "catalog", []string{"*"}}, false},
		{"  repository : alice/app : pull , push  ",
			Scope{TypeRepository, "alice/app", []string{"pull", "push"}}, false},
		{"repository:alice/app:", Scope{TypeRepository, "alice/app", nil}, false},
		// Malformed
		{"", Scope{}, true},
		{"repository", Scope{}, true},
		{"repository:alice/app", Scope{}, true},
		{":alice/app:pull", Scope{}, true},
		{"repository::pull", Scope{}, true},
	}
	for _, c := range cases {
		got, err := Parse(c.in)
		if (err != nil) != c.wantErr {
			t.Errorf("Parse(%q) err=%v, wantErr=%v", c.in, err, c.wantErr)
			continue
		}
		if err == nil && !reflect.DeepEqual(got, c.want) {
			t.Errorf("Parse(%q) = %+v, want %+v", c.in, got, c.want)
		}
	}
}

func TestParseMany_SpaceJoined(t *testing.T) {
	ok, bad := ParseMany([]string{"repository:a:pull repository:b:push"})
	if len(bad) != 0 {
		t.Fatalf("unexpected bad: %v", bad)
	}
	if len(ok) != 2 || ok[0].Name != "a" || ok[1].Name != "b" {
		t.Fatalf("got %+v", ok)
	}
}

func TestParseMany_Mixed(t *testing.T) {
	ok, bad := ParseMany([]string{"repository:a:pull", "garbage", "registry:catalog:*"})
	if len(ok) != 2 {
		t.Fatalf("expected 2 ok, got %+v", ok)
	}
	if len(bad) != 1 || bad[0] != "garbage" {
		t.Fatalf("expected 1 bad 'garbage', got %v", bad)
	}
}

func TestMatchPattern(t *testing.T) {
	cases := []struct {
		pat, name string
		want      bool
	}{
		{"*", "anything", true},
		{"*", "alice/app", true},
		{"alice/app", "alice/app", true},
		{"alice/app", "alice/other", false},
		{"alice/*", "alice/app", true},
		{"alice/*", "alice/app/sub", false}, // different segment count
		{"alice/*", "bob/app", false},
		{"*/app", "alice/app", true},
		{"*/app", "alice/web", false},
		{"team/*/api", "team/alpha/api", true},
		{"team/*/api", "team/alpha", false},
	}
	for _, c := range cases {
		if got := MatchPattern(c.pat, c.name); got != c.want {
			t.Errorf("MatchPattern(%q, %q) = %v, want %v", c.pat, c.name, got, c.want)
		}
	}
}

func TestMatch_RegistryAdminOnly(t *testing.T) {
	sc := Scope{Type: TypeRegistry, Name: "catalog", Actions: []string{"*"}}
	if got := Match(RoleAdmin, nil, sc); !reflect.DeepEqual(got, []string{"*"}) {
		t.Errorf("admin registry: got %v", got)
	}
	if got := Match(RoleWrite, []string{"*"}, sc); len(got) != 0 {
		t.Errorf("write registry: expected nil, got %v", got)
	}
	if got := Match(RoleView, []string{"*"}, sc); len(got) != 0 {
		t.Errorf("view registry: expected nil, got %v", got)
	}
}

func TestMatch_AdminBypassesPatterns(t *testing.T) {
	sc := Scope{Type: TypeRepository, Name: "anywhere", Actions: []string{"pull", "push", "delete"}}
	got := Match(RoleAdmin, nil /* no patterns */, sc)
	want := []string{"pull", "push", "delete"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("admin bypass: got %v want %v", got, want)
	}
}

func TestMatch_WriteRole(t *testing.T) {
	// write has pull+push+delete on matched repos
	patterns := []string{"alice/*"}

	// match — pull+push granted
	sc := Scope{Type: TypeRepository, Name: "alice/app", Actions: []string{"pull", "push"}}
	got := Match(RoleWrite, patterns, sc)
	if !reflect.DeepEqual(got, []string{"pull", "push"}) {
		t.Errorf("got %v", got)
	}

	// match + delete requested — granted
	sc.Actions = []string{"delete"}
	got = Match(RoleWrite, patterns, sc)
	if !reflect.DeepEqual(got, []string{"delete"}) {
		t.Errorf("got %v", got)
	}

	// no-match — empty
	sc = Scope{Type: TypeRepository, Name: "bob/app", Actions: []string{"pull"}}
	if got := Match(RoleWrite, patterns, sc); len(got) != 0 {
		t.Errorf("no-match write: got %v", got)
	}
}

func TestMatch_ViewRoleOnlyPull(t *testing.T) {
	patterns := []string{"shared/*"}
	// pull requested → granted
	sc := Scope{Type: TypeRepository, Name: "shared/app", Actions: []string{"pull"}}
	if got := Match(RoleView, patterns, sc); !reflect.DeepEqual(got, []string{"pull"}) {
		t.Errorf("view pull: got %v", got)
	}
	// push requested on matched repo → NOT granted (view can't push)
	sc.Actions = []string{"pull", "push"}
	if got := Match(RoleView, patterns, sc); !reflect.DeepEqual(got, []string{"pull"}) {
		t.Errorf("view pull+push: got %v want [pull]", got)
	}
	sc.Actions = []string{"push"}
	if got := Match(RoleView, patterns, sc); len(got) != 0 {
		t.Errorf("view push only: expected empty, got %v", got)
	}
}

func TestMatch_WildcardExpansion(t *testing.T) {
	patterns := []string{"*"}
	// write user asks "*" actions on matched repo → gets pull+push+delete
	sc := Scope{Type: TypeRepository, Name: "foo", Actions: []string{"*"}}
	got := Match(RoleWrite, patterns, sc)
	want := []string{"pull", "push", "delete"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v want %v", got, want)
	}
}

func TestMatch_DeduplicatesActions(t *testing.T) {
	sc := Scope{Type: TypeRepository, Name: "foo", Actions: []string{"pull", "pull", "push"}}
	got := Match(RoleAdmin, nil, sc)
	if !reflect.DeepEqual(got, []string{"pull", "push"}) {
		t.Errorf("dedupe: got %v", got)
	}
}
