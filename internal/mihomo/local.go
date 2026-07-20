package mihomo

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/kagura-ririku/subconverter-mihomo/internal/config"
)

func SyncLocalProviders(cfg *config.Config, subscription *config.Subscription) error {
	var wg sync.WaitGroup
	var mu sync.Mutex
	errs := []error{}

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
				errs = append(errs, err)
				mu.Unlock()
			}
		}()
	}

	wg.Wait()
	return errors.Join(errs...)
}

func syncLocalProviderFile(cfg *config.Config, upstream *config.Upstream) error {
	content, err := os.ReadFile(upstream.LocalPath)
	if err != nil {
		return fmt.Errorf("read local upstream %q: %w", upstream.Path, err)
	}
	if _, err := parseRawProvider(content); err != nil {
		return fmt.Errorf("parse local upstream %q: %w", upstream.Path, err)
	}

	providersDir := filepath.Join(cfg.MihomoHome, "providers")
	if err := os.MkdirAll(providersDir, 0o755); err != nil {
		return fmt.Errorf("create provider directory: %w", err)
	}
	target := filepath.Join(cfg.MihomoHome, filepath.FromSlash(providerRelativePath(upstream.ProviderName)))
	temporary, err := os.CreateTemp(providersDir, ".local-provider-*.yaml")
	if err != nil {
		return fmt.Errorf("create local provider temporary file: %w", err)
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)

	if _, err := temporary.Write(content); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("write local provider %q: %w", upstream.Path, err)
	}
	if err := temporary.Chmod(0o644); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("set local provider permissions: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close local provider %q: %w", upstream.Path, err)
	}
	if err := os.Rename(temporaryPath, target); err != nil {
		return fmt.Errorf("replace local provider %q: %w", upstream.Path, err)
	}
	return nil
}

func providerRelativePath(name string) string {
	return filepath.ToSlash(filepath.Join("providers", name+".yaml"))
}
