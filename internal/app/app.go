package app

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/url"
	"path"
	"strings"
	"sync"

	"github.com/kagura-ririku/subconverter-mihomo/internal/config"
	"github.com/kagura-ririku/subconverter-mihomo/internal/mihomo"
	"github.com/kagura-ririku/subconverter-mihomo/internal/nodes"
	"github.com/kagura-ririku/subconverter-mihomo/internal/remoteconfig"
	"github.com/kagura-ririku/subconverter-mihomo/internal/render"
)

type Application struct {
	cfg        *config.Config
	controller *mihomo.Controller
	mux        *http.ServeMux
}

type externalConfigResult struct {
	externalConfig *remoteconfig.ExternalConfig
	err            error
}

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

	uuid := strings.Trim(path.Clean(r.URL.Path), "/")
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

	providerNames := subscription.ProviderNames()
	a.updateProvidersConcurrently(ctx, providerNames)

	externalConfigResult := <-externalConfigCh
	if externalConfigResult.err != nil {
		http.Error(w, "invalid remote config: "+externalConfigResult.err.Error(), http.StatusBadRequest)
		return
	}
	externalConfig := externalConfigResult.externalConfig

	providers, err := a.controller.Providers(ctx)
	if err != nil {
		http.Error(w, "failed to read mihomo providers", http.StatusServiceUnavailable)
		return
	}

	rawProviders, err := mihomo.LoadRawProviders(a.cfg, providerNames)
	if err != nil {
		http.Error(w, "failed to read provider cache", http.StatusServiceUnavailable)
		return
	}

	allNodes, err := nodes.BuildFromRawProviders(rawProviders)
	if err != nil {
		http.Error(w, "failed to parse nodes", http.StatusInternalServerError)
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
		http.Error(w, "failed to compile remote config: "+err.Error(), http.StatusBadRequest)
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

func (a *Application) updateProvidersConcurrently(ctx context.Context, providerNames []string) {
	var wg sync.WaitGroup
	for _, providerName := range providerNames {
		providerName := providerName
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := a.controller.UpdateProvider(ctx, providerName); err != nil {
				log.Printf("provider update %s failed: %v", providerName, err)
			}
		}()
	}
	wg.Wait()
}

func contentDisposition(filename string) string {
	escaped := url.PathEscape(filename)
	return "attachment; filename*=UTF-8''" + escaped
}

func init() {
	log.SetFlags(log.LstdFlags | log.Lmicroseconds)
}
