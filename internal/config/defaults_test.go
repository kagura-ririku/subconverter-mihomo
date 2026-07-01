package config

import "testing"

func TestDefaultRegionSortOrderUsesPrimaryRegionsOnly(t *testing.T) {
	want := []string{"HKG", "TWN", "SGP", "JPN", "KOR", "USA"}

	if got := defaultRegionOrder(); !sameStringSlice(got, want) {
		t.Fatalf("defaultRegionOrder() = %v, want %v", got, want)
	}
	if got := defaultNodeSortRegionOrder(); !sameStringSlice(got, want) {
		t.Fatalf("defaultNodeSortRegionOrder() = %v, want %v", got, want)
	}
}

func TestDefaultRegionsIncludeCommonExtraCountries(t *testing.T) {
	flags := defaultRegionFlags()
	keywords := defaultRegionKeywords()

	for _, country := range []string{"AUS", "NZL", "NLD", "FRA", "UKR"} {
		if flags[country] == "" {
			t.Fatalf("defaultRegionFlags()[%q] is empty", country)
		}
		if len(keywords[country]) == 0 {
			t.Fatalf("defaultRegionKeywords()[%q] is empty", country)
		}
	}
}

func sameStringSlice(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for idx := range left {
		if left[idx] != right[idx] {
			return false
		}
	}
	return true
}
