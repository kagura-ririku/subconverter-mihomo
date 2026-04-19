package render

import (
	"sort"

	"github.com/kagura-ririku/subconverter-mihomo/internal/model"
	"github.com/kagura-ririku/subconverter-mihomo/internal/remoteconfig"
	"gopkg.in/yaml.v3"
)

func Clash(compiled *remoteconfig.Compiled, allNodes []model.Node) ([]byte, error) {
	root := &yaml.Node{Kind: yaml.MappingNode}

	keys := make([]string, 0, len(compiled.BaseExtras))
	for key := range compiled.BaseExtras {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		appendValue(root, key, compiled.BaseExtras[key])
	}

	proxies := make([]map[string]any, 0, len(allNodes))
	for _, node := range allNodes {
		proxies = append(proxies, node.Proxy)
	}
	appendValue(root, "proxies", proxies)
	appendValue(root, "proxy-groups", compiled.ProxyGroups)
	if len(compiled.RuleProviders) > 0 {
		appendValue(root, "rule-providers", compiled.RuleProviders)
	}
	appendValue(root, "rules", compiled.Rules)

	return yaml.Marshal(root)
}

func appendValue(root *yaml.Node, key string, value any) {
	keyNode := &yaml.Node{Kind: yaml.ScalarNode, Value: key}
	valueNode := &yaml.Node{}
	encoded, err := yaml.Marshal(value)
	if err != nil {
		return
	}
	if err := yaml.Unmarshal(encoded, valueNode); err != nil {
		return
	}
	if valueNode.Kind == yaml.DocumentNode && len(valueNode.Content) > 0 {
		valueNode = valueNode.Content[0]
	}
	if valueNode.Kind == 0 && len(valueNode.Content) > 0 {
		valueNode = valueNode.Content[0]
	}
	root.Content = append(root.Content, keyNode, valueNode)
}
