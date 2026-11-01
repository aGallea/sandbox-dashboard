package main

import (
	"log/slog"
	"reflect"
	"testing"

	"github.com/aGallea/sandbox-dashboard/internal/server"
)

// The chart sets the port through the environment so a single value can drive
// both the container port and the probes. A flag still wins, so an operator
// running the binary by hand is never overridden by a stale export.
func TestResolveListenAddr(t *testing.T) {
	tests := []struct {
		name string
		flag string
		env  string
		want string
	}{
		{"nothing set", "", "", ":8080"},
		{"env only", "", ":9090", ":9090"},
		{"flag only", ":7000", "", ":7000"},
		{"flag beats env", ":7000", ":9090", ":7000"},
		{"bare port from env is accepted", "", "9090", ":9090"},
		{"host and port", "", "127.0.0.1:9090", "127.0.0.1:9090"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := resolveListenAddr(tc.flag, tc.env); got != tc.want {
				t.Errorf("resolveListenAddr(%q, %q) = %q, want %q", tc.flag, tc.env, got, tc.want)
			}
		})
	}
}

func TestResolveWatchNamespaces(t *testing.T) {
	tests := []struct {
		name string
		flag string
		env  string
		want []string
	}{
		{"nothing set means every namespace", "", "", nil},
		{"single namespace", "default", "", []string{"default"}},
		{"comma separated", "default,team-a", "", []string{"default", "team-a"}},
		{"env only", "", "default", []string{"default"}},
		{"flag beats env", "team-a", "default", []string{"team-a"}},
		{"surrounding whitespace is trimmed", " default , team-a ", "", []string{"default", "team-a"}},
		// A trailing comma is the kind of thing a Helm range emits; it should not
		// become an empty namespace name, which would 403 at list time.
		{"blank entries are dropped", "default,,team-a,", "", []string{"default", "team-a"}},
		{"only separators means every namespace", ",  ,", "", nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := resolveWatchNamespaces(tc.flag, tc.env)
			if len(got) != len(tc.want) {
				t.Fatalf("resolveWatchNamespaces(%q, %q) = %v, want %v", tc.flag, tc.env, got, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Errorf("resolveWatchNamespaces(%q, %q)[%d] = %q, want %q", tc.flag, tc.env, i, got[i], tc.want[i])
				}
			}
		})
	}
}

func TestResolveLogLevel(t *testing.T) {
	tests := []struct {
		name, flag, env string
		want            slog.Level
		wantErr         bool
	}{
		{"default is info", "", "", slog.LevelInfo, false},
		{"flag wins", "debug", "error", slog.LevelDebug, false},
		{"env fallback", "", "WARN", slog.LevelWarn, false},
		{"garbage falls back to info and says so", "loud", "", slog.LevelInfo, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := resolveLogLevel(tc.flag, tc.env)
			if (err != nil) != tc.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tc.wantErr)
			}
			if got != tc.want {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
		})
	}
}

// A mistake in the setting fails startup: a column that silently went missing
// would read as a fleet that stamps nothing.
func TestResolveLabelColumns(t *testing.T) {
	owner := `[{"name":"Owner","labels":["example.com/user","owner"]}]`
	tests := []struct {
		name, flag, env string
		want            []server.LabelColumn
		wantErr         bool
	}{
		{"nothing set means no columns", "", "", nil, false},
		{"env only", "", owner, []server.LabelColumn{{Name: "Owner", Labels: []string{"example.com/user", "owner"}}}, false},
		{"flag beats env", `[{"name":"Team","labels":["team"]}]`, owner, []server.LabelColumn{{Name: "Team", Labels: []string{"team"}}}, false},
		{"not JSON", "Owner=owner", "", nil, true},
		{"a misspelt field", `[{"name":"Owner","label":["owner"]}]`, "", nil, true},
		{"a column with no name", `[{"labels":["owner"]}]`, "", nil, true},
		{"a column with no labels", `[{"name":"Owner","labels":[]}]`, "", nil, true},
		{"an empty label key", `[{"name":"Owner","labels":[""]}]`, "", nil, true},
		{"two columns with one name", `[{"name":"Owner","labels":["a"]},{"name":"Owner","labels":["b"]}]`, "", nil, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := resolveLabelColumns(tc.flag, tc.env)
			if (err != nil) != tc.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tc.wantErr)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("got %#v, want %#v", got, tc.want)
			}
		})
	}
}
