package app

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kagura-ririku/subconverter-mihomo/internal/config"
	"github.com/kagura-ririku/subconverter-mihomo/internal/mihomo"
	"go.yaml.in/yaml/v3"
)

func TestHandleSubscriptionSkipsFailedUpstream(t *testing.T) {
	controller := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPut && strings.HasSuffix(r.URL.Path, "/good"):
			w.WriteHeader(http.StatusNoContent)
		case r.Method == http.MethodPut && strings.HasSuffix(r.URL.Path, "/bad"):
			w.WriteHeader(http.StatusServiceUnavailable)
		case r.Method == http.MethodGet && r.URL.Path == "/providers/proxies":
			_ = json.NewEncoder(w).Encode(map[string]any{"providers": map[string]any{
				"bad": map[string]any{"subscriptionInfo": map[string]any{"download": 123}},
			}})
		default:
			http.NotFound(w, r)
		}
	}))
	defer controller.Close()

	application := newTestApplication(t, controller.URL, []config.Upstream{
		{URL: "https://example.com/good", ProviderName: "good"},
		{URL: "https://example.com/bad", ProviderName: "bad"},
	})
	writeProviderCache(t, application.cfg.MihomoHome, "good", "good-node")
	writeProviderCache(t, application.cfg.MihomoHome, "bad", "stale-bad-node")

	request := httptest.NewRequest(http.MethodGet, "/test-uuid/clash", nil)
	response := httptest.NewRecorder()
	application.Handler().ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %q", response.Code, response.Body.String())
	}
	var output struct {
		Proxies []map[string]any `yaml:"proxies"`
	}
	if err := yaml.Unmarshal(response.Body.Bytes(), &output); err != nil {
		t.Fatal(err)
	}
	if len(output.Proxies) != 1 || output.Proxies[0]["name"] != "good-node" {
		t.Fatalf("proxies = %#v, want only good-node", output.Proxies)
	}
	if got := response.Header().Get("subscription-userinfo"); got != "" {
		t.Fatalf("subscription-userinfo = %q, want empty for failed provider", got)
	}
}

func TestHandleSubscriptionReturnsBadGatewayWhenAllUpstreamsFail(t *testing.T) {
	controller := newProviderController(t, map[string]int{"bad": http.StatusServiceUnavailable})
	defer controller.Close()

	application := newTestApplication(t, controller.URL, []config.Upstream{
		{URL: "https://example.com/bad", ProviderName: "bad"},
	})
	writeProviderCache(t, application.cfg.MihomoHome, "bad", "stale-bad-node")

	request := httptest.NewRequest(http.MethodGet, "/test-uuid/clash", nil)
	response := httptest.NewRecorder()
	application.Handler().ServeHTTP(response, request)

	if response.Code != http.StatusBadGateway || strings.TrimSpace(response.Body.String()) != "no usable upstream providers" {
		t.Fatalf("status/body = %d/%q, want 502/no usable upstream providers", response.Code, response.Body.String())
	}
}

func TestHandleSubscriptionValidatesLocalProvider(t *testing.T) {
	for _, valid := range []bool{true, false} {
		updateStatus, wantStatus := http.StatusNoContent, http.StatusOK
		if !valid {
			updateStatus, wantStatus = http.StatusInternalServerError, http.StatusBadGateway
		}
		controller := newProviderController(t, map[string]int{"local": updateStatus})
		t.Cleanup(controller.Close)

		sourcePath := filepath.Join(t.TempDir(), "local.yaml")
		writeProviderSource(t, sourcePath, "local-node")
		application := newTestApplication(t, controller.URL, []config.Upstream{
			{Path: "local.yaml", LocalPath: sourcePath, ProviderName: "local"},
		})

		request := httptest.NewRequest(http.MethodGet, "/test-uuid/clash", nil)
		response := httptest.NewRecorder()
		application.Handler().ServeHTTP(response, request)

		if response.Code != wantStatus || strings.Contains(response.Body.String(), "local-node") != valid {
			t.Fatalf("valid=%v: status/body = %d/%q, want %d", valid, response.Code, response.Body.String(), wantStatus)
		}
	}
}

func TestRefreshProvidersKeepsFastUpstreamWhenAnotherTimesOut(t *testing.T) {
	controller := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut || !strings.HasPrefix(r.URL.Path, "/providers/proxies/") {
			http.NotFound(w, r)
			return
		}
		if strings.HasSuffix(r.URL.Path, "/slow") {
			<-r.Context().Done()
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer controller.Close()

	application := newTestApplication(t, controller.URL, []config.Upstream{
		{URL: "https://example.com/slow", ProviderName: "slow"},
		{URL: "https://example.com/fast", ProviderName: "fast"},
	})
	application.cfg.UpstreamTimeout = 40 * time.Millisecond
	writeProviderCache(t, application.cfg.MihomoHome, "fast", "fast-node")

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	rawProviders, providerNames, err := application.refreshProviders(ctx, &application.cfg.Subscriptions[0])
	if err != nil {
		t.Fatalf("refreshProviders() error = %v", err)
	}
	if strings.Join(providerNames, ",") != "fast" || rawProviders["fast"][0]["name"] != "fast-node" {
		t.Fatalf("providers = %v/%#v, want fast provider", providerNames, rawProviders)
	}
}

func TestNewReloadsWrittenRuntimeConfig(t *testing.T) {
	var reloadedPayload string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/version":
			w.WriteHeader(http.StatusOK)
		case r.Method == http.MethodPut && r.URL.Path == "/configs":
			var request struct {
				Payload string `json:"payload"`
			}
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
				t.Errorf("decode reload request: %v", err)
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			reloadedPayload = request.Payload
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	if _, err := New(&config.Config{
		RequestTimeout:           time.Second,
		ControllerURL:            server.URL,
		ControllerStartupTimeout: time.Second,
		MihomoHome:               t.TempDir(),
	}); err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if !strings.Contains(reloadedPayload, "proxy-groups:") {
		t.Fatalf("reload payload = %q, want generated mihomo config", reloadedPayload)
	}
}

func TestParseSubscriptionPathRejectsPathNormalization(t *testing.T) {
	valid := []string{"/test-uuid", "/test-uuid/", "/test-uuid/clash"}
	for _, path := range valid {
		if _, _, ok := parseSubscriptionPath(path); !ok {
			t.Errorf("parseSubscriptionPath(%q) rejected valid path", path)
		}
	}

	invalid := []string{"/test-uuid/../other", "/test-uuid/./clash", "/test-uuid//clash", "/test-uuid/clash/extra", `/test-uuid\\clash`}
	for _, path := range invalid {
		if _, _, ok := parseSubscriptionPath(path); ok {
			t.Errorf("parseSubscriptionPath(%q) accepted malformed path", path)
		}
	}
}

func newTestApplication(t *testing.T, controllerURL string, upstreams []config.Upstream) *Application {
	t.Helper()
	subscription := config.Subscription{
		UUID:              "test-uuid",
		Name:              "test-subscription",
		NodeRenameEnabled: false,
		Upstreams:         upstreams,
	}
	cfg := &config.Config{
		RequestTimeout: time.Second,
		ControllerURL:  controllerURL,
		MihomoHome:     t.TempDir(),
		Subscriptions:  []config.Subscription{subscription},
	}
	application := &Application{
		cfg:        cfg,
		controller: mihomo.NewController(controllerURL, time.Second),
		mux:        http.NewServeMux(),
	}
	application.routes()
	return application
}

func newProviderController(t *testing.T, updateStatuses map[string]int) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/providers/proxies" {
			_ = json.NewEncoder(w).Encode(map[string]any{"providers": map[string]any{}})
			return
		}
		if r.Method == http.MethodPut && strings.HasPrefix(r.URL.Path, "/providers/proxies/") {
			name := strings.TrimPrefix(r.URL.Path, "/providers/proxies/")
			status, ok := updateStatuses[name]
			if !ok {
				status = http.StatusNotFound
			}
			w.WriteHeader(status)
			return
		}
		http.NotFound(w, r)
	}))
}

func writeProviderCache(t *testing.T, home, name, nodeName string) {
	t.Helper()
	directory := filepath.Join(home, "providers")
	if err := os.MkdirAll(directory, 0o755); err != nil {
		t.Fatal(err)
	}
	content := "proxies:\n  - name: " + nodeName + "\n    type: ss\n    server: example.com\n    port: 443\n    cipher: aes-128-gcm\n    password: test\n"
	if err := os.WriteFile(filepath.Join(directory, name+".yaml"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func writeProviderSource(t *testing.T, path, nodeName string) {
	t.Helper()
	content := "proxies:\n  - name: " + nodeName + "\n    type: ss\n    server: example.com\n    port: 443\n    cipher: aes-128-gcm\n    password: test\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
