package mihomo

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/kagura-ririku/subconverter-mihomo/internal/config"
)

// SyncLocalProviders copies and validates local providers independently.
func SyncLocalProviders(cfg *config.Config, subscription *config.Subscription) map[string]error {
	var wg sync.WaitGroup
	var mu sync.Mutex
	failures := map[string]error{}

	for idx := range subscription.Upstreams {
		upstream := &subscription.Upstreams[idx]
		if !upstream.IsLocal() {
			continue
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := syncLocalProviderFile(cfg, upstream); err != nil {
				mu.Lock()
				failures[upstream.ProviderName] = err
				mu.Unlock()
			}
		}()
	}

	wg.Wait()
	return failures
}

func syncLocalProviderFile(cfg *config.Config, upstream *config.Upstream) error {
	providerName := strings.TrimSpace(upstream.ProviderName)
	if err := validateProviderName(providerName); err != nil {
		return fmt.Errorf("invalid provider name %q: %w", upstream.ProviderName, err)
	}
	content, err := readProviderCacheFile(upstream.LocalPath)
	if err != nil {
		return fmt.Errorf("read local upstream %q: %w", upstream.Path, err)
	}
	if _, err := parseRawProvider(content, upstream.AgeSecretKey); err != nil {
		return fmt.Errorf("parse local upstream %q: %w", upstream.Path, err)
	}

	providersDir := filepath.Join(cfg.MihomoHome, "providers")
	if err := os.MkdirAll(providersDir, 0o755); err != nil {
		return fmt.Errorf("create provider directory: %w", err)
	}
	target := filepath.Join(cfg.MihomoHome, filepath.FromSlash(providerRelativePath(providerName)))
	if err := atomicWriteFile(target, content, ".local-provider-*.yaml"); err != nil {
		return fmt.Errorf("replace local provider %q: %w", upstream.Path, err)
	}
	return nil
}

func providerRelativePath(name string) string {
	return filepath.ToSlash(filepath.Join("providers", strings.TrimSpace(name)+".yaml"))
}

func validateProviderName(name string) error {
	if name == "" {
		return errors.New("provider name is empty")
	}
	if strings.EqualFold(name, "default") {
		return errors.New("provider name is reserved by mihomo")
	}
	if name == "." || name == ".." || filepath.IsAbs(name) {
		return errors.New("provider name must be a relative file name")
	}
	if strings.ContainsRune(name, '\x00') || strings.ContainsAny(name, `/\\`) {
		return errors.New("provider name must not contain path separators")
	}
	if filepath.Base(name) != name {
		return errors.New("provider name must be a file name")
	}
	return nil
}
