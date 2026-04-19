package mihomo

import (
	"os"
	"path/filepath"

	"github.com/kagura-ririku/subconverter-mihomo/internal/config"
	"gopkg.in/yaml.v3"
)

func WriteRuntimeConfig(cfg *config.Config) error {
	if err := os.MkdirAll(cfg.MihomoHome, 0o755); err != nil {
		return err
	}
	providersDir := filepath.Join(cfg.MihomoHome, "providers")
	if err := os.MkdirAll(providersDir, 0o755); err != nil {
		return err
	}

	proxyProviders := map[string]any{}
	groupUses := []string{}
	for _, subscription := range cfg.Subscriptions {
		for _, upstream := range subscription.Upstreams {
			name := upstream.ProviderName
			if name == "" {
				continue
			}
			groupUses = append(groupUses, name)
			providerCachePath := filepath.Join(providersDir, name+".yaml")
			if err := ensureProviderCacheFile(providerCachePath); err != nil {
				return err
			}

			header := map[string][]string{}
			for key, value := range upstream.Headers {
				header[key] = []string{value}
			}
			if upstream.UserAgent != "" {
				header["User-Agent"] = []string{upstream.UserAgent}
			}

			provider := map[string]any{
				"type":     "http",
				"url":      upstream.URL,
				"interval": 0,
				"path":     filepath.ToSlash(filepath.Join("providers", name+".yaml")),
			}
			if len(header) > 0 {
				provider["header"] = header
			}
			proxyProviders[name] = provider
		}
	}

	runtimeConfig := map[string]any{
		"mixed-port":          7890,
		"allow-lan":           false,
		"bind-address":        "127.0.0.1",
		"mode":                "rule",
		"log-level":           "info",
		"external-controller": "0.0.0.0:9093",
		"profile": map[string]any{
			"store-selected": false,
			"store-fake-ip":  false,
		},
		"dns": map[string]any{
			"enable": false,
		},
		"proxy-providers": proxyProviders,
		"proxy-groups": []map[string]any{
			{
				"name": "SUBCONVERTER_BOOTSTRAP",
				"type": "select",
				"use":  groupUses,
			},
		},
		"rules": []string{
			"MATCH,SUBCONVERTER_BOOTSTRAP",
		},
	}

	encoded, err := yaml.Marshal(runtimeConfig)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(cfg.MihomoHome, "config.yaml"), encoded, 0o644)
}

func ensureProviderCacheFile(path string) error {
	if _, err := os.Stat(path); err == nil {
		return nil
	} else if !os.IsNotExist(err) {
		return err
	}
	return os.WriteFile(path, []byte("proxies: []\n"), 0o644)
}
