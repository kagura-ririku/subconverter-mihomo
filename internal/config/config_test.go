package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPrepareSubscriptionsSupportsLocalYAML(t *testing.T) {
	configDir := t.TempDir()
	upstreamsDir := filepath.Join(configDir, "upstreams")
	if err := os.MkdirAll(upstreamsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	localPath := filepath.Join(upstreamsDir, "local.yaml")
	if err := os.WriteFile(localPath, []byte("proxies: []\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	subscriptions, err := prepareSubscriptions([]Subscription{{
		UUID: "11111111-1111-1111-1111-111111111111",
		Name: "local",
		Upstreams: []Upstream{{
			Name: "file",
			Path: "upstreams/local.yaml",
		}},
	}}, configDir)
	if err != nil {
		t.Fatalf("prepareSubscriptions() error = %v", err)
	}
	upstream := subscriptions[0].Upstreams[0]
	if !upstream.IsLocal() {
		t.Fatal("local upstream was not marked as local")
	}
	expectedPath, err := filepath.EvalSymlinks(localPath)
	if err != nil {
		t.Fatal(err)
	}
	if upstream.LocalPath != expectedPath {
		t.Fatalf("LocalPath = %q, want %q", upstream.LocalPath, expectedPath)
	}
	if upstream.UserAgent != "" {
		t.Fatalf("local upstream UserAgent = %q, want empty", upstream.UserAgent)
	}
}

func TestPrepareSubscriptionsRequiresExactlyOneUpstreamSource(t *testing.T) {
	tests := []struct {
		name     string
		upstream Upstream
		want     string
	}{
		{name: "missing", upstream: Upstream{}, want: "must contain url or path"},
		{name: "both", upstream: Upstream{URL: "https://example.com/sub", Path: "local.yaml"}, want: "cannot contain both url and path"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := prepareSubscriptions([]Subscription{{
				UUID:      "11111111-1111-1111-1111-111111111111",
				Upstreams: []Upstream{tt.upstream},
			}}, t.TempDir())
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("prepareSubscriptions() error = %v, want containing %q", err, tt.want)
			}
		})
	}
}

func TestPrepareSubscriptionsRejectsLocalPathOutsideConfigDirectory(t *testing.T) {
	root := t.TempDir()
	configDir := filepath.Join(root, "config")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "outside.yaml"), []byte("proxies: []\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := prepareSubscriptions([]Subscription{{
		UUID: "11111111-1111-1111-1111-111111111111",
		Upstreams: []Upstream{{
			Path: "../outside.yaml",
		}},
	}}, configDir)
	if err == nil || !strings.Contains(err.Error(), "must stay inside") {
		t.Fatalf("prepareSubscriptions() error = %v, want path containment error", err)
	}
}

func TestPrepareSubscriptionsRejectsLocalSymlinkOutsideConfigDirectory(t *testing.T) {
	root := t.TempDir()
	configDir := filepath.Join(root, "config")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(root, "outside.yaml")
	if err := os.WriteFile(outside, []byte("proxies: []\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(configDir, "linked.yaml")); err != nil {
		t.Fatal(err)
	}

	_, err := prepareSubscriptions([]Subscription{{
		UUID: "11111111-1111-1111-1111-111111111111",
		Upstreams: []Upstream{{
			Path: "linked.yaml",
		}},
	}}, configDir)
	if err == nil || !strings.Contains(err.Error(), "must stay inside") {
		t.Fatalf("prepareSubscriptions() error = %v, want path containment error", err)
	}
}
