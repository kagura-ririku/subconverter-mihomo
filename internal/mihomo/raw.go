package mihomo

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/kagura-ririku/subconverter-mihomo/internal/config"
	"github.com/metacubex/mihomo/common/convert"
	"gopkg.in/yaml.v3"
)

type rawProxySchema struct {
	Proxies []map[string]any `yaml:"proxies"`
}

func LoadRawProviders(cfg *config.Config, providerNames []string) (map[string][]map[string]any, error) {
	result := map[string][]map[string]any{}

	for _, name := range providerNames {
		if strings.TrimSpace(name) == "" {
			continue
		}
		path := filepath.Join(cfg.MihomoHome, "providers", name+".yaml")
		content, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read provider cache %s: %w", path, err)
		}
		proxies, err := parseRawProvider(content)
		if err != nil {
			return nil, fmt.Errorf("parse provider cache %s: %w", path, err)
		}
		result[name] = proxies
	}

	return result, nil
}

func parseRawProvider(content []byte) ([]map[string]any, error) {
	schema := rawProxySchema{}
	if err := yaml.Unmarshal(content, &schema); err == nil && (schema.Proxies != nil || bytes.Contains(content, []byte("proxies:"))) {
		return schema.Proxies, nil
	}
	return convert.ConvertsV2Ray(content)
}
