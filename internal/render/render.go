package render

import (
	"fmt"
	"sort"

	"github.com/kagura-ririku/subconverter-mihomo/internal/model"
	"github.com/kagura-ririku/subconverter-mihomo/internal/remoteconfig"
	"go.yaml.in/yaml/v3"
)

func Clash(compiled *remoteconfig.Compiled, allNodes []model.Node) ([]byte, error) {
	root := &yaml.Node{Kind: yaml.MappingNode}

	keys := make([]string, 0, len(compiled.BaseExtras))
	for key := range compiled.BaseExtras {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		if err := appendValue(root, key, compiled.BaseExtras[key]); err != nil {
			return nil, err
		}
	}

	proxies := make([]map[string]any, 0, len(allNodes))
	for _, node := range allNodes {
		proxies = append(proxies, node.Proxy)
	}
	if err := appendValue(root, "proxies", proxies); err != nil {
		return nil, err
	}
	if err := appendValue(root, "proxy-groups", compiled.ProxyGroups); err != nil {
		return nil, err
	}
	if len(compiled.RuleProviders) > 0 {
		if err := appendValue(root, "rule-providers", compiled.RuleProviders); err != nil {
			return nil, err
		}
	}
	if err := appendValue(root, "rules", compiled.Rules); err != nil {
		return nil, err
	}

	return yaml.Marshal(root)
}

func appendValue(root *yaml.Node, key string, value any) (err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("marshal YAML field %q: %v", key, recovered)
		}
	}()

	keyNode := &yaml.Node{Kind: yaml.ScalarNode, Value: key}
	valueNode := &yaml.Node{}
	encoded, err := yaml.Marshal(value)
	if err != nil {
		return err
	}
	if err := yaml.Unmarshal(encoded, valueNode); err != nil {
		return err
	}
	if valueNode.Kind == yaml.DocumentNode && len(valueNode.Content) > 0 {
		valueNode = valueNode.Content[0]
	}
	if valueNode.Kind == 0 && len(valueNode.Content) > 0 {
		valueNode = valueNode.Content[0]
	}
	root.Content = append(root.Content, keyNode, valueNode)
	return nil
}
