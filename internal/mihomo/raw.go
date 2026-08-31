package mihomo

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/kagura-ririku/subconverter-mihomo/internal/config"
	"github.com/metacubex/mihomo/common/convert"
	mihomoage "github.com/metacubex/mihomo/component/age"
	"gopkg.in/yaml.v3"
)

type rawProxySchema struct {
	Proxies []map[string]any `yaml:"proxies"`
}

var errProviderCacheTooLarge = errors.New("provider cache exceeds size limit")
var errProviderHasNoProxies = errors.New("provider cache has no proxies")

func validateProviderAgeSecretKey(secretKey string) error {
	if secretKey == "" {
		return nil
	}
	return mihomoage.VeritySecretKeys(secretKey)
}

// LoadRawProviders parses provider caches independently.
func LoadRawProviders(cfg *config.Config, providerNames []string) (map[string][]map[string]any, map[string]error) {
	result := map[string][]map[string]any{}
	failures := map[string]error{}

	for _, name := range providerNames {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		if err := validateProviderName(name); err != nil {
			failures[name] = fmt.Errorf("invalid provider name: %w", err)
			continue
		}
		path := filepath.Join(cfg.MihomoHome, filepath.FromSlash(providerRelativePath(name)))
		content, err := readProviderCacheFile(path)
		if err != nil {
			failures[name] = fmt.Errorf("read provider cache %s: %w", path, err)
			continue
		}
		proxies, err := parseRawProvider(content, providerAgeSecretKey(cfg, name))
		if err != nil {
			failures[name] = fmt.Errorf("parse provider cache %s: %w", path, err)
			continue
		}
		result[name] = proxies
	}

	return result, failures
}

func providerAgeSecretKey(cfg *config.Config, providerName string) string {
	for _, subscription := range cfg.Subscriptions {
		for _, upstream := range subscription.Upstreams {
			if strings.TrimSpace(upstream.ProviderName) == providerName {
				return strings.TrimSpace(upstream.AgeSecretKey)
			}
		}
	}
	return ""
}

func readProviderCacheFile(path string) ([]byte, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if err := validateProviderCacheInfo(info); err != nil {
		return nil, err
	}

	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	info, err = file.Stat()
	if err != nil {
		return nil, err
	}
	if err := validateProviderCacheInfo(info); err != nil {
		return nil, err
	}
	content, err := io.ReadAll(io.LimitReader(file, maxProviderSize+1))
	if err != nil {
		return nil, err
	}
	if len(content) > maxProviderSize {
		return nil, fmt.Errorf("%w: more than %d bytes", errProviderCacheTooLarge, maxProviderSize)
	}
	return content, nil
}

func validateProviderCacheInfo(info os.FileInfo) error {
	if !info.Mode().IsRegular() {
		return errors.New("provider cache is not a regular file")
	}
	if info.Size() > maxProviderSize {
		return fmt.Errorf("%w: %d bytes (limit %d)", errProviderCacheTooLarge, info.Size(), maxProviderSize)
	}
	return nil
}

func parseRawProvider(content []byte, secretKeys ...string) ([]map[string]any, error) {
	decrypted, err := mihomoage.DecryptBytes(content, secretKeys...)
	if err != nil {
		return nil, fmt.Errorf("decrypt provider cache: %w", err)
	}
	if len(decrypted) > maxProviderSize {
		return nil, fmt.Errorf("%w: decrypted content is %d bytes (limit %d)", errProviderCacheTooLarge, len(decrypted), maxProviderSize)
	}
	content = decrypted

	schema := rawProxySchema{}
	if err := yaml.Unmarshal(content, &schema); err == nil && schema.Proxies != nil {
		if len(schema.Proxies) == 0 {
			return nil, errProviderHasNoProxies
		}
		return schema.Proxies, nil
	}
	proxies, err := convert.ConvertsV2Ray(content)
	if err != nil {
		return nil, err
	}
	if len(proxies) == 0 {
		return nil, errProviderHasNoProxies
	}
	return proxies, nil
}
