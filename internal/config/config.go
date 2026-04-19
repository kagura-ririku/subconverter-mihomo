package config

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var invalidProviderName = regexp.MustCompile(`[^a-zA-Z0-9_-]+`)

type RegexList []string

type Upstream struct {
	Name            string            `json:"name"`
	URL             string            `json:"url"`
	UserAgent       string            `json:"user_agent"`
	Headers         map[string]string `json:"headers"`
	IncludeRegex    RegexList         `json:"include_regex"`
	ExcludeRegex    RegexList         `json:"exclude_regex"`
	ProviderName    string            `json:"-"`
	IncludePatterns []*regexp.Regexp  `json:"-"`
	ExcludePatterns []*regexp.Regexp  `json:"-"`
}

type Subscription struct {
	UUID              string           `json:"uuid"`
	Name              string           `json:"name"`
	RemoteConfig      string           `json:"remote_config"`
	EnableNodeRename  *bool            `json:"enable_node_rename,omitempty"`
	NodeRenameEnabled bool             `json:"-"`
	SortNodesByRegion bool             `json:"sort_nodes_by_region"`
	Upstreams         []Upstream       `json:"upstreams"`
	IncludeRegex      RegexList        `json:"include_regex"`
	ExcludeRegex      RegexList        `json:"exclude_regex"`
	IncludePatterns   []*regexp.Regexp `json:"-"`
	ExcludePatterns   []*regexp.Regexp `json:"-"`
}

type Config struct {
	ListenAddr               string
	AllowedHosts             []string
	RequestTimeout           time.Duration
	ControllerURL            string
	ControllerStartupTimeout time.Duration
	MihomoHome               string
	Subscriptions            []Subscription
	RegionOrder              []string
	NodeSortRegionOrder      []string
	CitylessCountries        map[string]struct{}
	RegionFlags              map[string]string
	RegionKeywords           map[string][]string
	CityKeywords             map[string][]string
}

const defaultSubscriptionsFile = "./config/subscriptions.json"

func (r *RegexList) UnmarshalJSON(data []byte) error {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		*r = nil
		return nil
	}

	var list []string
	if err := json.Unmarshal(trimmed, &list); err == nil {
		*r = RegexList(normalizeList(list))
		return nil
	}

	var single string
	if err := json.Unmarshal(trimmed, &single); err == nil {
		*r = RegexList(splitRegexText(single))
		return nil
	}

	return fmt.Errorf("regex list must be a string or string array")
}

func (c *Config) FindSubscription(uuid string) *Subscription {
	for idx := range c.Subscriptions {
		if c.Subscriptions[idx].UUID == uuid {
			return &c.Subscriptions[idx]
		}
	}
	return nil
}

func (s *Subscription) ProviderNames() []string {
	result := make([]string, 0, len(s.Upstreams))
	for _, upstream := range s.Upstreams {
		if upstream.ProviderName != "" {
			result = append(result, upstream.ProviderName)
		}
	}
	return result
}

func (s *Subscription) FindUpstreamByProvider(providerName string) *Upstream {
	for idx := range s.Upstreams {
		if s.Upstreams[idx].ProviderName == providerName {
			return &s.Upstreams[idx]
		}
	}
	return nil
}

func Load() (*Config, error) {
	subscriptions, err := loadSubscriptions()
	if err != nil {
		return nil, err
	}
	if len(subscriptions) == 0 {
		return nil, fmt.Errorf("config/subscriptions.json is required")
	}

	cfg := &Config{
		ListenAddr:               getenvPrefixed("LISTEN_ADDR", ":8080"),
		AllowedHosts:             splitListEnv("ALLOWED_HOSTS"),
		RequestTimeout:           mustDurationSeconds("REQUEST_TIMEOUT_SECONDS", 20),
		ControllerURL:            strings.TrimRight(getenvPrefixed("CONTROLLER_URL", "http://mihomo:9093"), "/"),
		ControllerStartupTimeout: mustDurationSeconds("CONTROLLER_STARTUP_TIMEOUT_SECONDS", 90),
		MihomoHome:               getenvPrefixed("MIHOMO_HOME", "/data/mihomo"),
		Subscriptions:            subscriptions,
		RegionOrder:              defaultRegionOrder(),
		NodeSortRegionOrder:      defaultNodeSortRegionOrder(),
		CitylessCountries:        defaultCitylessCountries(),
		RegionFlags:              defaultRegionFlags(),
		RegionKeywords:           defaultRegionKeywords(),
		CityKeywords:             defaultCityKeywords(),
	}

	if v := splitListEnv("REGION_ORDER"); len(v) > 0 {
		cfg.RegionOrder = v
	}
	if v := splitListEnv("CITYLESS_COUNTRIES"); len(v) > 0 {
		cfg.CitylessCountries = make(map[string]struct{}, len(v))
		for _, country := range v {
			cfg.CitylessCountries[strings.ToUpper(country)] = struct{}{}
		}
	}

	if regionFlags := loadStringMapEnv("REGION_FLAGS_JSON"); len(regionFlags) > 0 {
		cfg.RegionFlags = make(map[string]string, len(regionFlags))
		for key, value := range regionFlags {
			cfg.RegionFlags[strings.ToUpper(strings.TrimSpace(key))] = strings.TrimSpace(value)
		}
	}
	if regionKeywords := loadMapListEnv("REGION_KEYWORDS_JSON"); len(regionKeywords) > 0 {
		cfg.RegionKeywords = regionKeywords
	}
	if cityKeywords := loadMapListEnv("CITY_KEYWORDS_JSON"); len(cityKeywords) > 0 {
		cfg.CityKeywords = cityKeywords
	}

	return cfg, nil
}

func loadSubscriptions() ([]Subscription, error) {
	if file := strings.TrimSpace(getenvPrefixed("SUBSCRIPTIONS_FILE", "")); file != "" {
		return loadSubscriptionsFromFile(file)
	}
	if _, err := os.Stat(defaultSubscriptionsFile); err == nil {
		return loadSubscriptionsFromFile(defaultSubscriptionsFile)
	}

	raw := strings.TrimSpace(getenvPrefixed("SUBSCRIPTIONS_JSON", ""))
	if raw == "" {
		return nil, nil
	}
	var subscriptions []Subscription
	if err := json.Unmarshal([]byte(raw), &subscriptions); err != nil {
		return nil, fmt.Errorf("parse SUBCONVERTER_SUBSCRIPTIONS_JSON: %w", err)
	}
	return prepareSubscriptions(subscriptions)
}

func loadSubscriptionsFromFile(path string) ([]Subscription, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read subscriptions file %q: %w", path, err)
	}

	var subscriptions []Subscription
	if err := json.Unmarshal(content, &subscriptions); err != nil {
		return nil, fmt.Errorf("parse subscriptions file %q: %w", path, err)
	}
	return prepareSubscriptions(subscriptions)
}

func prepareSubscriptions(subscriptions []Subscription) ([]Subscription, error) {
	seenUUIDs := map[string]struct{}{}

	for subIdx := range subscriptions {
		subscription := &subscriptions[subIdx]
		subscription.UUID = strings.TrimSpace(subscription.UUID)
		if subscription.UUID == "" {
			return nil, fmt.Errorf("subscription %d missing uuid", subIdx+1)
		}
		if _, exists := seenUUIDs[subscription.UUID]; exists {
			return nil, fmt.Errorf("duplicate subscription uuid %q", subscription.UUID)
		}
		seenUUIDs[subscription.UUID] = struct{}{}

		subscription.Name = strings.TrimSpace(subscription.Name)
		if subscription.Name == "" {
			subscription.Name = fmt.Sprintf("subscription-%d", subIdx+1)
		}
		subscription.RemoteConfig = strings.TrimSpace(subscription.RemoteConfig)
		subscription.NodeRenameEnabled = true
		if subscription.EnableNodeRename != nil {
			subscription.NodeRenameEnabled = *subscription.EnableNodeRename
		}

		includePatterns, err := compilePatterns([]string(subscription.IncludeRegex))
		if err != nil {
			return nil, fmt.Errorf("compile include regex for subscription %q: %w", subscription.Name, err)
		}
		excludePatterns, err := compilePatterns([]string(subscription.ExcludeRegex))
		if err != nil {
			return nil, fmt.Errorf("compile exclude regex for subscription %q: %w", subscription.Name, err)
		}
		subscription.IncludePatterns = includePatterns
		subscription.ExcludePatterns = excludePatterns

		if len(subscription.Upstreams) == 0 {
			return nil, fmt.Errorf("subscription %q must contain at least one upstream", subscription.Name)
		}
		for upstreamIdx := range subscription.Upstreams {
			upstream := &subscription.Upstreams[upstreamIdx]
			upstream.Name = strings.TrimSpace(upstream.Name)
			if upstream.Name == "" {
				upstream.Name = fmt.Sprintf("upstream-%d", upstreamIdx+1)
			}
			upstream.URL = strings.TrimSpace(upstream.URL)
			if upstream.URL == "" {
				return nil, fmt.Errorf("subscription %q upstream %q missing url", subscription.Name, upstream.Name)
			}
			if strings.TrimSpace(upstream.UserAgent) == "" {
				upstream.UserAgent = "MetaCubeX/mihomo"
			}
			if upstream.Headers == nil {
				upstream.Headers = map[string]string{}
			}
			upstream.ProviderName = buildProviderName(subscription.Name, subIdx, upstream.Name, upstreamIdx)

			upstreamIncludePatterns, err := compilePatterns([]string(upstream.IncludeRegex))
			if err != nil {
				return nil, fmt.Errorf("compile include regex for subscription %q upstream %q: %w", subscription.Name, upstream.Name, err)
			}
			upstreamExcludePatterns, err := compilePatterns([]string(upstream.ExcludeRegex))
			if err != nil {
				return nil, fmt.Errorf("compile exclude regex for subscription %q upstream %q: %w", subscription.Name, upstream.Name, err)
			}
			upstream.IncludePatterns = upstreamIncludePatterns
			upstream.ExcludePatterns = upstreamExcludePatterns
		}
	}

	return subscriptions, nil
}

func buildProviderName(subscriptionName string, subIdx int, upstreamName string, upstreamIdx int) string {
	subscriptionPart := sanitizeProviderPart(subscriptionName)
	upstreamPart := sanitizeProviderPart(upstreamName)
	return fmt.Sprintf("s%d-%s-u%d-%s", subIdx+1, subscriptionPart, upstreamIdx+1, upstreamPart)
}

func sanitizeProviderPart(value string) string {
	value = invalidProviderName.ReplaceAllString(strings.TrimSpace(value), "-")
	value = strings.Trim(value, "-")
	if value == "" {
		return "default"
	}
	return value
}

func compilePatterns(values []string) ([]*regexp.Regexp, error) {
	result := make([]*regexp.Regexp, 0, len(values))
	for _, value := range values {
		if value == "" {
			continue
		}
		re, err := regexp.Compile(value)
		if err != nil {
			return nil, err
		}
		result = append(result, re)
	}
	return result, nil
}

func splitRegexText(value string) []string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	parts := strings.FieldsFunc(value, func(r rune) bool {
		return r == '\n' || r == '|'
	})
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			result = append(result, part)
		}
	}
	return result
}

func splitListEnv(base string) []string {
	value := strings.TrimSpace(getenvPrefixed(base, ""))
	if value == "" {
		return nil
	}

	if strings.HasPrefix(value, "[") {
		var list []string
		if err := json.Unmarshal([]byte(value), &list); err == nil {
			return normalizeList(list)
		}
	}

	parts := strings.FieldsFunc(value, func(r rune) bool {
		return r == ',' || r == '\n'
	})
	return normalizeList(parts)
}

func normalizeList(values []string) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			result = append(result, value)
		}
	}
	return result
}

func loadStringMapEnv(base string) map[string]string {
	raw := strings.TrimSpace(getenvPrefixed(base, ""))
	if raw == "" {
		return map[string]string{}
	}
	result := map[string]string{}
	_ = json.Unmarshal([]byte(raw), &result)
	return result
}

func loadMapListEnv(base string) map[string][]string {
	raw := strings.TrimSpace(getenvPrefixed(base, ""))
	if raw == "" {
		return nil
	}
	result := map[string][]string{}
	if err := json.Unmarshal([]byte(raw), &result); err != nil {
		return nil
	}
	normalized := make(map[string][]string, len(result))
	for key, values := range result {
		normalized[strings.ToUpper(strings.TrimSpace(key))] = normalizeList(values)
	}
	return normalized
}

func mustDurationSeconds(base string, fallback int) time.Duration {
	value := strings.TrimSpace(getenvPrefixed(base, ""))
	if value == "" {
		return time.Duration(fallback) * time.Second
	}
	seconds, err := strconv.Atoi(value)
	if err != nil {
		return time.Duration(fallback) * time.Second
	}
	return time.Duration(seconds) * time.Second
}

func getenvPrefixed(base, fallback string) string {
	value := strings.TrimSpace(os.Getenv("SUBCONVERTER_" + base))
	if value == "" {
		return fallback
	}
	return value
}
