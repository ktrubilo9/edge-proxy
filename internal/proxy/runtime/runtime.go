package runtime

import (
	"edge-proxy/internal/config"
	"edge-proxy/internal/lb"
	"edge-proxy/internal/logger"
	"edge-proxy/internal/metrics"
	"edge-proxy/internal/view"
	"net"
	"net/http"
	"sync"
	"sync/atomic"

	"time"
)

type RuntimeState struct {
	Snapshot     *config.Snapshot
	HTTPClient   *http.Client
	LoadBalancer lb.LoadBalancer

	backendStatuses map[string]*BackendStatus
}

type Runtime struct {
	updateMu sync.Mutex

	Config   *config.Service
	Metrics  *metrics.Metrics
	statuses *BackendRegistry

	state     atomic.Pointer[RuntimeState]
	StartTime time.Time

	callbackMu        sync.RWMutex
	onRateLimitUpdate func(domain string, cfg config.RateLimitingConfig)
}

type BackendStatus struct {
	Active          atomic.Bool
	ErrorCount      atomic.Uint32
	mu              sync.RWMutex
	LastError       string
	LastHealthCheck atomic.Int64
}

func (bs *BackendStatus) SetLastError(err string) {
	bs.mu.Lock()
	defer bs.mu.Unlock()
	bs.LastError = err
}

func (bs *BackendStatus) GetLastError() string {
	bs.mu.RLock()
	defer bs.mu.RUnlock()
	return bs.LastError
}

func NewRuntime(configPath string) (*Runtime, error) {
	cfgService, err := config.NewService(configPath)
	if err != nil {
		return nil, err
	}

	snapshot := cfgService.Snapshot()
	runtimeMetrics := metrics.NewMetrics()

	rt := &Runtime{
		Config:    cfgService,
		Metrics:   runtimeMetrics,
		statuses:  NewBackendRegistry(),
		StartTime: time.Now(),
	}

	for _, b := range snapshot.Raw.Backends {
		rt.Metrics.Backends.Register(b.URL)
	}

	statuses := rt.statuses.Reconcile(snapshot.Raw.Backends)
	initial := rt.buildRuntimeState(nil, snapshot, statuses)

	rt.state.Store(initial)

	return rt, nil
}

func (state *RuntimeState) BackendStatus(url string) (*BackendStatus, bool) {
	if state == nil {
		return nil, false
	}

	status, ok := state.backendStatuses[url]
	return status, ok
}

func (state *RuntimeState) Backends() []view.BackendResponse {
	if state == nil || state.Snapshot == nil || state.Snapshot.Raw == nil {
		return nil
	}

	backends := state.Snapshot.Raw.Backends
	resp := make([]view.BackendResponse, 0, len(backends))
	for _, backend := range backends {
		if backend == nil {
			continue
		}

		item := view.BackendResponse{
			URL:     backend.URL,
			Weight:  backend.Weight,
			Enabled: backend.Enabled,
		}
		if status, ok := state.BackendStatus(backend.URL); ok {
			item.Active = status.Active.Load()
			item.ErrorCount = status.ErrorCount.Load()
			item.LastError = status.GetLastError()
		}

		resp = append(resp, item)
	}

	return resp
}

func (state *RuntimeState) Backend(url string) *view.BackendResponse {
	if state == nil || state.Snapshot == nil {
		return nil
	}

	backend := state.Snapshot.BackendsByURL[url]
	if backend == nil {
		return nil
	}

	resp := &view.BackendResponse{
		URL:     backend.URL,
		Weight:  backend.Weight,
		Enabled: backend.Enabled,
	}
	if status, ok := state.BackendStatus(url); ok {
		resp.Active = status.Active.Load()
		resp.ErrorCount = status.ErrorCount.Load()
		resp.LastError = status.GetLastError()
	}

	return resp
}

func (rt *Runtime) buildRuntimeState(previous *RuntimeState, snapshot *config.Snapshot, statuses map[string]*BackendStatus) *RuntimeState {
	var client *http.Client
	var balancer lb.LoadBalancer

	if previous != nil {
		client = previous.HTTPClient
		balancer = previous.LoadBalancer
	}

	if previous == nil ||
		previous.Snapshot == nil ||
		previous.HTTPClient == nil ||
		previous.Snapshot.Raw.Timeouts != snapshot.Raw.Timeouts {
		client = buildHTTPClient(snapshot)
	}

	if previous == nil ||
		previous.Snapshot == nil ||
		previous.LoadBalancer == nil ||
		previous.Snapshot.Raw.LBStrategy != snapshot.Raw.LBStrategy {
		balancer = lb.GetLoadBalancer(
			snapshot.Raw.LBStrategy,
			rt.Metrics,
		)
	}

	return &RuntimeState{
		Snapshot:        snapshot,
		HTTPClient:      client,
		LoadBalancer:    balancer,
		backendStatuses: statuses,
	}
}

func buildHTTPClient(snapshot *config.Snapshot) *http.Client {
	t := snapshot.Raw.Timeouts

	logger.Debug("Rebuilding HTTP client", map[string]interface{}{
		"connect_timeout_ms":    t.ConnectTimeoutMs,
		"response_timeout_ms":   t.ResponseTimeoutMs,
		"keep_alive_timeout_ms": t.KeepAliveTimeoutMs,
		"idle_conn_timeout_ms":  t.IdleConnTimeoutMs,
	})

	transport := &http.Transport{
		MaxIdleConns:        10000,
		MaxIdleConnsPerHost: 1000,
		MaxConnsPerHost:     2000,
		DisableKeepAlives:   false,
		DisableCompression:  true,
		ForceAttemptHTTP2:   true,

		IdleConnTimeout:       time.Duration(t.IdleConnTimeoutMs) * time.Millisecond,
		TLSHandshakeTimeout:   time.Duration(t.ConnectTimeoutMs) * time.Millisecond,
		ExpectContinueTimeout: time.Duration(t.ResponseTimeoutMs) * time.Millisecond,

		DialContext: (&net.Dialer{
			Timeout:   time.Duration(t.ConnectTimeoutMs) * time.Millisecond,
			KeepAlive: time.Duration(t.KeepAliveTimeoutMs) * time.Millisecond,
		}).DialContext,
	}

	return &http.Client{
		Transport: transport,
		Timeout:   time.Duration(t.ResponseTimeoutMs) * time.Millisecond,
	}
}

func (rt *Runtime) applyUpdate(mutate func() error) error {
	rt.updateMu.Lock()
	defer rt.updateMu.Unlock()

	if err := mutate(); err != nil {
		return err
	}

	snapshot := rt.Config.Snapshot()
	previous := rt.State()

	statuses := rt.statuses.Reconcile(snapshot.Raw.Backends)
	next := rt.buildRuntimeState(previous, snapshot, statuses)

	rt.registerBackendMetrics(snapshot)

	published := rt.state.Swap(next)

	rt.deregisterRemovedBackendMetrics(published, next)
	closeReplacedHTTPClient(published, next)

	return nil
}

func (rt *Runtime) registerBackendMetrics(snapshot *config.Snapshot) {
	if snapshot == nil || snapshot.Raw == nil {
		return
	}

	for _, backend := range snapshot.Raw.Backends {
		if backend != nil {
			rt.Metrics.Backends.Register(backend.URL)
		}
	}
}

func (rt *Runtime) deregisterRemovedBackendMetrics(previous, next *RuntimeState) {
	if previous == nil || previous.Snapshot == nil || previous.Snapshot.Raw == nil {
		return
	}

	var nextBackends map[string]*config.BackendConfig
	if next != nil && next.Snapshot != nil {
		nextBackends = next.Snapshot.BackendsByURL
	}

	for _, backend := range previous.Snapshot.Raw.Backends {
		if backend == nil {
			continue
		}
		if nextBackends == nil || nextBackends[backend.URL] == nil {
			rt.Metrics.Backends.Deregister(backend.URL)
		}
	}
}

func closeReplacedHTTPClient(previous, next *RuntimeState) {
	if previous == nil || previous.HTTPClient == nil {
		return
	}
	if next != nil && previous.HTTPClient == next.HTTPClient {
		return
	}

	previous.HTTPClient.CloseIdleConnections()
}

func (rt *Runtime) State() *RuntimeState {
	return rt.state.Load()
}

func (rt *Runtime) AddBackend(backend config.BackendConfig) error {
	return rt.applyUpdate(func() error {
		return rt.Config.AddBackend(backend)
	})
}

func (rt *Runtime) RemoveBackend(url string) error {
	return rt.applyUpdate(func() error {
		return rt.Config.RemoveBackend(url)
	})
}

func (rt *Runtime) UpdateBackend(url string, weight int32, enabled bool) error {
	return rt.applyUpdate(func() error {
		return rt.Config.UpdateBackend(url, weight, enabled)
	})
}

func (rt *Runtime) GetBackends() []view.BackendResponse {
	return rt.State().Backends()
}

func (rt *Runtime) GetBackend(url string) *view.BackendResponse {
	return rt.State().Backend(url)
}

func (rt *Runtime) UpdateGlobalConfig(proxyPort int32, strategy string) error {
	return rt.applyUpdate(func() error {
		return rt.Config.UpdateGlobal(int(proxyPort), strategy)
	})
}

func (rt *Runtime) GetGlobalConfig() view.GlobalConfigResponse {
	raw := rt.State().Snapshot.Raw
	return view.GlobalConfigResponse{
		ProxyPort:  raw.ProxyPort,
		LBStrategy: raw.LBStrategy,
	}
}

func (rt *Runtime) GetVirtualHosts() []view.VirtualHostResponse {
	virtualHosts := rt.State().Snapshot.Raw.VirtualHosts
	resp := make([]view.VirtualHostResponse, 0, len(virtualHosts))
	for _, v := range virtualHosts {
		resp = append(resp, view.VirtualHostResponse{
			Domain:     v.Domain,
			Backends:   append([]string(nil), v.Backends...),
			PathRoutes: append([]config.PathRoute(nil), v.PathRoutes...),
			Security:   v.Security,
		})
	}
	return resp
}

func (rt *Runtime) AddVirtualHost(vhost config.VirtualHost) error {
	return rt.applyUpdate(func() error {
		return rt.Config.AddVirtualHost(vhost)
	})
}

func (rt *Runtime) RemoveVirtualHost(domain string) error {
	return rt.applyUpdate(func() error {
		return rt.Config.RemoveVirtualHost(domain)
	})
}

func (rt *Runtime) GetSecurityConfigHost(host string) view.SecurityConfigResponse {
	if idx := len(host); idx > 0 {
		for i, c := range host {
			if c == ':' {
				host = host[:i]
				break
			}
		}
		_ = idx
	}

	vhost := rt.State().Snapshot.VHostsByDomain[host]
	if vhost == nil || vhost.Security == nil {
		return view.SecurityConfigResponse{}
	}

	return view.SecurityConfigResponse{
		RateLimiting: vhost.Security.RateLimiting,
	}
}

func (rt *Runtime) UpdateVirtualHost(domain string, vhost config.VirtualHost) error {
	return rt.applyUpdate(func() error {
		return rt.Config.UpdateVirtualHost(domain, vhost)
	})
}

func (rt *Runtime) SetOnRateLimitUpdate(cb func(domain string, cfg config.RateLimitingConfig)) {
	rt.callbackMu.Lock()
	defer rt.callbackMu.Unlock()
	rt.onRateLimitUpdate = cb
}

func (rt *Runtime) UpdateVirtualHostRateLimiting(domain string, rate config.RateLimitingConfig) error {
	if err := rt.applyUpdate(func() error {
		return rt.Config.UpdateVirtualHostRateLimiting(domain, rate)
	}); err != nil {
		return err
	}

	rt.callbackMu.RLock()
	callback := rt.onRateLimitUpdate
	rt.callbackMu.RUnlock()
	if callback != nil {
		callback(domain, rate)
	}

	return nil
}

func (rt *Runtime) GetVirtualHostSecurityConfig(domain string) *view.SecurityConfigResponse {
	vhost := rt.State().Snapshot.VHostsByDomain[domain]
	if vhost == nil || vhost.Security == nil {
		return nil
	}

	resp := view.SecurityConfigResponse{
		RateLimiting: vhost.Security.RateLimiting,
	}
	return &resp
}

func (rt *Runtime) GetVirtualHost(host string) *view.VirtualHostResponse {
	vhost := rt.State().Snapshot.VHostsByDomain[host]
	if vhost == nil {
		return nil
	}

	return &view.VirtualHostResponse{
		Domain:     vhost.Domain,
		Backends:   append([]string(nil), vhost.Backends...),
		PathRoutes: append([]config.PathRoute(nil), vhost.PathRoutes...),
		Security:   vhost.Security,
	}
}

func (rt *Runtime) SnapshotView() *config.Snapshot {
	return rt.State().Snapshot
}
