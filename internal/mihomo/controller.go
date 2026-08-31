package mihomo

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/kagura-ririku/subconverter-mihomo/internal/model"
)

type Controller struct {
	baseURL string
	client  *http.Client
}

const maxProviderMetadataSize = 16 << 20

func NewController(baseURL string, timeout time.Duration) *Controller {
	return &Controller{
		baseURL: strings.TrimRight(baseURL, "/"),
		client: &http.Client{
			Timeout: timeout,
		},
	}
}

func (c *Controller) WaitReady(ctx context.Context) error {
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		if err := c.Version(ctx); err == nil {
			return nil
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

func (c *Controller) Version(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/version", nil)
	if err != nil {
		return err
	}
	resp, err := c.do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return fmt.Errorf("controller version status %d", resp.StatusCode)
	}
	return nil
}

func (c *Controller) ReloadConfig(ctx context.Context, content []byte) error {
	payload, err := json.Marshal(struct {
		Path    string `json:"path"`
		Payload string `json:"payload"`
	}{
		Path:    "",
		Payload: string(content),
	})
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, c.baseURL+"/configs?force=true", bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return fmt.Errorf("reload config status %d", resp.StatusCode)
	}
	return nil
}

func (c *Controller) Providers(ctx context.Context) (map[string]model.ProviderMetadata, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/providers/proxies", nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return nil, fmt.Errorf("providers status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxProviderMetadataSize+1))
	if err != nil {
		return nil, err
	}
	if len(body) > maxProviderMetadataSize {
		return nil, fmt.Errorf("providers response exceeds %d bytes", maxProviderMetadataSize)
	}
	var decoded struct {
		Providers map[string]struct {
			SubscriptionInfo model.SubscriptionInfo `json:"subscriptionInfo"`
		} `json:"providers"`
	}
	if err := json.Unmarshal(body, &decoded); err != nil {
		return nil, err
	}
	providers := make(map[string]model.ProviderMetadata, len(decoded.Providers))
	for name, provider := range decoded.Providers {
		providers[name] = model.ProviderMetadata{SubscriptionInfo: provider.SubscriptionInfo}
	}
	return providers, nil
}

func (c *Controller) UpdateProvider(ctx context.Context, name string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, c.baseURL+"/providers/proxies/"+url.PathEscape(name), nil)
	if err != nil {
		return err
	}
	resp, err := c.do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return fmt.Errorf("update provider %s status %d", name, resp.StatusCode)
	}
	return nil
}

func (c *Controller) do(req *http.Request) (*http.Response, error) {
	return c.client.Do(req)
}
