package render

import (
	"strings"
	"testing"

	"github.com/kagura-ririku/subconverter-mihomo/internal/model"
	"github.com/kagura-ririku/subconverter-mihomo/internal/remoteconfig"
)

func TestClashReturnsYAMLMarshalError(t *testing.T) {
	_, err := Clash(&remoteconfig.Compiled{
		BaseExtras: map[string]any{
			"unsupported": make(chan struct{}),
		},
	}, nil)
	if err == nil || !strings.Contains(err.Error(), "cannot marshal") {
		t.Fatalf("Clash() error = %v, want YAML marshal error", err)
	}
}

func TestClashRendersEmptyNodeLists(t *testing.T) {
	body, err := Clash(&remoteconfig.Compiled{
		BaseExtras:  map[string]any{},
		ProxyGroups: []map[string]any{{"name": "PROXY", "type": "select", "proxies": []string{"DIRECT"}}},
		Rules:       []string{"MATCH,PROXY"},
	}, []model.Node{})
	if err != nil {
		t.Fatalf("Clash() error = %v", err)
	}
	if !strings.Contains(string(body), "proxies: []") {
		t.Fatalf("Clash() output = %q, want empty proxies list", body)
	}
}
