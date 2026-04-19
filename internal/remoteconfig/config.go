package remoteconfig

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/dlclark/regexp2"
	"github.com/kagura-ririku/subconverter-mihomo/internal/config"
	"github.com/kagura-ririku/subconverter-mihomo/internal/model"
	"github.com/kagura-ririku/subconverter-mihomo/internal/nodes"
	"gopkg.in/ini.v1"
	"gopkg.in/yaml.v3"
)

type ExternalConfig struct {
	Source                 string
	EnableRuleGenerator    bool
	OverwriteOriginalRules bool
	ClashRuleBase          string
	GroupSpecs             []string
	RuleSpecs              []RuleSpec
	NodeRules              nodes.ExtraRules
}

type RuleSpec struct {
	Group    string
	Kind     string
	Source   string
	Interval int
	Inline   string
}

type Compiled struct {
	BaseExtras    map[string]any
	ProxyGroups   []map[string]any
	RuleProviders map[string]any
	Rules         []string
}

func Load(ctx context.Context, cfg *config.Config, rawRef string) (*ExternalConfig, error) {
	rawRef = strings.TrimSpace(rawRef)
	if rawRef == "" {
		return nil, nil
	}

	content, source, err := readResource(ctx, cfg, rawRef, "")
	if err != nil {
		return nil, err
	}

	if looksLikeYAMLExternal(content) {
		return parseYAMLExternal(ctx, cfg, source, content)
	}
	return parseINIExternal(ctx, cfg, source, content)
}

func Compile(ctx context.Context, cfg *config.Config, ext *ExternalConfig, allNodes []model.Node) (*Compiled, error) {
	base := &Compiled{
		BaseExtras:    defaultBaseExtras(),
		RuleProviders: map[string]any{},
	}
	if ext == nil {
		base.ProxyGroups = defaultProxyGroups(allNodes)
		base.Rules = []string{"MATCH,PROXY"}
		return base, nil
	}

	type baseConfigResult struct {
		extras        map[string]any
		baseGroups    []map[string]any
		baseRules     []string
		ruleProviders map[string]any
		err           error
	}
	type rulesCompileResult struct {
		rules     []string
		providers map[string]any
		err       error
	}

	var (
		baseConfigCh chan baseConfigResult
		rulesCh      chan rulesCompileResult
	)

	if ext.ClashRuleBase != "" {
		baseConfigCh = make(chan baseConfigResult, 1)
		go func() {
			extras, baseGroups, baseRules, baseRuleProviders, err := loadBaseConfig(ctx, cfg, ext.Source, ext.ClashRuleBase)
			baseConfigCh <- baseConfigResult{
				extras:        extras,
				baseGroups:    baseGroups,
				baseRules:     baseRules,
				ruleProviders: baseRuleProviders,
				err:           err,
			}
		}()
	}
	if ext.EnableRuleGenerator {
		rulesCh = make(chan rulesCompileResult, 1)
		go func() {
			rules, providers, err := compileRules(ctx, cfg, ext.Source, ext.RuleSpecs)
			rulesCh <- rulesCompileResult{
				rules:     rules,
				providers: providers,
				err:       err,
			}
		}()
	}

	if baseConfigCh != nil {
		result := <-baseConfigCh
		if result.err != nil {
			return nil, result.err
		}
		base.BaseExtras = result.extras
		base.ProxyGroups = result.baseGroups
		base.Rules = result.baseRules
		for key, value := range result.ruleProviders {
			base.RuleProviders[key] = value
		}
	}

	if len(ext.GroupSpecs) > 0 {
		base.ProxyGroups = compileProxyGroups(ext.GroupSpecs, allNodes)
	}
	if len(base.ProxyGroups) == 0 {
		base.ProxyGroups = defaultProxyGroups(allNodes)
	}

	if rulesCh != nil {
		result := <-rulesCh
		if result.err != nil {
			return nil, result.err
		}
		if ext.OverwriteOriginalRules || len(base.Rules) == 0 {
			base.Rules = result.rules
		} else {
			base.Rules = append(base.Rules, result.rules...)
		}
		for key, value := range result.providers {
			base.RuleProviders[key] = value
		}
	}

	if len(base.Rules) == 0 {
		base.Rules = []string{"MATCH,PROXY"}
	}

	return base, nil
}

func parseINIExternal(ctx context.Context, cfg *config.Config, source string, content []byte) (*ExternalConfig, error) {
	file, err := ini.LoadSources(ini.LoadOptions{
		AllowShadows:        true,
		IgnoreInlineComment: true,
	}, content)
	if err != nil {
		return nil, err
	}

	section := file.Section("custom")
	result := &ExternalConfig{
		Source:                 source,
		EnableRuleGenerator:    section.Key("enable_rule_generator").MustBool(false),
		OverwriteOriginalRules: section.Key("overwrite_original_rules").MustBool(false),
		ClashRuleBase:          strings.TrimSpace(section.Key("clash_rule_base").String()),
	}

	groupSpecs, err := expandShadowKeys(ctx, cfg, source, section, "custom_proxy_group")
	if err != nil {
		return nil, err
	}
	ruleSpecsRaw, err := expandShadowKeys(ctx, cfg, source, section, "ruleset")
	if err != nil {
		return nil, err
	}
	renameSpecs, err := expandShadowKeys(ctx, cfg, source, section, "rename")
	if err != nil {
		return nil, err
	}
	includeSpecs, err := expandShadowKeys(ctx, cfg, source, section, "include_remarks")
	if err != nil {
		return nil, err
	}
	excludeSpecs, err := expandShadowKeys(ctx, cfg, source, section, "exclude_remarks")
	if err != nil {
		return nil, err
	}

	result.GroupSpecs = groupSpecs
	result.RuleSpecs = parseRuleSpecs(ruleSpecsRaw)
	result.NodeRules, err = compileNodeRules(renameSpecs, includeSpecs, excludeSpecs)
	if err != nil {
		return nil, err
	}
	return result, nil
}

func parseYAMLExternal(ctx context.Context, cfg *config.Config, source string, content []byte) (*ExternalConfig, error) {
	type yamlCustom struct {
		EnableRuleGenerator    bool     `yaml:"enable_rule_generator"`
		OverwriteOriginalRules bool     `yaml:"overwrite_original_rules"`
		ClashRuleBase          string   `yaml:"clash_rule_base"`
		ProxyGroups            []any    `yaml:"proxy_groups"`
		Rulesets               []any    `yaml:"rulesets"`
		RenameNode             []any    `yaml:"rename_node"`
		IncludeRemarks         []string `yaml:"include_remarks"`
		ExcludeRemarks         []string `yaml:"exclude_remarks"`
	}
	type yamlRoot struct {
		Custom yamlCustom `yaml:"custom"`
	}

	decoded := yamlRoot{}
	if err := yaml.Unmarshal(content, &decoded); err != nil {
		return nil, err
	}

	groupSpecs, err := expandYAMLItems(ctx, cfg, source, decoded.Custom.ProxyGroups)
	if err != nil {
		return nil, err
	}
	ruleSpecsRaw, err := expandYAMLItems(ctx, cfg, source, decoded.Custom.Rulesets)
	if err != nil {
		return nil, err
	}
	renameSpecs, err := expandYAMLItems(ctx, cfg, source, decoded.Custom.RenameNode)
	if err != nil {
		return nil, err
	}

	nodeRules, err := compileNodeRules(renameSpecs, decoded.Custom.IncludeRemarks, decoded.Custom.ExcludeRemarks)
	if err != nil {
		return nil, err
	}

	return &ExternalConfig{
		Source:                 source,
		EnableRuleGenerator:    decoded.Custom.EnableRuleGenerator,
		OverwriteOriginalRules: decoded.Custom.OverwriteOriginalRules,
		ClashRuleBase:          strings.TrimSpace(decoded.Custom.ClashRuleBase),
		GroupSpecs:             groupSpecs,
		RuleSpecs:              parseRuleSpecs(ruleSpecsRaw),
		NodeRules:              nodeRules,
	}, nil
}

func compileNodeRules(renameSpecs, includeSpecs, excludeSpecs []string) (nodes.ExtraRules, error) {
	result := nodes.ExtraRules{}

	for _, spec := range renameSpecs {
		patternText, target, ok := strings.Cut(spec, "@")
		if !ok {
			return result, fmt.Errorf("invalid rename rule %q", spec)
		}
		pattern, err := regexp.Compile(strings.TrimSpace(patternText))
		if err != nil {
			return result, err
		}
		result.Renames = append(result.Renames, nodes.RenameRule{
			Pattern: pattern,
			Target:  strings.TrimSpace(target),
		})
	}

	for _, spec := range includeSpecs {
		spec = strings.TrimSpace(spec)
		if spec == "" {
			continue
		}
		pattern, err := regexp.Compile(spec)
		if err != nil {
			return result, err
		}
		result.Include = append(result.Include, pattern)
	}
	for _, spec := range excludeSpecs {
		spec = strings.TrimSpace(spec)
		if spec == "" {
			continue
		}
		pattern, err := regexp.Compile(spec)
		if err != nil {
			return result, err
		}
		result.Exclude = append(result.Exclude, pattern)
	}
	return result, nil
}

func expandShadowKeys(ctx context.Context, cfg *config.Config, source string, section *ini.Section, key string) ([]string, error) {
	result := []string{}
	for _, value := range section.Key(key).ValueWithShadows() {
		values, err := expandTextValue(ctx, cfg, source, value)
		if err != nil {
			return nil, err
		}
		for _, expanded := range values {
			if strings.HasPrefix(expanded, key+"=") {
				expanded = strings.TrimSpace(strings.TrimPrefix(expanded, key+"="))
			}
			if expanded != "" {
				result = append(result, expanded)
			}
		}
	}
	return result, nil
}

func expandYAMLItems(ctx context.Context, cfg *config.Config, source string, items []any) ([]string, error) {
	result := []string{}
	for _, item := range items {
		switch value := item.(type) {
		case string:
			result = append(result, strings.TrimSpace(value))
		case map[string]any:
			if imported, ok := value["import"].(string); ok {
				values, err := expandTextValue(ctx, cfg, source, "!!import:"+imported)
				if err != nil {
					return nil, err
				}
				result = append(result, values...)
			}
		case map[any]any:
			if imported, ok := value["import"].(string); ok {
				values, err := expandTextValue(ctx, cfg, source, "!!import:"+imported)
				if err != nil {
					return nil, err
				}
				result = append(result, values...)
			}
		}
	}
	return result, nil
}

func expandTextValue(ctx context.Context, cfg *config.Config, source, value string) ([]string, error) {
	value = strings.TrimSpace(value)
	if !strings.HasPrefix(value, "!!import:") {
		return []string{value}, nil
	}

	importRef := strings.TrimSpace(strings.TrimPrefix(value, "!!import:"))
	content, _, err := readResource(ctx, cfg, importRef, source)
	if err != nil {
		return nil, err
	}

	result := []string{}
	scanner := bufio.NewScanner(bytes.NewReader(content))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, ";") || strings.HasPrefix(line, "#") {
			continue
		}
		result = append(result, line)
	}
	return result, scanner.Err()
}

func parseRuleSpecs(values []string) []RuleSpec {
	result := make([]RuleSpec, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}

		group, remainder, ok := strings.Cut(value, ",")
		if !ok {
			continue
		}
		group = strings.TrimSpace(group)
		remainder = strings.TrimSpace(remainder)

		interval := 86400
		source := remainder

		if !strings.HasPrefix(remainder, "[]") {
			parts := strings.Split(remainder, ",")
			if len(parts) > 1 {
				last := strings.TrimSpace(parts[len(parts)-1])
				if parsed, err := strconv.Atoi(last); err == nil && parsed > 0 {
					source = strings.TrimSpace(strings.Join(parts[:len(parts)-1], ","))
					interval = parsed
				}
			}
		}

		spec := RuleSpec{
			Group:    group,
			Source:   source,
			Interval: interval,
		}
		if strings.HasPrefix(source, "[]") {
			spec.Inline = strings.TrimPrefix(source, "[]")
		} else if kind, inner, ok := strings.Cut(source, ":"); ok && isRuleType(kind) {
			spec.Kind = kind
			spec.Source = inner
		}
		result = append(result, spec)
	}
	return result
}

func compileProxyGroups(specs []string, allNodes []model.Node) []map[string]any {
	output := []map[string]any{}
	groupNames := map[string]struct{}{}

	for _, spec := range specs {
		group, ok := parseGroupSpec(spec, allNodes)
		if !ok {
			continue
		}
		output = append(output, group)
		if name, _ := group["name"].(string); name != "" {
			groupNames[name] = struct{}{}
		}
	}

	validTargets := map[string]struct{}{
		"DIRECT": {},
		"REJECT": {},
		"GLOBAL": {},
		"PASS":   {},
	}
	for _, node := range allNodes {
		validTargets[node.FinalName] = struct{}{}
	}
	for name := range groupNames {
		validTargets[name] = struct{}{}
	}

	filtered := make([]map[string]any, 0, len(output))
	for _, group := range output {
		raw, _ := group["proxies"].([]string)
		proxies := make([]string, 0, len(raw))
		seen := map[string]struct{}{}
		for _, item := range raw {
			if _, ok := validTargets[item]; !ok {
				continue
			}
			if _, exists := seen[item]; exists {
				continue
			}
			seen[item] = struct{}{}
			proxies = append(proxies, item)
		}
		if len(proxies) == 0 {
			continue
		}
		group["proxies"] = proxies
		filtered = append(filtered, group)
	}
	return filtered
}

func parseGroupSpec(spec string, allNodes []model.Node) (map[string]any, bool) {
	parts := strings.Split(spec, "`")
	if len(parts) < 2 {
		return nil, false
	}
	groupName := strings.TrimSpace(parts[0])
	groupType := strings.TrimSpace(parts[1])
	if groupName == "" || groupType == "" {
		return nil, false
	}

	group := map[string]any{
		"name": groupName,
		"type": groupType,
	}

	memberSpecs := parts[2:]
	switch groupType {
	case "url-test", "fallback", "load-balance":
		if len(memberSpecs) < 3 {
			return nil, false
		}
		intervalSpec := memberSpecs[len(memberSpecs)-1]
		testURL := memberSpecs[len(memberSpecs)-2]
		memberSpecs = memberSpecs[:len(memberSpecs)-2]
		proxies := resolveGroupMembers(memberSpecs, allNodes)
		if len(proxies) == 0 {
			return nil, false
		}
		group["proxies"] = proxies
		group["url"] = strings.TrimSpace(testURL)
		interval, tolerance := parseGroupInterval(intervalSpec)
		group["interval"] = interval
		if tolerance > 0 && groupType == "url-test" {
			group["tolerance"] = tolerance
		}
	default:
		proxies := resolveGroupMembers(memberSpecs, allNodes)
		if len(proxies) == 0 {
			return nil, false
		}
		group["proxies"] = proxies
	}

	return group, true
}

func resolveGroupMembers(specs []string, allNodes []model.Node) []string {
	result := []string{}
	seen := map[string]struct{}{}

	for _, spec := range specs {
		spec = strings.TrimSpace(spec)
		if spec == "" {
			continue
		}
		if strings.HasPrefix(spec, "[]") {
			name := strings.TrimPrefix(spec, "[]")
			if _, ok := seen[name]; ok {
				continue
			}
			seen[name] = struct{}{}
			result = append(result, name)
			continue
		}
		if strings.HasPrefix(spec, "!!") {
			continue
		}
		matcher, err := compileGroupMatcher(spec)
		if err != nil {
			continue
		}
		for _, node := range allNodes {
			if matcher(node.OriginalName) || matcher(node.WorkingName) || matcher(node.FinalName) {
				if _, ok := seen[node.FinalName]; ok {
					continue
				}
				seen[node.FinalName] = struct{}{}
				result = append(result, node.FinalName)
			}
		}
	}

	return result
}

func compileGroupMatcher(spec string) (func(string) bool, error) {
	if re, err := regexp.Compile(spec); err == nil {
		return re.MatchString, nil
	}

	re2, err := regexp2.Compile(spec, regexp2.None)
	if err != nil {
		return nil, err
	}
	return func(value string) bool {
		matched, err := re2.MatchString(value)
		return err == nil && matched
	}, nil
}

func compileRules(ctx context.Context, cfg *config.Config, source string, specs []RuleSpec) ([]string, map[string]any, error) {
	type compiledRuleSpec struct {
		index        int
		rules        []string
		providerName string
		provider     map[string]any
		err          error
	}

	results := make([]compiledRuleSpec, len(specs))
	var wg sync.WaitGroup

	for idx, spec := range specs {
		if spec.Group == "" {
			continue
		}

		wg.Add(1)
		go func(idx int, spec RuleSpec) {
			defer wg.Done()

			if spec.Inline != "" {
				results[idx] = compiledRuleSpec{
					index: idx,
					rules: []string{inlineRule(spec.Group, spec.Inline)},
				}
				return
			}

			resolved, err := resolveRuleSource(ctx, cfg, source, spec)
			if err != nil {
				results[idx] = compiledRuleSpec{index: idx, err: err}
				return
			}
			if resolved.inline != nil {
				results[idx] = compiledRuleSpec{
					index: idx,
					rules: resolved.inline,
				}
				return
			}

			name := fmt.Sprintf("rule_%02d_%s", idx+1, sanitizeName(spec.Group))
			if resolved.provider != nil {
				resolved.provider["path"] = filepath.ToSlash(filepath.Join("ruleset", name+".yaml"))
			}
			results[idx] = compiledRuleSpec{
				index:        idx,
				providerName: name,
				provider:     resolved.provider,
				rules:        []string{"RULE-SET," + name + "," + spec.Group},
			}
		}(idx, spec)
	}

	wg.Wait()

	rules := make([]string, 0, len(specs))
	ruleProviders := map[string]any{}
	for idx, spec := range specs {
		if spec.Group == "" {
			continue
		}
		result := results[idx]
		if result.err != nil {
			return nil, nil, result.err
		}
		rules = append(rules, result.rules...)
		if result.providerName != "" && result.provider != nil {
			ruleProviders[result.providerName] = result.provider
		}
	}

	return rules, ruleProviders, nil
}

type resolvedRuleSource struct {
	inline   []string
	provider map[string]any
}

func resolveRuleSource(ctx context.Context, cfg *config.Config, baseSource string, spec RuleSpec) (*resolvedRuleSource, error) {
	source, err := resolveRemoteRef(spec.Source, baseSource)
	if err != nil {
		return nil, err
	}

	kind := spec.Kind
	if kind == "" {
		kind = "surge"
	}

	behavior, format, err := ruleProviderShape(kind, source)
	if err != nil {
		return nil, err
	}
	provider := map[string]any{
		"type":     "http",
		"url":      source,
		"behavior": behavior,
		"interval": spec.Interval,
		"path":     filepath.ToSlash(filepath.Join("ruleset", sanitizeName(spec.Group)+".yaml")),
	}
	if format != "" {
		provider["format"] = format
	}
	return &resolvedRuleSource{provider: provider}, nil
}

func parseInlineRules(content []byte, kind, target string) []string {
	result := []string{}
	scanner := bufio.NewScanner(bytes.NewReader(content))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") || strings.HasPrefix(line, "//") {
			continue
		}
		if kind == "surge" || kind == "clash-classic" {
			result = append(result, attachTarget(line, target))
		}
	}
	return result
}

func inlineRule(group, raw string) string {
	raw = strings.TrimSpace(raw)
	switch {
	case strings.EqualFold(raw, "FINAL"), strings.EqualFold(raw, "MATCH"):
		return "MATCH," + group
	default:
		return attachTarget(raw, group)
	}
}

func attachTarget(rule, target string) string {
	parts := strings.Split(rule, ",")
	if len(parts) == 0 {
		return rule
	}
	if strings.EqualFold(parts[0], "MATCH") {
		return "MATCH," + target
	}
	if len(parts) == 2 {
		return parts[0] + "," + parts[1] + "," + target
	}
	if len(parts) > 2 {
		payload := parts[1]
		params := strings.Join(parts[2:], ",")
		return parts[0] + "," + payload + "," + target + "," + params
	}
	return rule + "," + target
}

func loadBaseConfig(ctx context.Context, cfg *config.Config, source, ref string) (map[string]any, []map[string]any, []string, map[string]any, error) {
	content, _, err := readResource(ctx, cfg, ref, source)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	decoded := map[string]any{}
	if err := yaml.Unmarshal(content, &decoded); err != nil {
		return nil, nil, nil, nil, err
	}

	extras := map[string]any{}
	groups := []map[string]any{}
	rules := []string{}
	ruleProviders := map[string]any{}

	normalizeLegacyKeys(decoded)
	for key, value := range decoded {
		switch key {
		case "proxies":
		case "proxy-groups":
			groups = toGroupSlice(value)
		case "rules":
			rules = toStringSlice(value)
		case "rule-providers":
			if mapping, ok := value.(map[string]any); ok {
				ruleProviders = mapping
			}
		default:
			extras[key] = value
		}
	}

	return extras, groups, rules, ruleProviders, nil
}

func normalizeLegacyKeys(decoded map[string]any) {
	if value, ok := decoded["Proxy Group"]; ok {
		decoded["proxy-groups"] = value
		delete(decoded, "Proxy Group")
	}
	if value, ok := decoded["Rule"]; ok {
		decoded["rules"] = value
		delete(decoded, "Rule")
	}
	if value, ok := decoded["Proxy"]; ok {
		decoded["proxies"] = value
		delete(decoded, "Proxy")
	}
}

func toGroupSlice(value any) []map[string]any {
	items, ok := value.([]any)
	if !ok {
		return nil
	}
	result := make([]map[string]any, 0, len(items))
	for _, item := range items {
		if mapping, ok := item.(map[string]any); ok {
			result = append(result, mapping)
		}
	}
	return result
}

func toStringSlice(value any) []string {
	items, ok := value.([]any)
	if !ok {
		return nil
	}
	result := make([]string, 0, len(items))
	for _, item := range items {
		if text, ok := item.(string); ok {
			result = append(result, text)
		}
	}
	return result
}

func defaultBaseExtras() map[string]any {
	return map[string]any{
		"mixed-port": 7890,
		"allow-lan":  false,
		"mode":       "rule",
		"log-level":  "info",
		"dns": map[string]any{
			"enable": false,
		},
	}
}

func defaultProxyGroups(nodes []model.Node) []map[string]any {
	proxies := make([]string, 0, len(nodes))
	for _, node := range nodes {
		proxies = append(proxies, node.FinalName)
	}
	sort.Strings(proxies)
	if len(proxies) == 0 {
		return []map[string]any{
			{
				"name": "PROXY",
				"type": "select",
				"proxies": []string{
					"DIRECT",
				},
			},
		}
	}
	return []map[string]any{
		{
			"name":     "AUTO",
			"type":     "url-test",
			"proxies":  proxies,
			"url":      "https://cp.cloudflare.com/generate_204",
			"interval": 300,
		},
		{
			"name": "PROXY",
			"type": "select",
			"proxies": append([]string{
				"AUTO",
				"DIRECT",
			}, proxies...),
		},
	}
}

func parseGroupInterval(value string) (interval int, tolerance int) {
	interval = 300
	parts := strings.Split(strings.TrimSpace(value), ",")
	if len(parts) > 0 {
		if parsed, err := strconv.Atoi(strings.TrimSpace(parts[0])); err == nil && parsed > 0 {
			interval = parsed
		}
	}
	if len(parts) > 2 {
		if parsed, err := strconv.Atoi(strings.TrimSpace(parts[2])); err == nil && parsed > 0 {
			tolerance = parsed
		}
	}
	return interval, tolerance
}

func ruleProviderShape(kind, source string) (behavior string, format string, err error) {
	switch kind {
	case "surge", "clash-classic":
		return "classical", "text", nil
	case "clash-domain":
		return "domain", inferRuleFormat(source), nil
	case "clash-ipcidr":
		return "ipcidr", inferRuleFormat(source), nil
	case "quanx":
		return "", "", errors.New("quanx ruleset is not supported yet")
	default:
		return "", "", fmt.Errorf("unsupported ruleset type %q", kind)
	}
}

func inferRuleFormat(source string) string {
	lower := strings.ToLower(source)
	if strings.HasSuffix(lower, ".yaml") || strings.HasSuffix(lower, ".yml") {
		return "yaml"
	}
	return "text"
}

func isRuleType(value string) bool {
	switch value {
	case "surge", "quanx", "clash-domain", "clash-ipcidr", "clash-classic":
		return true
	default:
		return false
	}
}

func looksLikeYAMLExternal(content []byte) bool {
	trimmed := bytes.TrimSpace(content)
	return bytes.HasPrefix(trimmed, []byte("custom:")) || bytes.HasPrefix(trimmed, []byte("---"))
}

func readResource(ctx context.Context, cfg *config.Config, ref string, baseSource string) ([]byte, string, error) {
	resolved, err := resolveRemoteRef(ref, baseSource)
	if err != nil {
		return nil, "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, resolved, nil)
	if err != nil {
		return nil, "", err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return nil, "", fmt.Errorf("fetch %s: status %d", resolved, resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	return body, resolved, err
}

func resolveRemoteRef(ref string, baseSource string) (string, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return "", fmt.Errorf("empty reference")
	}
	if looksLikeURL(ref) {
		return ref, nil
	}
	if baseSource != "" && looksLikeURL(baseSource) {
		baseURL, err := url.Parse(baseSource)
		if err == nil {
			refURL, err := url.Parse(ref)
			if err == nil {
				return baseURL.ResolveReference(refURL).String(), nil
			}
		}
	}
	return "", fmt.Errorf("local path %q is not supported, use a remote URL", ref)
}

func looksLikeURL(value string) bool {
	return strings.HasPrefix(value, "http://") || strings.HasPrefix(value, "https://")
}

func sanitizeName(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "item"
	}
	value = strings.ToLower(value)
	value = regexp.MustCompile(`[^a-z0-9_-]+`).ReplaceAllString(value, "-")
	value = strings.Trim(value, "-")
	if value == "" {
		return "item"
	}
	return value
}
