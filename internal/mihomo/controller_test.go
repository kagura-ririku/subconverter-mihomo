package mihomo

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestControllerProvidersLoadsSubscriptionInfo(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/providers/proxies" {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"providers": map[string]any{
				"source": map[string]any{
					"subscriptionInfo": map[string]any{"download": 123},
					"proxies":          []map[string]any{{"name": "ignored"}},
				},
			},
		})
	}))
	defer server.Close()

	providers, err := NewController(server.URL, time.Second).Providers(t.Context())
	if err != nil {
		t.Fatalf("Providers() error = %v", err)
	}
	provider, ok := providers["source"]
	if !ok {
		t.Fatalf("Providers() = %#v, want source provider", providers)
	}
	if provider.SubscriptionInfo.Download != 123 {
		t.Fatalf("subscription download = %d, want 123", provider.SubscriptionInfo.Download)
	}
}

func TestControllerReloadConfig(t *testing.T) {
	const content = "mixed-port: 7890\n"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut || r.URL.Path != "/configs" {
			http.NotFound(w, r)
			return
		}
		if got := r.URL.Query().Get("force"); got != "true" {
			t.Errorf("force query = %q, want true", got)
		}
		if got := r.Header.Get("Content-Type"); got != "application/json" {
			t.Errorf("Content-Type = %q, want application/json", got)
		}
		var request struct {
			Path    string `json:"path"`
			Payload string `json:"payload"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Errorf("decode request: %v", err)
			return
		}
		if request.Payload != content {
			t.Errorf("payload = %q, want %q", request.Payload, content)
		}
		if request.Path != "" {
			t.Errorf("path = %q, want empty path for payload reload", request.Path)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	if err := NewController(server.URL, time.Second).ReloadConfig(t.Context(), []byte(content)); err != nil {
		t.Fatalf("ReloadConfig() error = %v", err)
	}
}
