package config

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
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
	Path            string            `json:"path"`
	UserAgent       string            `json:"user_agent"`
	Headers         map[string]string `json:"headers"`
	AgeSecretKey    string            `json:"age_secret_key"`
	IncludeRegex    RegexList         `json:"include_regex"`
	ExcludeRegex    RegexList         `json:"exclude_regex"`
	LocalPath       string            `json:"-"`
	ProviderName    string            `json:"-"`
	IncludePatterns []*regexp.Regexp  `json:"-"`
	ExcludePatterns []*regexp.Regexp  `json:"-"`
}

func (u *Upstream) IsLocal() bool {
	return u.LocalPath != ""
}

type CityKeyword struct {
	Country  string
	City     string
	Keywords []string
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
	UpstreamTimeout          time.Duration
	ControllerURL            string
	ControllerStartupTimeout time.Duration
	MihomoHome               string
	Subscriptions            []Subscription
	RegionOrder              []string
	NodeSortRegionOrder      []string
	CitylessCountries        map[string]struct{}
	RegionFlags              map[string]string
	RegionKeywords           map[string][]string
	CityKeywords             []CityKeyword
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
		*r = RegexList(normalizeList([]string{single}))
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
	seen := make(map[string]struct{}, len(s.Upstreams))
	for _, upstream := range s.Upstreams {
		name := strings.TrimSpace(upstream.ProviderName)
		if name == "" {
			continue
		}
		if _, exists := seen[name]; exists {
			continue
		}
		seen[name] = struct{}{}
		result = append(result, name)
	}
	return result
}

func (s *Subscription) FindUpstreamByProvider(providerName string) *Upstream {
	for idx := range s.Upstreams {
		if strings.TrimSpace(s.Upstreams[idx].ProviderName) == providerName {
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
		UpstreamTimeout:          mustDurationSeconds("UPSTREAM_TIMEOUT_SECONDS", 15),
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
		cfg.CityKeywords = cityKeywordsFromMap("", cityKeywords)
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
	return prepareSubscriptions(subscriptions, filepath.Dir(defaultSubscriptionsFile))
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
	return prepareSubscriptions(subscriptions, filepath.Dir(path))
}

func prepareSubscriptions(subscriptions []Subscription, subscriptionsDir string) ([]Subscription, error) {
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
			upstream.Path = strings.TrimSpace(upstream.Path)
			upstream.AgeSecretKey = strings.TrimSpace(upstream.AgeSecretKey)
			switch {
			case upstream.URL == "" && upstream.Path == "":
				return nil, fmt.Errorf("subscription %q upstream %q must contain url or path", subscription.Name, upstream.Name)
			case upstream.URL != "" && upstream.Path != "":
				return nil, fmt.Errorf("subscription %q upstream %q cannot contain both url and path", subscription.Name, upstream.Name)
			case upstream.Path != "":
				localPath, err := resolveLocalUpstreamPath(subscriptionsDir, upstream.Path)
				if err != nil {
					return nil, fmt.Errorf("subscription %q upstream %q: %w", subscription.Name, upstream.Name, err)
				}
				upstream.LocalPath = localPath
			}
			if upstream.URL != "" && strings.TrimSpace(upstream.UserAgent) == "" {
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

func resolveLocalUpstreamPath(subscriptionsDir, path string) (string, error) {
	extension := strings.ToLower(filepath.Ext(path))
	if extension != ".yaml" && extension != ".yml" {
		return "", fmt.Errorf("local upstream path %q must use .yaml or .yml", path)
	}

	baseDir, err := filepath.Abs(subscriptionsDir)
	if err != nil {
		return "", fmt.Errorf("resolve subscriptions directory: %w", err)
	}
	baseDir, err = filepath.EvalSymlinks(baseDir)
	if err != nil {
		return "", fmt.Errorf("resolve subscriptions directory links: %w", err)
	}
	resolved := path
	if !filepath.IsAbs(resolved) {
		resolved = filepath.Join(baseDir, resolved)
	}
	resolved, err = filepath.Abs(resolved)
	if err != nil {
		return "", fmt.Errorf("resolve local upstream path %q: %w", path, err)
	}

	canonical, err := filepath.EvalSymlinks(resolved)
	if err == nil {
		if !pathWithin(baseDir, canonical) {
			return "", fmt.Errorf("local upstream path %q must stay inside %s", path, baseDir)
		}
		info, statErr := os.Stat(canonical)
		if statErr != nil {
			return "", fmt.Errorf("read local upstream path %q: %w", path, statErr)
		}
		if !info.Mode().IsRegular() {
			return "", fmt.Errorf("local upstream path %q is not a regular file", path)
		}
		return canonical, nil
	}
	if !os.IsNotExist(err) {
		return "", fmt.Errorf("read local upstream path %q: %w", path, err)
	}

	// A missing file is a request-time upstream failure. Walk existing path
	// components first so a parent symlink cannot later redirect reads outside
	// the subscriptions directory.
	relative, relErr := filepath.Rel(baseDir, resolved)
	if relErr != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("local upstream path %q must stay inside %s", path, baseDir)
	}
	parts := strings.Split(relative, string(filepath.Separator))
	current := baseDir
	for idx, part := range parts {
		if part == "" || part == "." {
			continue
		}
		candidate := filepath.Join(current, part)
		info, lstatErr := os.Lstat(candidate)
		if lstatErr != nil {
			if os.IsNotExist(lstatErr) {
				for _, missingPart := range parts[idx:] {
					if missingPart != "" && missingPart != "." {
						current = filepath.Join(current, missingPart)
					}
				}
				return current, nil
			}
			return "", fmt.Errorf("read local upstream path %q: %w", path, lstatErr)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			canonical, evalErr := filepath.EvalSymlinks(candidate)
			if evalErr != nil {
				if os.IsNotExist(evalErr) {
					return "", fmt.Errorf("local upstream path %q must stay inside %s", path, baseDir)
				}
				return "", fmt.Errorf("read local upstream path %q: %w", path, evalErr)
			}
			if !pathWithin(baseDir, canonical) {
				return "", fmt.Errorf("local upstream path %q must stay inside %s", path, baseDir)
			}
			current = canonical
			continue
		}
		current = candidate
	}
	return current, nil
}

func pathWithin(baseDir, candidate string) bool {
	relative, err := filepath.Rel(baseDir, candidate)
	return err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
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

func cityKeywordsFromMap(country string, values map[string][]string) []CityKeyword {
	result := make([]CityKeyword, 0, len(values))
	for city, keywords := range values {
		result = append(result, CityKeyword{
			Country:  strings.ToUpper(strings.TrimSpace(country)),
			City:     strings.TrimSpace(city),
			Keywords: normalizeList(keywords),
		})
	}
	return result
}

func mustDurationSeconds(base string, fallback int) time.Duration {
	value := strings.TrimSpace(getenvPrefixed(base, ""))
	if value == "" {
		return time.Duration(fallback) * time.Second
	}
	seconds, err := strconv.Atoi(value)
	if err != nil || seconds <= 0 {
		return time.Duration(fallback) * time.Second
	}
	duration := time.Duration(seconds) * time.Second
	if duration <= 0 || duration/time.Second != time.Duration(seconds) {
		return time.Duration(fallback) * time.Second
	}
	return duration
}

func getenvPrefixed(base, fallback string) string {
	value := strings.TrimSpace(os.Getenv("SUBCONVERTER_" + base))
	if value == "" {
		return fallback
	}
	return value
}
