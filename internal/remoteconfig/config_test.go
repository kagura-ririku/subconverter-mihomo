package remoteconfig

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/kagura-ririku/subconverter-mihomo/internal/config"
	"github.com/kagura-ririku/subconverter-mihomo/internal/model"
)

func TestReadResourceRejectsOversizedResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(strings.Repeat("x", maxRemoteResourceSize+1)))
	}))
	defer server.Close()

	_, _, err := readResource(context.Background(), &config.Config{}, server.URL, "")
	if err == nil || !strings.Contains(err.Error(), "response exceeds") {
		t.Fatalf("readResource() error = %v, want size limit error", err)
	}
}

func TestMigrateGlobalClientFingerprint(t *testing.T) {
	baseExtras := map[string]any{
		"global-client-fingerprint": " chrome ",
		"mode":                      "rule",
	}
	supportedTypes := []string{
		"ss",
		"vmess",
		"vless",
		"trojan",
		"anytls",
		"snell",
		"trusttunnel",
		"gost-relay",
	}
	nodes := make([]model.Node, 0, len(supportedTypes)+3)
	for _, proxyType := range supportedTypes {
		nodes = append(nodes, model.Node{Proxy: map[string]any{"type": proxyType}})
	}
	nodes = append(nodes,
		model.Node{Proxy: map[string]any{"type": "VMESS"}},
		model.Node{Proxy: map[string]any{"type": "hysteria2"}},
		model.Node{Proxy: map[string]any{
			"type":               "trojan",
			"client-fingerprint": "firefox",
		}},
	)

	migrateGlobalClientFingerprint(baseExtras, nodes)

	if _, exists := baseExtras["global-client-fingerprint"]; exists {
		t.Fatal("global-client-fingerprint was not removed from base config")
	}
	if baseExtras["mode"] != "rule" {
		t.Fatalf("unrelated base config changed: %#v", baseExtras)
	}
	for index, proxyType := range supportedTypes {
		if got := nodes[index].Proxy["client-fingerprint"]; got != "chrome" {
			t.Errorf("%s client-fingerprint = %#v, want chrome", proxyType, got)
		}
	}
	if got := nodes[len(supportedTypes)].Proxy["client-fingerprint"]; got != "chrome" {
		t.Errorf("uppercase proxy type client-fingerprint = %#v, want chrome", got)
	}
	if got := nodes[len(supportedTypes)+1].Proxy["client-fingerprint"]; got != nil {
		t.Errorf("unsupported proxy client-fingerprint = %#v, want nil", got)
	}
	if got := nodes[len(supportedTypes)+2].Proxy["client-fingerprint"]; got != "firefox" {
		t.Errorf("existing client-fingerprint = %#v, want firefox", got)
	}
}

func TestMigrateGlobalClientFingerprintRemovesInvalidValue(t *testing.T) {
	for _, value := range []any{"  ", 123} {
		baseExtras := map[string]any{"global-client-fingerprint": value}
		node := model.Node{Proxy: map[string]any{"type": "vmess"}}

		migrateGlobalClientFingerprint(baseExtras, []model.Node{node})

		if _, exists := baseExtras["global-client-fingerprint"]; exists {
			t.Fatalf("global-client-fingerprint %#v was not removed", value)
		}
		if got := node.Proxy["client-fingerprint"]; got != nil {
			t.Fatalf("invalid global value %#v produced client-fingerprint %#v", value, got)
		}
	}
}

func TestMigrateGlobalClientFingerprintIgnoresNilProxy(t *testing.T) {
	baseExtras := map[string]any{"global-client-fingerprint": "chrome"}

	// Malformed node data should be skipped rather than crashing compilation.
	migrateGlobalClientFingerprint(baseExtras, []model.Node{{}})

	if _, exists := baseExtras["global-client-fingerprint"]; exists {
		t.Fatal("global-client-fingerprint was not removed")
	}
}

func TestCompileProxyGroupsKeepsCurrentBuiltInProxies(t *testing.T) {
	specs := []string{
		"Built-ins`select`[]DIRECT`[]REJECT`[]REJECT-DROP`[]COMPATIBLE`[]PASS`[]PASS-RULE`[]GLOBAL",
	}

	groups := compileProxyGroups(specs, nil)
	if len(groups) != 1 {
		t.Fatalf("compileProxyGroups() = %#v, want one group", groups)
	}
	want := []string{"DIRECT", "REJECT", "REJECT-DROP", "COMPATIBLE", "PASS", "PASS-RULE", "GLOBAL"}
	got, _ := groups[0]["proxies"].([]string)
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("group proxies = %v, want %v", got, want)
	}
}

func TestInferRuleFormatHandlesMRSURLs(t *testing.T) {
	tests := []struct {
		name   string
		source string
		want   string
	}{
		{name: "plain", source: "https://example.com/rules.mrs", want: "mrs"},
		{name: "query-and-fragment", source: "https://example.com/rules.MRS?download=1#latest", want: "mrs"},
		{name: "yaml-query", source: "https://example.com/rules.YML?download=1", want: "yaml"},
		{name: "text", source: "https://example.com/rules.txt?download=1", want: "text"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := inferRuleFormat(tt.source); got != tt.want {
				t.Fatalf("inferRuleFormat(%q) = %q, want %q", tt.source, got, tt.want)
			}
		})
	}
}

func TestResolveRuleSourceUsesMRSAndExactSizeLimit(t *testing.T) {
	resolved, err := resolveRuleSource(context.Background(), &config.Config{}, "", RuleSpec{
		Group:  "Domestic",
		Kind:   "clash-domain",
		Source: "https://example.com/rules.mrs?download=1",
	})
	if err != nil {
		t.Fatalf("resolveRuleSource() error = %v", err)
	}
	if got := resolved.provider["format"]; got != "mrs" {
		t.Fatalf("provider format = %#v, want mrs", got)
	}
	if got := resolved.provider["size-limit"]; got != maxRemoteResourceSize {
		t.Fatalf("provider size-limit = %#v, want %d", got, maxRemoteResourceSize)
	}
}
