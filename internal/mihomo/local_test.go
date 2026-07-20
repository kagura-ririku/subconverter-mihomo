package mihomo

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/kagura-ririku/subconverter-mihomo/internal/config"
	"gopkg.in/yaml.v3"
)

func TestWriteRuntimeConfigSupportsLocalProvider(t *testing.T) {
	home := t.TempDir()
	sourceDir := t.TempDir()
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
		Providers map[string]map[string]any `yaml:"proxy-providers"`
	}{}
	if err := yaml.Unmarshal(runtimeContent, &decoded); err != nil {
		t.Fatal(err)
	}
	if got := decoded.Providers["remote-provider"]["type"]; got != "http" {
		t.Fatalf("remote provider type = %v, want http", got)
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
	if err := SyncLocalProviders(cfg, &subscription); err != nil {
		t.Fatalf("SyncLocalProviders() error = %v", err)
	}
	if got, err := os.ReadFile(target); err != nil || !bytes.Equal(got, second) {
		t.Fatalf("updated local provider = %q, error = %v", got, err)
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
