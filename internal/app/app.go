package app

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/kagura-ririku/subconverter-mihomo/internal/config"
	"github.com/kagura-ririku/subconverter-mihomo/internal/mihomo"
	"github.com/kagura-ririku/subconverter-mihomo/internal/model"
	"github.com/kagura-ririku/subconverter-mihomo/internal/nodes"
	"github.com/kagura-ririku/subconverter-mihomo/internal/remoteconfig"
	"github.com/kagura-ririku/subconverter-mihomo/internal/render"
)

type Application struct {
	cfg          *config.Config
	controller   *mihomo.Controller
	mux          *http.ServeMux
	refreshGates sync.Map
}

type externalConfigResult struct {
	externalConfig *remoteconfig.ExternalConfig
	err            error
}

const providerMetadataTimeout = 2 * time.Second

func New(cfg *config.Config) (*Application, error) {
	if err := mihomo.WriteRuntimeConfig(cfg); err != nil {
		return nil, fmt.Errorf("write mihomo config: %w", err)
	}

	controller := mihomo.NewController(cfg.ControllerURL, cfg.RequestTimeout)
	waitCtx, cancel := context.WithTimeout(context.Background(), cfg.ControllerStartupTimeout)
	defer cancel()
	if err := controller.WaitReady(waitCtx); err != nil {
		return nil, fmt.Errorf("wait for mihomo controller: %w", err)
	}
	configContent, err := os.ReadFile(filepath.Join(cfg.MihomoHome, "config.yaml"))
	if err != nil {
		return nil, fmt.Errorf("read mihomo config for reload: %w", err)
	}
	reloadController := mihomo.NewController(cfg.ControllerURL, cfg.ControllerStartupTimeout)
	reloadCtx, reloadCancel := context.WithTimeout(context.Background(), cfg.ControllerStartupTimeout)
	defer reloadCancel()
	if err := reloadController.ReloadConfig(reloadCtx, configContent); err != nil {
		return nil, fmt.Errorf("reload mihomo config: %w", err)
	}

	app := &Application{
		cfg:        cfg,
		controller: controller,
		mux:        http.NewServeMux(),
	}
	app.routes()
	return app, nil
}

func (a *Application) Handler() http.Handler {
	return a.mux
}

func (a *Application) routes() {
	a.mux.HandleFunc("/healthz", a.handleHealth)
	a.mux.HandleFunc("/readyz", a.handleReady)
	a.mux.HandleFunc("/", a.handleSubscription)
}

func (a *Application) handleHealth(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}

func (a *Application) handleReady(w http.ResponseWriter, r *http.Request) {
	if err := a.controller.Version(r.Context()); err != nil {
		http.Error(w, "mihomo not ready", http.StatusServiceUnavailable)
		return
	}
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ready"))
}

func (a *Application) handleSubscription(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/" {
		http.NotFound(w, r)
		return
	}
	if r.URL.RawQuery != "" {
		http.Error(w, "query parameters are not supported", http.StatusBadRequest)
		return
	}
	if !a.allowHost(r.Host) {
		http.Error(w, "forbidden host", http.StatusForbidden)
		return
	}

	uuid, format, ok := parseSubscriptionPath(r.URL.Path)
	if !ok {
		http.NotFound(w, r)
		return
	}
	if format != "clash" {
		http.Error(w, "unsupported subscription format", http.StatusBadRequest)
		return
	}

	subscription := a.cfg.FindSubscription(uuid)
	if subscription == nil {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), a.cfg.RequestTimeout)
	defer cancel()

	externalConfigCh := make(chan externalConfigResult, 1)
	go func() {
		externalConfig, err := remoteconfig.Load(ctx, a.cfg, subscription.RemoteConfig)
		externalConfigCh <- externalConfigResult{
			externalConfig: externalConfig,
			err:            err,
		}
	}()

	rawProviders, providerNames, err := a.refreshProviders(ctx, subscription)
	if err != nil {
		if !a.writeContextError(w, err, "upstream refresh timed out") {
			http.Error(w, "failed to refresh upstream providers", http.StatusBadGateway)
		}
		return
	}
	if err := ctx.Err(); err != nil {
		a.writeContextError(w, err, "upstream refresh timed out")
		return
	}
	if len(providerNames) == 0 {
		http.Error(w, "no usable upstream providers", http.StatusBadGateway)
		return
	}

	var externalResult externalConfigResult
	select {
	case externalResult = <-externalConfigCh:
	case <-ctx.Done():
		a.writeContextError(w, ctx.Err(), "request timed out")
		return
	}
	if externalResult.err != nil {
		if a.writeContextError(w, externalResult.err, "request timed out") {
			return
		}
		http.Error(w, "invalid remote config: "+externalResult.err.Error(), http.StatusBadRequest)
		return
	}
	externalConfig := externalResult.externalConfig

	allNodes := nodes.Build(rawProviders)
	providers, metadataErr := a.readProviderMetadata(ctx)
	if metadataErr != nil {
		log.Printf("subscription %q: read provider metadata: %v", subscription.Name, metadataErr)
	}
	if err := ctx.Err(); err != nil {
		a.writeContextError(w, err, "request timed out")
		return
	}
	userInfo := nodes.SelectSubscriptionInfo(providers, providerNames)

	extraRules := nodes.ExtraRules{}
	if externalConfig != nil {
		extraRules = externalConfig.NodeRules
	}

	finalNodes, err := nodes.Apply(allNodes, a.cfg, subscription, extraRules)
	if err != nil {
		http.Error(w, "failed to process nodes", http.StatusInternalServerError)
		return
	}

	compiled, err := remoteconfig.Compile(ctx, a.cfg, externalConfig, finalNodes)
	if err != nil {
		if a.writeContextError(w, err, "request timed out") {
			return
		}
		http.Error(w, "failed to compile remote config: "+err.Error(), http.StatusBadRequest)
		return
	}
	if err := ctx.Err(); err != nil {
		a.writeContextError(w, err, "request timed out")
		return
	}

	body, err := render.Clash(compiled, finalNodes)
	if err != nil {
		http.Error(w, "failed to render config", http.StatusInternalServerError)
		return
	}

	filename := subscription.Name

	w.Header().Set("Content-Type", "text/yaml; charset=utf-8")
	w.Header().Set("Content-Disposition", contentDisposition(filename))
	if userInfo != nil {
		w.Header().Set("subscription-userinfo", userInfo.HeaderValue())
	}
	_, _ = w.Write(body)
}

func (a *Application) readProviderMetadata(ctx context.Context) (map[string]model.ProviderMetadata, error) {
	metadataCtx, cancel := context.WithTimeout(ctx, providerMetadataTimeout)
	defer cancel()
	return a.controller.Providers(metadataCtx)
}

// writeContextError handles request cancellation without trying to write to a
// connection that the client has already closed. It returns true when the
// caller should stop processing the request.
func (a *Application) writeContextError(w http.ResponseWriter, err error, timeoutMessage string) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.Canceled) {
		return true
	}
	if errors.Is(err, context.DeadlineExceeded) {
		http.Error(w, timeoutMessage, http.StatusGatewayTimeout)
		return true
	}
	return false
}

func (a *Application) allowHost(hostport string) bool {
	if len(a.cfg.AllowedHosts) == 0 {
		return true
	}
	host := hostport
	if parsedHost, _, err := net.SplitHostPort(hostport); err == nil {
		host = parsedHost
	}
	host = strings.ToLower(strings.TrimSpace(host))
	for _, allowed := range a.cfg.AllowedHosts {
		if strings.EqualFold(host, strings.TrimSpace(allowed)) {
			return true
		}
	}
	return false
}

func (a *Application) updateProvidersConcurrently(ctx context.Context, providerNames []string) map[string]error {
	var wg sync.WaitGroup
	var mu sync.Mutex
	failures := map[string]error{}
	updateTimeout := a.providerUpdateTimeout()

	for _, providerName := range providerNames {
		providerName := providerName
		wg.Add(1)
		go func() {
			defer wg.Done()
			updateCtx := ctx
			cancel := func() {}
			if updateTimeout > 0 {
				updateCtx, cancel = context.WithTimeout(ctx, updateTimeout)
			}
			defer cancel()
			if err := a.controller.UpdateProvider(updateCtx, providerName); err != nil {
				mu.Lock()
				failures[providerName] = err
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	return failures
}

func (a *Application) providerUpdateTimeout() time.Duration {
	if a.cfg == nil {
		return 0
	}
	timeout := a.cfg.UpstreamTimeout
	if timeout <= 0 {
		timeout = a.cfg.RequestTimeout
	}
	return timeout
}

func (a *Application) refreshProviders(ctx context.Context, subscription *config.Subscription) (map[string][]map[string]any, []string, error) {
	release, err := a.acquireRefreshGate(ctx, subscription.UUID)
	if err != nil {
		return nil, nil, err
	}
	defer release()

	providerNames := subscription.ProviderNames()
	localFailures := mihomo.SyncLocalProviders(a.cfg, subscription)
	a.logProviderFailures(subscription.Name, "load local provider", providerNames, localFailures)
	providerNames = providerNamesWithoutFailures(providerNames, localFailures)

	remoteProviderNames := make([]string, 0, len(providerNames))
	for _, providerName := range providerNames {
		upstream := subscription.FindUpstreamByProvider(providerName)
		if upstream == nil || !upstream.IsLocal() {
			remoteProviderNames = append(remoteProviderNames, providerName)
		}
	}
	updateFailures := a.updateProvidersConcurrently(ctx, remoteProviderNames)
	a.logProviderFailures(subscription.Name, "update provider", subscription.ProviderNames(), updateFailures)
	providerNames = providerNamesWithoutFailures(providerNames, updateFailures)
	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}
	if len(providerNames) == 0 {
		return nil, nil, nil
	}

	rawProviders, cacheFailures := mihomo.LoadRawProviders(a.cfg, providerNames)
	a.logProviderFailures(subscription.Name, "read provider cache", providerNames, cacheFailures)
	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}
	providerNames = providerNamesWithoutFailures(providerNames, cacheFailures)
	return rawProviders, providerNames, nil
}

func (a *Application) acquireRefreshGate(ctx context.Context, subscriptionID string) (func(), error) {
	value, _ := a.refreshGates.LoadOrStore(subscriptionID, make(chan struct{}, 1))
	gate := value.(chan struct{})
	select {
	case gate <- struct{}{}:
		return func() { <-gate }, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (a *Application) logProviderFailures(subscriptionName, operation string, providerNames []string, failures map[string]error) {
	for _, providerName := range providerNames {
		if err, ok := failures[providerName]; ok {
			log.Printf("subscription %q: skip provider %q: %s: %v", subscriptionName, providerName, operation, err)
		}
	}
}

func providerNamesWithoutFailures(providerNames []string, failures map[string]error) []string {
	result := make([]string, 0, len(providerNames))
	for _, providerName := range providerNames {
		if _, failed := failures[providerName]; !failed {
			result = append(result, providerName)
		}
	}
	return result
}

func contentDisposition(filename string) string {
	escaped := url.PathEscape(filename)
	return "attachment; filename*=UTF-8''" + escaped
}

func parseSubscriptionPath(requestPath string) (uuid string, format string, ok bool) {
	cleaned := strings.Trim(path.Clean(requestPath), "/")
	if cleaned == "" || cleaned == "." {
		return "", "", false
	}

	parts := strings.Split(cleaned, "/")
	switch len(parts) {
	case 1:
		return parts[0], "clash", true
	case 2:
		return parts[0], strings.ToLower(strings.TrimSpace(parts[1])), true
	default:
		return "", "", false
	}
}

func init() {
	log.SetFlags(log.LstdFlags | log.Lmicroseconds)
}
