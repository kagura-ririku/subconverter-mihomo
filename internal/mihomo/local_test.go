package mihomo

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/kagura-ririku/subconverter-mihomo/internal/config"
	mihomoage "github.com/metacubex/mihomo/component/age"
	"go.yaml.in/yaml/v3"
)

func TestWriteRuntimeConfigSupportsLocalProvider(t *testing.T) {
	home := t.TempDir()
	sourceDir := t.TempDir()
	secretKey, _, err := mihomoage.GenX25519KeyPair()
	if err != nil {
		t.Fatal(err)
	}
	sourcePath := filepath.Join(sourceDir, "local.yaml")
	first := []byte("proxies:\n  - name: Local One\n    type: ss\n    server: 127.0.0.1\n    port: 443\n    cipher: aes-128-gcm\n    password: test\n")
	if err := os.WriteFile(sourcePath, first, 0o644); err != nil {
		t.Fatal(err)
	}

	subscription := config.Subscription{
		Upstreams: []config.Upstream{
			{
				Name:         "remote",
				URL:          "https://example.com/subscription",
				UserAgent:    "MetaCubeX/mihomo",
				AgeSecretKey: secretKey,
				ProviderName: "remote-provider",
			},
			{
				Name:         "local",
				Path:         "upstreams/local.yaml",
				LocalPath:    sourcePath,
				ProviderName: "local-provider",
			},
		},
	}
	cfg := &config.Config{
		MihomoHome:    home,
		Subscriptions: []config.Subscription{subscription},
	}

	if err := WriteRuntimeConfig(cfg); err != nil {
		t.Fatalf("WriteRuntimeConfig() error = %v", err)
	}
	runtimeContent, err := os.ReadFile(filepath.Join(home, "config.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	decoded := struct {
		Providers   map[string]map[string]any `yaml:"proxy-providers"`
		ProxyGroups []map[string]any          `yaml:"proxy-groups"`
	}{}
	if err := yaml.Unmarshal(runtimeContent, &decoded); err != nil {
		t.Fatal(err)
	}
	if got := decoded.Providers["remote-provider"]["type"]; got != "http" {
		t.Fatalf("remote provider type = %v, want http", got)
	}
	if got := decoded.Providers["remote-provider"]["size-limit"]; got != providerDownloadSizeLimit {
		t.Fatalf("remote provider size-limit = %v, want %d", got, providerDownloadSizeLimit)
	}
	if got := decoded.Providers["remote-provider"]["proxy"]; got != "DIRECT" {
		t.Fatalf("remote provider proxy = %v, want DIRECT", got)
	}
	if got := decoded.Providers["remote-provider"]["age-secret-key"]; got != secretKey {
		t.Fatalf("remote provider age-secret-key = %v, want generated secret key", got)
	}
	if got := decoded.Providers["local-provider"]["type"]; got != "file" {
		t.Fatalf("local provider type = %v, want file", got)
	}
	if got := decoded.Providers["local-provider"]["path"]; got != "providers/local-provider.yaml" {
		t.Fatalf("local provider path = %v, want providers/local-provider.yaml", got)
	}

	target := filepath.Join(home, "providers", "local-provider.yaml")
	if got, err := os.ReadFile(target); err != nil || !bytes.Equal(got, first) {
		t.Fatalf("initial local provider = %q, error = %v", got, err)
	}

	second := []byte("proxies:\n  - name: Local Two\n    type: ss\n    server: 127.0.0.2\n    port: 443\n    cipher: aes-128-gcm\n    password: test\n")
	if err := os.WriteFile(sourcePath, second, 0o644); err != nil {
		t.Fatal(err)
	}
	if failures := SyncLocalProviders(cfg, &subscription); len(failures) != 0 {
		t.Fatalf("SyncLocalProviders() failures = %v", failures)
	}
	if got, err := os.ReadFile(target); err != nil || !bytes.Equal(got, second) {
		t.Fatalf("updated local provider = %q, error = %v", got, err)
	}
}

func TestWriteRuntimeConfigToleratesInvalidLocalProvider(t *testing.T) {
	home := t.TempDir()
	sourcePath := filepath.Join(t.TempDir(), "invalid.yaml")
	if err := os.WriteFile(sourcePath, []byte("proxies: [\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg := &config.Config{
		MihomoHome: home,
		Subscriptions: []config.Subscription{{
			Upstreams: []config.Upstream{{
				Path:         "upstreams/invalid.yaml",
				LocalPath:    sourcePath,
				ProviderName: "invalid-local",
			}},
		}},
	}

	if err := WriteRuntimeConfig(cfg); err != nil {
		t.Fatalf("WriteRuntimeConfig() error = %v", err)
	}
	cache, err := os.ReadFile(filepath.Join(home, "providers", "invalid-local.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if string(cache) != providerPlaceholderYAML {
		t.Fatalf("cache = %q, want placeholder provider", cache)
	}
	failures := SyncLocalProviders(cfg, &cfg.Subscriptions[0])
	if failures["invalid-local"] == nil {
		t.Fatalf("failures = %#v, want invalid-local error", failures)
	}
}

func TestValidateProviderNameRejectsPathTraversal(t *testing.T) {
	for _, name := range []string{"../provider", `..\\provider`, "/tmp/provider", "default"} {
		if err := validateProviderName(name); err == nil {
			t.Errorf("validateProviderName(%q) = nil, want error", name)
		}
	}
	if err := validateProviderName("provider_1-2"); err != nil {
		t.Fatalf("validateProviderName(valid) error = %v", err)
	}
}
func TestParseRawProviderAcceptsFullMihomoConfig(t *testing.T) {
	content := []byte(`mixed-port: 7890
mode: rule
proxies:
  - name: Local One
    type: ss
    server: 127.0.0.1
    port: 443
    cipher: aes-128-gcm
    password: test
proxy-groups:
  - name: PROXY
    type: select
    proxies:
      - Local One
rules:
  - MATCH,PROXY
`)

	proxies, err := parseRawProvider(content)
	if err != nil {
		t.Fatalf("parseRawProvider() error = %v", err)
	}
	if len(proxies) != 1 || proxies[0]["name"] != "Local One" {
		t.Fatalf("parseRawProvider() proxies = %#v, want Local One", proxies)
	}
}
