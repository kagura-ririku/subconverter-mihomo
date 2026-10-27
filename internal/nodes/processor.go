package nodes

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"unicode"

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

var noticePatterns = []*regexp.Regexp{
	regexp.MustCompile(`^(?:剩[余餘]流量|已用流量|[总總]流量|流量[余餘][额額]|` +
		`(?:套餐|[订訂][阅閱]|[账帳][号號])?(?:到期(?:时间|時間|日期)?|有效期)|` +
		`(?:距[离離])?(?:下次)?(?:流量)?重置(?:时间|時間|日期|剩[余餘]|[还還]有)?|剩[余餘]天[数數]|` +
		`(?:官方|[备備]用)?(?:官[网網]|[网網]站|[网網]址|客服|公告|通知)|` +
		`(?:官方)?(?:[联聯][系繫]方式|[联聯][系繫]客服|[订訂][阅閱]地址|[订訂][阅閱]更新|使用[说說]明|` +
		`[电電][报報]群|[电電][报報][频頻]道|用[户戶]群|TG群|TG[频頻]道))` +
		`(?:$|[^\p{L}\p{N}])`),
	regexp.MustCompile(`(?i)^(?:remaining\s+traffic|traffic\s+remaining)\s*[:：]?\s*\d+(?:\.\d+)?\s*(?:[kmgtpe]i?)?b\b`),
	regexp.MustCompile(`(?i)^(?:remaining\s+traffic|traffic\s+remaining|used\s+traffic|total\s+traffic|` +
		`expire(?:s|d|\s+date|\s+time)?|expiration(?:\s+date)?|expiry(?:\s+date)?|reset(?:\s+date)?|` +
		`(?:official\s+)?(?:website|support|customer\s+service|telegram(?:\s+(?:group|channel))?)|` +
		`announcement|notice|subscription\s+(?:url|update))\s*(?:$|[:：=|｜]|https?://|www\.|t\.me/)`),
	regexp.MustCompile(`^(?:(?:请|請)(?:及时|及時|定期|手动|手動|重新)?(?:更新|刷新|导入|導入)[订訂][阅閱]|` +
		`(?:套餐|[订訂][阅閱]|[账帳][号號])已(?:到期|[过過]期)|流量已用[尽盡])`),
	regexp.MustCompile(`(?i)^(?:https?://|www\.|t\.me/)\S+$`),
}

func isNoticeName(name string) bool {
	name = strings.TrimLeftFunc(name, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsNumber(r)
	})
	for _, pattern := range noticePatterns {
		if pattern.MatchString(name) {
			return true
		}
	}
	return false
}

func Build(providers map[string][]map[string]any) []model.Node {
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
			if strings.TrimSpace(name) == "" || isNoticeName(name) {
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

	return nodes
}

func SelectSubscriptionInfo(providers map[string]model.ProviderMetadata, preferredOrder []string) *model.SubscriptionInfo {
	allowed := make(map[string]struct{}, len(preferredOrder))
	for _, providerName := range preferredOrder {
		allowed[providerName] = struct{}{}
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
		if len(preferredOrder) > 0 {
			if _, ok := allowed[providerName]; !ok {
				continue
			}
		}
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

	if subscription.NodeRenameEnabled {
		assignFinalNames(deduped, cfg)
	} else {
		preserveWorkingNames(deduped)
	}
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

func preserveWorkingNames(nodes []model.Node) {
	for idx := range nodes {
		finalName := strings.TrimSpace(nodes[idx].WorkingName)
		if finalName == "" {
			finalName = strings.TrimSpace(nodes[idx].OriginalName)
		}
		if finalName == "" {
			finalName = "Unnamed"
		}
		nodes[idx].FinalName = finalName
		nodes[idx].Proxy["name"] = finalName
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
		if left.Country == "Unknown" && right.Country != "Unknown" {
			return false
		}
		if left.Country != "Unknown" && right.Country == "Unknown" {
			return true
		}
		if left.City != right.City {
			return left.City < right.City
		}
		return strings.ToLower(nodes[i].FinalName) < strings.ToLower(nodes[j].FinalName)
	})
}

func extractRegionInfo(name string, cfg *config.Config) regionInfo {
	for _, city := range cfg.CityKeywords {
		if matchesKeywordList(name, city.Keywords) {
			return regionInfo{Country: city.Country, City: city.City}
		}
	}

	bestCountry := ""
	bestKeywordLength := 0
	bestOrder := 0
	for _, candidate := range leadingCountryCandidates(name) {
		for order, country := range orderedRegionCountries(cfg) {
			matchLength := bestLeadingKeywordLength(candidate, cfg.RegionKeywords[country])
			if matchLength == 0 {
				continue
			}
			if matchLength > bestKeywordLength || (matchLength == bestKeywordLength && (bestCountry == "" || order < bestOrder)) {
				bestCountry = country
				bestKeywordLength = matchLength
				bestOrder = order
			}
		}
		if bestCountry != "" {
			return regionInfo{Country: bestCountry}
		}
	}

	return regionInfo{Country: "Unknown"}
}

func leadingCountryCandidates(name string) []string {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil
	}

	candidates := []string{}
	stripped := stripLeadingRegionalIndicators(name)
	if stripped != "" && stripped != name {
		candidates = append(candidates, stripped)
	}
	candidates = append(candidates, name)
	return candidates
}

func stripLeadingRegionalIndicators(name string) string {
	runes := []rune(strings.TrimSpace(name))
	idx := 0
	for idx < len(runes) && isRegionalIndicatorRune(runes[idx]) {
		idx++
	}
	if idx == 0 {
		return string(runes)
	}
	return strings.TrimLeftFunc(string(runes[idx:]), isLeadingRegionSeparator)
}

func isLeadingRegionSeparator(r rune) bool {
	if unicode.IsSpace(r) {
		return true
	}
	switch r {
	case '-', '_', '|', '/', '\\', ':', '.', ',', '+', '~', '#', '@', '[', ']', '(', ')':
		return true
	default:
		return false
	}
}

func orderedRegionCountries(cfg *config.Config) []string {
	seen := map[string]struct{}{}
	result := make([]string, 0, len(cfg.RegionKeywords))

	for _, country := range cfg.RegionOrder {
		country = strings.ToUpper(strings.TrimSpace(country))
		if country == "" {
			continue
		}
		if _, ok := cfg.RegionKeywords[country]; !ok {
			continue
		}
		if _, ok := seen[country]; ok {
			continue
		}
		seen[country] = struct{}{}
		result = append(result, country)
	}

	rest := make([]string, 0, len(cfg.RegionKeywords))
	for country := range cfg.RegionKeywords {
		country = strings.ToUpper(strings.TrimSpace(country))
		if country == "" {
			continue
		}
		if _, ok := seen[country]; ok {
			continue
		}
		seen[country] = struct{}{}
		rest = append(rest, country)
	}
	sort.Strings(rest)

	return append(result, rest...)
}

func bestLeadingKeywordLength(name string, keywords []string) int {
	best := 0
	for _, keyword := range keywords {
		if !matchLeadingKeyword(name, keyword) {
			continue
		}
		length := len([]rune(strings.TrimSpace(keyword)))
		if length > best {
			best = length
		}
	}
	return best
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
	if hasCJK(keyword) || hasRegionalIndicator(keyword) {
		return strings.Contains(name, keyword)
	}
	return containsToken(strings.ToLower(name), strings.ToLower(keyword))
}

func matchLeadingKeyword(name, keyword string) bool {
	name = strings.TrimSpace(name)
	keyword = strings.TrimSpace(keyword)
	if name == "" || keyword == "" {
		return false
	}

	if hasCJK(keyword) || hasRegionalIndicator(keyword) {
		return strings.HasPrefix(name, keyword)
	}

	name = strings.ToLower(name)
	keyword = strings.ToLower(keyword)
	if !strings.HasPrefix(name, keyword) {
		return false
	}
	return isLeadingKeywordBoundary(name, len(keyword))
}

func containsToken(name, keyword string) bool {
	start := 0
	for {
		idx := strings.Index(name[start:], keyword)
		if idx < 0 {
			return false
		}
		matchStart := start + idx
		matchEnd := matchStart + len(keyword)
		if isKeywordBoundary(name, matchStart-1) && isKeywordBoundary(name, matchEnd) {
			return true
		}
		start = matchStart + 1
	}
}

func isKeywordBoundary(value string, idx int) bool {
	if idx < 0 || idx >= len(value) {
		return true
	}
	ch := value[idx]
	return !((ch >= 'a' && ch <= 'z') || (ch >= '0' && ch <= '9'))
}

func isLeadingKeywordBoundary(value string, idx int) bool {
	if idx < 0 || idx >= len(value) {
		return true
	}
	ch := value[idx]
	if ch >= '0' && ch <= '9' {
		return true
	}
	return !(ch >= 'a' && ch <= 'z')
}

func hasCJK(s string) bool {
	for _, r := range s {
		if r >= 0x4e00 && r <= 0x9fff {
			return true
		}
	}
	return false
}

func hasRegionalIndicator(s string) bool {
	for _, r := range s {
		if isRegionalIndicatorRune(r) {
			return true
		}
	}
	return false
}

func isRegionalIndicatorRune(r rune) bool {
	return r >= 0x1f1e6 && r <= 0x1f1ff
}
