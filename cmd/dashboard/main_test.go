package main

import "testing"

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
