package nodes

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/kagura-ririku/subconverter-mihomo/internal/config"
	"github.com/kagura-ririku/subconverter-mihomo/internal/model"
)

type RenameRule struct {
	Pattern *regexp.Regexp
	Target  string
}

type ExtraRules struct {
	Include []*regexp.Regexp
	Exclude []*regexp.Regexp
	Renames []RenameRule
}

type regionInfo struct {
	Country string
	City    string
}

type renameGroup struct {
	Country  string
	City     string
	BaseName string
	Indexes  []int
}

func BuildNodes(providers map[string]model.Provider) ([]model.Node, error) {
	nodes := make([]model.Node, 0)

	providerNames := make([]string, 0, len(providers))
	for name := range providers {
		providerNames = append(providerNames, name)
	}
	sort.Strings(providerNames)

	for _, providerName := range providerNames {
		provider := providers[providerName]
		for _, proxy := range provider.Proxies {
			sanitized := sanitizeProxy(proxy)
			name, _ := sanitized["name"].(string)
			if name == "" {
				continue
			}
			nodes = append(nodes, model.Node{
				ProviderName: providerName,
				OriginalName: name,
				WorkingName:  name,
				FinalName:    name,
				Proxy:        sanitized,
			})
		}
	}

	return nodes, nil
}

func BuildFromRawProviders(providers map[string][]map[string]any) ([]model.Node, error) {
	nodes := make([]model.Node, 0)

	providerNames := make([]string, 0, len(providers))
	for name := range providers {
		providerNames = append(providerNames, name)
	}
	sort.Strings(providerNames)

	for _, providerName := range providerNames {
		for _, proxy := range providers[providerName] {
			sanitized := sanitizeProxy(proxy)
			name, _ := sanitized["name"].(string)
			if name == "" {
				continue
			}
			nodes = append(nodes, model.Node{
				ProviderName: providerName,
				OriginalName: name,
				WorkingName:  name,
				FinalName:    name,
				Proxy:        sanitized,
			})
		}
	}

	return nodes, nil
}

func SelectSubscriptionInfo(providers map[string]model.Provider, preferredOrder []string) *model.SubscriptionInfo {
	for _, providerName := range preferredOrder {
		provider, ok := providers[providerName]
		if !ok {
			continue
		}
		if model.HasSubscriptionInfo(provider.SubscriptionInfo) {
			info := provider.SubscriptionInfo
			return &info
		}
	}

	providerNames := make([]string, 0, len(providers))
	for providerName := range providers {
		providerNames = append(providerNames, providerName)
	}
	sort.Strings(providerNames)
	for _, providerName := range providerNames {
		if model.HasSubscriptionInfo(providers[providerName].SubscriptionInfo) {
			info := providers[providerName].SubscriptionInfo
			return &info
		}
	}

	return nil
}

func Apply(nodes []model.Node, cfg *config.Config, subscription *config.Subscription, extra ExtraRules) ([]model.Node, error) {
	deduped := make([]model.Node, 0, len(nodes))
	seen := map[string]struct{}{}

	for _, node := range nodes {
		for _, rule := range extra.Renames {
			node.WorkingName = rule.Pattern.ReplaceAllString(node.WorkingName, rule.Target)
		}
		if !matchesSubscriptionRules(node, subscription, extra) {
			continue
		}

		fingerprint, err := nodeFingerprint(node.Proxy)
		if err != nil {
			return nil, err
		}
		if _, exists := seen[fingerprint]; exists {
			continue
		}
		seen[fingerprint] = struct{}{}
		deduped = append(deduped, node)
	}

	assignFinalNames(deduped, cfg)
	if subscription.SortNodesByRegion {
		sortNodesByRegion(deduped, cfg)
	}
	return deduped, nil
}

func sanitizeProxy(proxy map[string]any) map[string]any {
	clean := map[string]any{}
	for key, value := range proxy {
		switch key {
		case "history", "extra", "alive", "provider-name":
			continue
		default:
			clean[key] = value
		}
	}
	return clean
}

func nodeFingerprint(proxy map[string]any) (string, error) {
	clone := map[string]any{}
	for key, value := range proxy {
		if key == "name" {
			continue
		}
		clone[key] = value
	}
	encoded, err := json.Marshal(clone)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(encoded)
	return hex.EncodeToString(sum[:]), nil
}

func matchesSubscriptionRules(node model.Node, subscription *config.Subscription, extra ExtraRules) bool {
	includePatterns := append([]*regexp.Regexp{}, subscription.IncludePatterns...)
	excludePatterns := append([]*regexp.Regexp{}, subscription.ExcludePatterns...)
	if upstream := subscription.FindUpstreamByProvider(node.ProviderName); upstream != nil {
		includePatterns = append(includePatterns, upstream.IncludePatterns...)
		excludePatterns = append(excludePatterns, upstream.ExcludePatterns...)
	}
	includePatterns = append(includePatterns, extra.Include...)
	excludePatterns = append(excludePatterns, extra.Exclude...)

	if !matchesInclude(node, includePatterns) {
		return false
	}
	if matchesExclude(node, excludePatterns) {
		return false
	}
	return true
}

func matchesInclude(node model.Node, patterns []*regexp.Regexp) bool {
	if len(patterns) == 0 {
		return true
	}
	for _, pattern := range patterns {
		if pattern.MatchString(node.OriginalName) || pattern.MatchString(node.WorkingName) {
			return true
		}
	}
	return false
}

func matchesExclude(node model.Node, patterns []*regexp.Regexp) bool {
	for _, pattern := range patterns {
		if pattern.MatchString(node.OriginalName) || pattern.MatchString(node.WorkingName) {
			return true
		}
	}
	return false
}

func assignFinalNames(nodes []model.Node, cfg *config.Config) {
	groups := map[string]*renameGroup{}
	order := make([]*renameGroup, 0)

	for idx, node := range nodes {
		info := extractRegionInfo(node.WorkingName, cfg)
		key, baseName := renameGroupKey(node, info)
		group, ok := groups[key]
		if !ok {
			group = &renameGroup{
				Country:  info.Country,
				City:     info.City,
				BaseName: baseName,
			}
			groups[key] = group
			order = append(order, group)
		}
		group.Indexes = append(group.Indexes, idx)
	}

	sort.SliceStable(order, func(i, j int) bool {
		ao := regionOrderIndex(order[i].Country, cfg.RegionOrder)
		bo := regionOrderIndex(order[j].Country, cfg.RegionOrder)
		if ao != bo {
			return ao < bo
		}
		if order[i].Country == "Unknown" && order[j].Country != "Unknown" {
			return false
		}
		if order[i].Country != "Unknown" && order[j].Country == "Unknown" {
			return true
		}
		if order[i].Country == "Unknown" && order[j].Country == "Unknown" {
			return strings.ToLower(order[i].BaseName) < strings.ToLower(order[j].BaseName)
		}
		return order[i].City < order[j].City
	})

	for _, group := range order {
		assignGroupFinalNames(nodes, cfg, group)
	}
}

func renameGroupKey(node model.Node, info regionInfo) (string, string) {
	if info.Country == "Unknown" {
		baseName := strings.TrimSpace(node.WorkingName)
		if baseName == "" {
			baseName = strings.TrimSpace(node.OriginalName)
		}
		if baseName == "" {
			baseName = "Unknown"
		}
		return "unknown|" + strings.ToLower(baseName), baseName
	}
	key := info.Country
	if info.City != "" {
		key += "|" + info.City
	}
	return key, ""
}

func assignGroupFinalNames(nodes []model.Node, cfg *config.Config, group *renameGroup) {
	if group.Country == "Unknown" {
		for offset, idx := range group.Indexes {
			finalName := group.BaseName
			if len(group.Indexes) > 1 {
				finalName = fmt.Sprintf("%s %02d", group.BaseName, offset+1)
			}
			nodes[idx].FinalName = finalName
			nodes[idx].Proxy["name"] = finalName
		}
		return
	}

	prefix := ""
	if flag := cfg.RegionFlags[group.Country]; flag != "" {
		prefix = flag + " "
	}
	for offset, idx := range group.Indexes {
		pad := fmt.Sprintf("%02d", offset+1)
		if _, ok := cfg.CitylessCountries[group.Country]; ok {
			nodes[idx].FinalName = prefix + group.Country + " " + pad
		} else if group.City != "" {
			nodes[idx].FinalName = prefix + group.Country + " " + group.City + " " + pad
		} else {
			nodes[idx].FinalName = prefix + group.Country + " " + pad
		}
		nodes[idx].Proxy["name"] = nodes[idx].FinalName
	}
}

func regionOrderIndex(country string, order []string) int {
	for idx, value := range order {
		if value == country {
			return idx
		}
	}
	return len(order) + 1
}

func sortNodesByRegion(nodes []model.Node, cfg *config.Config) {
	sort.SliceStable(nodes, func(i, j int) bool {
		left := extractRegionInfo(nodes[i].WorkingName, cfg)
		right := extractRegionInfo(nodes[j].WorkingName, cfg)

		leftOrder := regionOrderIndex(left.Country, cfg.NodeSortRegionOrder)
		rightOrder := regionOrderIndex(right.Country, cfg.NodeSortRegionOrder)
		if leftOrder != rightOrder {
			return leftOrder < rightOrder
		}
		if left.City != right.City {
			return left.City < right.City
		}
		return strings.ToLower(nodes[i].FinalName) < strings.ToLower(nodes[j].FinalName)
	})
}

func extractRegionInfo(name string, cfg *config.Config) regionInfo {
	for country, keywords := range cfg.RegionKeywords {
		if !matchesKeywordList(name, keywords) {
			continue
		}
		if _, ok := cfg.CitylessCountries[country]; ok {
			return regionInfo{Country: country}
		}
		for city, keywords := range cfg.CityKeywords {
			if matchesKeywordList(name, keywords) {
				return regionInfo{Country: country, City: city}
			}
		}
		return regionInfo{Country: country}
	}
	return regionInfo{Country: "Unknown"}
}

func matchesKeywordList(name string, keywords []string) bool {
	for _, keyword := range keywords {
		if matchKeyword(name, keyword) {
			return true
		}
	}
	return false
}

func matchKeyword(name, keyword string) bool {
	if hasCJK(keyword) || strings.Contains(keyword, "🇭") || strings.Contains(keyword, "🇺") {
		return strings.Contains(name, keyword)
	}
	return strings.Contains(strings.ToLower(name), strings.ToLower(keyword))
}

func hasCJK(s string) bool {
	for _, r := range s {
		if r >= 0x4e00 && r <= 0x9fff {
			return true
		}
	}
	return false
}
