package nodes

import (
	"testing"

	"github.com/kagura-ririku/subconverter-mihomo/internal/config"
)

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

func TestExtractRegionInfoMatchesCountryOnlyFromLeadingRegionPart(t *testing.T) {
	cfg := &config.Config{
		CitylessCountries: map[string]struct{}{},
		RegionKeywords: map[string][]string{
			"NGA": {"NG", "NGA", "Nigeria"},
			"USA": {"US", "USA", "America"},
		},
		CityKeywords: []config.CityKeyword{},
	}

	tests := []struct {
		name string
		want string
	}{
		{name: "NG-01 Lagos", want: "NGA"},
		{name: "USA Los Angeles 01", want: "USA"},
		{name: "Relay NG 01", want: "Unknown"},
		{name: "Los Angeles 01", want: "Unknown"},
	}

	for _, tt := range tests {
		if got := extractRegionInfo(tt.name, cfg).Country; got != tt.want {
			t.Fatalf("extractRegionInfo(%q).Country = %s, want %s", tt.name, got, tt.want)
		}
	}
}
