package mihomo

import (
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/kagura-ririku/subconverter-mihomo/internal/config"
	"gopkg.in/yaml.v3"
)

const (
	maxProviderSize           = 64 << 20
	providerDownloadSizeLimit = maxProviderSize
	providerPlaceholderYAML   = "proxies:\n  - name: SUBCONVERTER_PLACEHOLDER\n    type: direct\n"
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
	seenProviders := map[string]struct{}{}
	for _, subscription := range cfg.Subscriptions {
		for _, upstream := range subscription.Upstreams {
			name := strings.TrimSpace(upstream.ProviderName)
			if name == "" {
				continue
			}
			if err := validateProviderName(name); err != nil {
				log.Printf("skip provider %q: invalid provider name: %v", name, err)
				continue
			}
			if _, exists := seenProviders[name]; exists {
				log.Printf("skip duplicate provider %q", name)
				continue
			}
			if err := validateProviderAgeSecretKey(upstream.AgeSecretKey); err != nil {
				log.Printf("skip provider %q: invalid age-secret-key: %v", name, err)
				continue
			}
			providerPath := providerRelativePath(name)
			provider := map[string]any{
				"path": providerPath,
			}
			if upstream.IsLocal() {
				if err := syncLocalProviderFile(cfg, &upstream); err != nil {
					log.Printf("initialize local provider %q with existing or empty cache: %v", name, err)
					providerCachePath := filepath.Join(cfg.MihomoHome, filepath.FromSlash(providerPath))
					if err := ensureProviderCacheFile(providerCachePath, upstream.AgeSecretKey); err != nil {
						log.Printf("skip local provider %q: initialize cache: %v", name, err)
						continue
					}
				}
				provider["type"] = "file"
			} else {
				providerCachePath := filepath.Join(cfg.MihomoHome, filepath.FromSlash(providerPath))
				if err := ensureProviderCacheFile(providerCachePath, upstream.AgeSecretKey); err != nil {
					log.Printf("skip remote provider %q: initialize cache: %v", name, err)
					continue
				}

				header := providerHeaders(&upstream)

				provider["type"] = "http"
				provider["url"] = upstream.URL
				provider["interval"] = 0
				provider["proxy"] = "DIRECT"
				provider["size-limit"] = providerDownloadSizeLimit
				if len(header) > 0 {
					provider["header"] = header
				}
			}
			if upstream.AgeSecretKey != "" {
				provider["age-secret-key"] = upstream.AgeSecretKey
			}
			seenProviders[name] = struct{}{}
			groupUses = append(groupUses, name)
			proxyProviders[name] = provider
		}
	}

	bootstrapGroup := map[string]any{
		"name": "SUBCONVERTER_BOOTSTRAP",
		"type": "select",
	}
	if len(groupUses) > 0 {
		bootstrapGroup["use"] = groupUses
	} else {
		bootstrapGroup["proxies"] = []string{"DIRECT"}
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
		"proxy-groups":    []map[string]any{bootstrapGroup},
		"rules": []string{
			"MATCH,SUBCONVERTER_BOOTSTRAP",
		},
	}

	encoded, err := yaml.Marshal(runtimeConfig)
	if err != nil {
		return err
	}
	return writeRuntimeConfigFile(filepath.Join(cfg.MihomoHome, "config.yaml"), encoded)
}

func writeRuntimeConfigFile(path string, content []byte) error {
	return atomicWriteFile(path, content, ".mihomo-config-*.yaml")
}

func ensureProviderCacheFile(path string, secretKeys ...string) error {
	if content, err := readProviderCacheFile(path); err == nil {
		if _, parseErr := parseRawProvider(content, secretKeys...); parseErr == nil {
			return nil
		}
	}
	return writeProviderCacheFile(path, []byte(providerPlaceholderYAML))
}

func writeProviderCacheFile(path string, content []byte) error {
	return atomicWriteFile(path, content, ".provider-cache-*.yaml")
}

func atomicWriteFile(path string, content []byte, pattern string) error {
	directory := filepath.Dir(path)
	temporary, err := os.CreateTemp(directory, pattern)
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)

	if _, err := temporary.Write(content); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Chmod(0o644); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	return os.Rename(temporaryPath, path)
}

func providerHeaders(upstream *config.Upstream) map[string][]string {
	header := make(map[string][]string, len(upstream.Headers)+1)
	for key, value := range upstream.Headers {
		key = http.CanonicalHeaderKey(strings.TrimSpace(key))
		if key == "" {
			continue
		}
		header[key] = []string{value}
	}
	if userAgent := strings.TrimSpace(upstream.UserAgent); userAgent != "" {
		header["User-Agent"] = []string{userAgent}
	}
	return header
}
