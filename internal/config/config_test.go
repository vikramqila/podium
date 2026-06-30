package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadSampleConfig(t *testing.T) {
	cfg, err := Load(filepath.Join("..", "..", "gateway.yaml"))
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.Gateway.Port != 8080 {
		t.Fatalf("Gateway.Port = %d, want 8080", cfg.Gateway.Port)
	}
	if got := len(cfg.Gateway.Routes); got != 5 {
		t.Fatalf("len(Gateway.Routes) = %d, want 5", got)
	}
	if cfg.Gateway.Routes[2].Upstream.Targets[0].Weight != 3 {
		t.Fatalf("first products target weight = %d, want 3", cfg.Gateway.Routes[2].Upstream.Targets[0].Weight)
	}
	if cfg.Gateway.Routes[4].Auth == nil || cfg.Gateway.Routes[4].Auth.Header != "X-API-Key" {
		t.Fatalf("internal route auth was not parsed correctly")
	}
}

func TestLoadRejectsInvalidConfig(t *testing.T) {
	tests := []struct {
		name    string
		yaml    string
		wantErr string
	}{
		{
			name: "missing routes",
			yaml: `
gateway:
  port: 8080
`,
			wantErr: "gateway.routes must contain at least one route",
		},
		{
			name: "invalid duration",
			yaml: `
gateway:
  port: 8080
  routes:
    - path: /api/users
      methods: [GET]
      timeout: definitely-not-a-duration
      upstream:
        url: http://localhost:3001
`,
			wantErr: "gateway.routes[0].timeout must be a valid duration",
		},
		{
			name: "missing upstream",
			yaml: `
gateway:
  port: 8080
  routes:
    - path: /api/users
      methods: [GET]
      upstream: {}
`,
			wantErr: "gateway.routes[0].upstream must define url or targets",
		},
		{
			name: "unsupported rate limit strategy",
			yaml: `
gateway:
  port: 8080
  routes:
    - path: /api/users
      methods: [GET]
      upstream:
        url: http://localhost:3001
      rate_limit:
        requests: 1
        window: 1s
        strategy: token_bucket
        per: ip
`,
			wantErr: "gateway.routes[0].rate_limit.strategy must be fixed_window or sliding_window",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := writeTempConfig(t, tt.yaml)

			_, err := Load(path)
			if err == nil {
				t.Fatal("Load() error = nil, want error")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("Load() error = %q, want it to contain %q", err.Error(), tt.wantErr)
			}
		})
	}
}

func writeTempConfig(t *testing.T, content string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "gateway.yaml")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write temp config: %v", err)
	}
	return path
}
