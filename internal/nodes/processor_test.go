package nodes

import (
	"testing"

	"github.com/kagura-ririku/subconverter-mihomo/internal/config"
	"github.com/kagura-ririku/subconverter-mihomo/internal/model"
)

func TestBuildSkipsNoticeNodes(t *testing.T) {
	tests := []struct {
		name string
		want int
	}{
		{name: "剩余流量： 487.73 GB"},
		{name: "📊 剩余流量: 100 GiB"},
		{name: "剩余流量：无限制"},
		{name: "📅 套餐到期：2026-12-01"},
		{name: "流量重置时间：每月1日"},
		{name: "距离下次重置剩余：20 天"},
		{name: "在线设备"},
		{name: "在线设备：2 / 5"},
		{name: "📱 在线设备数：3"},
		{name: "节点 | 在线设备3台"},
		{name: "🌐 官网：https://example.com"},
		{name: "备用网址 example.com"},
		{name: "官方网站"},
		{name: "客服：@support"},
		{name: "电报群：https://t.me/example"},
		{name: "公告：请更新客户端"},
		{name: "订阅地址：https://example.com/sub"},
		{name: "请更新订阅获取最新节点"},
		{name: "套餐已到期，请续费"},
		{name: "机场名称 | 官网：example.com"},
		{name: "https://example.com"},
		{name: "🌐 HTTPS://example.com"},
		{name: "   "},
		{name: "🇭🇰 香港01|流量优化", want: 1},
		{name: "🇬🇧英国伦敦01|流媒体|0.1x", want: 1},
		{name: "Official Website: https://example.com", want: 1},
		{name: "Remaining Traffic: 20 GB", want: 1},
		{name: "Support Relay 01", want: 1},
		{name: "📱 在線設備數：3", want: 1},
		{name: "📊 剩餘流量: 100 GiB", want: 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for _, proxyType := range []string{"ss", "vless"} {
				got := Build(map[string][]map[string]any{
					"source": {{"name": tt.name, "type": proxyType, "server": "example.com", "port": 443}},
				})
				if len(got) != tt.want {
					t.Fatalf("Build() for %s returned %d nodes, want %d", proxyType, len(got), tt.want)
				}
				if tt.want == 1 && got[0].OriginalName != tt.name {
					t.Fatalf("Build() changed name to %q, want %q", got[0].OriginalName, tt.name)
				}
			}
		})
	}
}

func TestMatchKeywordUsesTokenBoundariesForLatinKeywords(t *testing.T) {
	tests := []struct {
		name    string
		keyword string
		want    bool
	}{
		{name: "USA Los Angeles 01", keyword: "NG", want: false},
		{name: "NG-01 Lagos", keyword: "NG", want: true},
		{name: "business-us-node", keyword: "US", want: true},
		{name: "business node", keyword: "US", want: false},
		{name: "[高级] TW01", keyword: "TW", want: true},
		{name: "[基础] TWICE", keyword: "TW", want: false},
		{name: "USA Los Angeles 01", keyword: "Los Angeles", want: true},
	}

	for _, tt := range tests {
		if got := matchKeyword(tt.name, tt.keyword); got != tt.want {
			t.Fatalf("matchKeyword(%q, %q) = %v, want %v", tt.name, tt.keyword, got, tt.want)
		}
	}
}

func TestExtractRegionInfoPrefersCityBeforeCountry(t *testing.T) {
	cfg := &config.Config{
		CitylessCountries: map[string]struct{}{},
		RegionKeywords: map[string][]string{
			"NGA": {"NG", "NGA", "Nigeria"},
			"TWN": {"TW", "TWN", "Taiwan"},
			"USA": {"US", "USA", "America"},
		},
		CityKeywords: []config.CityKeyword{
			{Country: "USA", City: "Los Angeles", Keywords: []string{"Los Angeles"}},
		},
	}

	info := extractRegionInfo("NG Relay Los Angeles 01", cfg)
	if info.Country != "USA" || info.City != "Los Angeles" {
		t.Fatalf("extractRegionInfo country/city = %s/%s, want USA/Los Angeles", info.Country, info.City)
	}
}

func TestExtractRegionInfoMatchesCountryAnywhere(t *testing.T) {
	cfg := &config.Config{
		CitylessCountries: map[string]struct{}{},
		RegionKeywords: map[string][]string{
			"NGA": {"NG", "NGA", "Nigeria"},
			"TWN": {"🇹🇼", "台湾", "TW", "TWN", "Taiwan"},
			"USA": {"🇺🇸", "美国", "US", "USA", "America"},
		},
		CityKeywords: []config.CityKeyword{},
	}

	tests := []struct {
		name string
		want string
	}{
		{name: "NG-01 Lagos", want: "NGA"},
		{name: "TW01 0.2x", want: "TWN"},
		{name: "🇨🇳 TW01 0.2x", want: "TWN"},
		{name: "🇨🇳-TW01 0.2x", want: "TWN"},
		{name: "TWICE node", want: "Unknown"},
		{name: "USA Los Angeles 01", want: "USA"},
		{name: "Relay NG 01", want: "NGA"},
		{name: "[高级] 🇨🇳 台湾 01", want: "TWN"},
		{name: "[基础] 🇺🇸 01", want: "USA"},
		{name: "🇹🇼 美国 01", want: "USA"},
		{name: "business node", want: "Unknown"},
		{name: "Los Angeles 01", want: "Unknown"},
	}

	for _, tt := range tests {
		if got := extractRegionInfo(tt.name, cfg).Country; got != tt.want {
			t.Fatalf("extractRegionInfo(%q).Country = %s, want %s", tt.name, got, tt.want)
		}
	}
}

func TestExtractRegionInfoPrefersLongestCountryKeyword(t *testing.T) {
	cfg := &config.Config{
		RegionOrder:       []string{"SGP", "IND", "IDN", "NZL"},
		CitylessCountries: map[string]struct{}{},
		RegionKeywords: map[string][]string{
			"SGP": {"新"},
			"IND": {"印度"},
			"IDN": {"印度尼西亚", "印度尼西亞"},
			"NZL": {"新西兰", "紐西蘭"},
		},
		CityKeywords: []config.CityKeyword{},
	}

	tests := []struct {
		name string
		want string
	}{
		{name: "印度尼西亚 01", want: "IDN"},
		{name: "印度 01", want: "IND"},
		{name: "新西兰 01", want: "NZL"},
		{name: "[高级] 印度尼西亚 01", want: "IDN"},
	}

	for _, tt := range tests {
		if got := extractRegionInfo(tt.name, cfg).Country; got != tt.want {
			t.Fatalf("extractRegionInfo(%q).Country = %s, want %s", tt.name, got, tt.want)
		}
	}
}

func TestApplyUsesDefaultPrimaryRegionSortBeforeExtraRegions(t *testing.T) {
	t.Setenv("SUBCONVERTER_SUBSCRIPTIONS_JSON", `[{
		"uuid":"11111111-1111-1111-1111-111111111111",
		"name":"test",
		"remote_config":"https://example.com/config.ini",
		"enable_node_rename":true,
		"sort_nodes_by_region":true,
		"upstreams":[{"name":"source","url":"https://example.com/sub"}]
	}]`)

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	subscription := &cfg.Subscriptions[0]
	providerName := subscription.ProviderNames()[0]

	nodes := []model.Node{
		testNode(providerName, "AU01 Sydney"),
		testNode(providerName, "US01"),
		testNode(providerName, "[高级] 🇭🇰 香港 01"),
		testNode(providerName, "mystery"),
		testNode(providerName, "KR01"),
		testNode(providerName, "[高级] 🇨🇳 台湾 01"),
		testNode(providerName, "[高级] 🇲🇽 墨西哥 01"),
		testNode(providerName, "NL01 Amsterdam"),
		testNode(providerName, "[基础] 🇯🇵 日本 01"),
		testNode(providerName, "SG01"),
	}

	got, err := Apply(nodes, cfg, subscription, ExtraRules{})
	if err != nil {
		t.Fatalf("Apply() error = %v", err)
	}

	wantPrefix := []string{
		"🇭🇰 HKG 01",
		"🇹🇼 TWN 01",
		"🇸🇬 SGP 01",
		"🇯🇵 JPN 01",
		"🇰🇷 KOR 01",
		"🇺🇸 USA 01",
	}
	for idx, want := range wantPrefix {
		if got[idx].FinalName != want {
			t.Fatalf("Apply()[%d].FinalName = %q, want %q", idx, got[idx].FinalName, want)
		}
	}
	foundMexico := false
	for _, node := range got {
		if node.Proxy["name"] != node.FinalName {
			t.Fatalf("proxy name = %v, want %q", node.Proxy["name"], node.FinalName)
		}
		if node.FinalName == "🇲🇽 MEX 01" {
			foundMexico = true
		}
	}
	if !foundMexico {
		t.Fatal("Mexican node was not renamed")
	}
	if got[len(got)-1].FinalName != "mystery" {
		t.Fatalf("last node FinalName = %q, want unknown region to stay last", got[len(got)-1].FinalName)
	}
}

func testNode(providerName, name string) model.Node {
	return model.Node{
		ProviderName: providerName,
		OriginalName: name,
		WorkingName:  name,
		FinalName:    name,
		Proxy: map[string]any{
			"name":   name,
			"type":   "ss",
			"server": name,
			"port":   443,
		},
	}
}

func TestSelectSubscriptionInfoIgnoresProvidersOutsideSuccessfulOrder(t *testing.T) {
	providers := map[string]model.ProviderMetadata{
		"failed": {
			SubscriptionInfo: model.SubscriptionInfo{Download: 123},
		},
	}

	if got := SelectSubscriptionInfo(providers, []string{"successful"}); got != nil {
		t.Fatalf("SelectSubscriptionInfo() = %#v, want nil", got)
	}
}
