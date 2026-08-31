package mihomo

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kagura-ririku/subconverter-mihomo/internal/config"
	mihomoage "github.com/metacubex/mihomo/component/age"
)

func TestLoadRawProvidersIsolatesInvalidCaches(t *testing.T) {
	home := t.TempDir()
	directory := filepath.Join(home, "providers")
	if err := os.MkdirAll(directory, 0o755); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		"valid.yaml":   "proxies:\n  - name: valid\n    type: ss\n",
		"invalid.yaml": "proxies: [\n",
		"empty.yaml":   "proxies: []\n",
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(directory, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	providers, failures := LoadRawProviders(&config.Config{MihomoHome: home}, []string{"valid", "invalid", "empty", "missing"})
	if len(providers) != 1 || len(providers["valid"]) != 1 {
		t.Fatalf("providers = %#v, want only valid provider", providers)
	}
	for _, name := range []string{"invalid", "empty", "missing"} {
		if failures[name] == nil {
			t.Fatalf("failures = %#v, want %s failure", failures, name)
		}
	}
}

func TestLoadRawProvidersRejectsOversizedCache(t *testing.T) {
	home := t.TempDir()
	directory := filepath.Join(home, "providers")
	if err := os.MkdirAll(directory, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(directory, "oversized.yaml")
	if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Truncate(path, maxProviderSize+1); err != nil {
		t.Fatal(err)
	}

	providers, failures := LoadRawProviders(&config.Config{MihomoHome: home}, []string{"oversized"})
	if len(providers) != 0 || !strings.Contains(failures["oversized"].Error(), "size limit") {
		t.Fatalf("providers/failure = %#v/%v, want size-limit failure", providers, failures["oversized"])
	}
}

func TestParseRawProviderDecryptsAgeArmor(t *testing.T) {
	secretKey, publicKey, err := mihomoage.GenX25519KeyPair()
	if err != nil {
		t.Fatal(err)
	}
	encrypted, err := mihomoage.EncryptBytes([]byte("proxies:\n  - name: encrypted\n    type: ss\n"), publicKey)
	if err != nil {
		t.Fatal(err)
	}

	proxies, err := parseRawProvider(encrypted, secretKey)
	if err != nil || len(proxies) != 1 || proxies[0]["name"] != "encrypted" {
		t.Fatalf("parseRawProvider() = %#v, error = %v", proxies, err)
	}
}
