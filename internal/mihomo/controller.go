package mihomo

import (
	"context"
	"encoding/json"
	"fmt"
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

func (c *Controller) Providers(ctx context.Context) (map[string]model.Provider, error) {
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

	decoded := model.ProvidersResponse{}
	if err := json.NewDecoder(resp.Body).Decode(&decoded); err != nil {
		return nil, err
	}
	return decoded.Providers, nil
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
