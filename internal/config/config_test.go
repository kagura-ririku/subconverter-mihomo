package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
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

func TestPrepareSubscriptionsAllowsMissingLocalPath(t *testing.T) {
	configDir := t.TempDir()
	canonicalConfigDir, err := filepath.EvalSymlinks(configDir)
	if err != nil {
		t.Fatal(err)
	}
	missingPath := filepath.Join(canonicalConfigDir, "upstreams", "not-yet-created.yaml")

	subscriptions, err := prepareSubscriptions([]Subscription{{
		UUID: "11111111-1111-1111-1111-111111111111",
		Upstreams: []Upstream{{
			Path: "upstreams/not-yet-created.yaml",
		}},
	}}, configDir)
	if err != nil {
		t.Fatalf("prepareSubscriptions() error = %v, want missing local path to be deferred", err)
	}
	upstream := subscriptions[0].Upstreams[0]
	if upstream.LocalPath != missingPath {
		t.Fatalf("LocalPath = %q, want %q", upstream.LocalPath, missingPath)
	}
}

func TestPrepareSubscriptionsRejectsMissingLocalPathThroughOutsideSymlink(t *testing.T) {
	root := t.TempDir()
	configDir := filepath.Join(root, "config")
	outsideDir := filepath.Join(root, "outside")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(outsideDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outsideDir, filepath.Join(configDir, "linked")); err != nil {
		t.Fatal(err)
	}

	_, err := prepareSubscriptions([]Subscription{{
		UUID: "11111111-1111-1111-1111-111111111111",
		Upstreams: []Upstream{{
			Path: "linked/not-yet-created.yaml",
		}},
	}}, configDir)
	if err == nil || !strings.Contains(err.Error(), "must stay inside") {
		t.Fatalf("prepareSubscriptions() error = %v, want path containment error", err)
	}
}

func TestMustDurationSecondsUsesFallbackForNonPositiveValues(t *testing.T) {
	for _, value := range []string{"0", "not-a-number", "9999999999"} {
		t.Run(value, func(t *testing.T) {
			t.Setenv("SUBCONVERTER_TEST_TIMEOUT_SECONDS", value)
			if got := mustDurationSeconds("TEST_TIMEOUT_SECONDS", 20); got != 20*time.Second {
				t.Fatalf("mustDurationSeconds() = %v, want %v", got, 20*time.Second)
			}
		})
	}

	t.Setenv("SUBCONVERTER_TEST_TIMEOUT_SECONDS", "3")
	if got := mustDurationSeconds("TEST_TIMEOUT_SECONDS", 20); got != 3*time.Second {
		t.Fatalf("mustDurationSeconds() = %v, want %v", got, 3*time.Second)
	}
}
