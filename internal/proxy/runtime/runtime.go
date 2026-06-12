package runtime

import (
	"edge-proxy/internal/config"
	"edge-proxy/internal/lb"
	"edge-proxy/internal/logger"
	"edge-proxy/internal/metrics"
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
}

type Runtime struct {
	updateMu sync.Mutex

	Config   *config.Service
	Metrics  *metrics.Metrics
	statuses *BackendRegistry

	state             atomic.Pointer[RuntimeState]
	StartTime         time.Time
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

	rt.statuses.Reconcile(snapshot.Raw.Backends)
	rt.state.Store(rt.buildRuntimeState(nil, snapshot))

	return rt, nil
}

func (rt *Runtime) buildRuntimeState(previous *RuntimeState, snapshot *config.Snapshot) *RuntimeState {
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
		Snapshot:     snapshot,
		HTTPClient:   client,
		LoadBalancer: balancer,
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

func (rt *Runtime) RefreshFromConfig() error {
	rt.updateMu.Lock()
	defer rt.updateMu.Unlock()

	snapshot := rt.Config.Snapshot()
	current := rt.state.Load()
	next := rt.buildRuntimeState(current, snapshot)
	rt.statuses.Reconcile(snapshot.Raw.Backends)

	previous := rt.state.Swap(next)
	closeReplacedHTTPClient(previous, next)

	return nil
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

func (rt *Runtime) BackendStatus(url string) (*BackendStatus, bool) {
	return rt.statuses.Get(url)
}

func (rt *Runtime) AddBackend(backend config.BackendConfig) error {
	if err := rt.Config.AddBackend(backend); err != nil {
		return err
	}
	rt.Metrics.Backends.Register(backend.URL)
	return rt.RefreshFromConfig()
}

func (rt *Runtime) RemoveBackend(url string) error {
	if err := rt.Config.RemoveBackend(url); err != nil {
		return err
	}
	rt.Metrics.Backends.Deregister(url)
	return rt.RefreshFromConfig()
}

func (rt *Runtime) UpdateBackend(url string, weight int32, enabled bool) error {
	if err := rt.Config.UpdateBackend(url, weight, enabled); err != nil {
		return err
	}
	return rt.RefreshFromConfig()
}

func (rt *Runtime) GetBackends() []config.BackendResponse {
	backends := rt.State().Snapshot.Raw.Backends
	resp := make([]config.BackendResponse, 0, len(backends))
	for _, backend := range backends {
		if backend == nil {
			continue
		}

		item := config.BackendResponse{
			URL:     backend.URL,
			Weight:  backend.Weight,
			Enabled: backend.Enabled,
		}

		if status, ok := rt.BackendStatus(backend.URL); ok {
			item.Active = status.Active.Load()
			item.ErrorCount = status.ErrorCount.Load()
			item.LastError = status.GetLastError()
		}

		resp = append(resp, item)
	}

	return resp
}

func (rt *Runtime) GetBackend(url string) *config.BackendResponse {
	backend := rt.State().Snapshot.BackendsByURL[url]
	if backend == nil {
		return nil
	}

	resp := &config.BackendResponse{
		URL:     backend.URL,
		Weight:  backend.Weight,
		Enabled: backend.Enabled,
	}

	if status, ok := rt.BackendStatus(url); ok {
		resp.Active = status.Active.Load()
		resp.ErrorCount = status.ErrorCount.Load()
		resp.LastError = status.GetLastError()
	}

	return resp
}

func (rt *Runtime) UpdateGlobalConfig(proxyPort int32, strategy string) error {
	if err := rt.Config.UpdateGlobal(int(proxyPort), strategy); err != nil {
		return err
	}
	return rt.RefreshFromConfig()
}

func (rt *Runtime) GetGlobalConfig() config.GlobalConfigResponse {
	raw := rt.State().Snapshot.Raw
	return config.GlobalConfigResponse{
		ProxyPort:  raw.ProxyPort,
		LBStrategy: raw.LBStrategy,
	}
}

func (rt *Runtime) GetVirtualHosts() []config.VirtualHostResponse {
	virtualHosts := rt.State().Snapshot.Raw.VirtualHosts
	resp := make([]config.VirtualHostResponse, 0, len(virtualHosts))
	for _, v := range virtualHosts {
		resp = append(resp, config.VirtualHostResponse{
			Domain:     v.Domain,
			Backends:   append([]string(nil), v.Backends...),
			PathRoutes: append([]config.PathRoute(nil), v.PathRoutes...),
			Security:   v.Security,
		})
	}
	return resp
}

func (rt *Runtime) AddVirtualHost(vhost config.VirtualHost) error {
	if err := rt.Config.AddVirtualHost(vhost); err != nil {
		return err
	}
	return rt.RefreshFromConfig()
}

func (rt *Runtime) RemoveVirtualHost(domain string) error {
	if err := rt.Config.RemoveVirtualHost(domain); err != nil {
		return err
	}
	return rt.RefreshFromConfig()
}

func (rt *Runtime) GetSecurityConfigHost(host string) config.SecurityConfigResponse {
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
		return config.SecurityConfigResponse{}
	}

	return config.SecurityConfigResponse{
		RateLimiting: vhost.Security.RateLimiting,
	}
}

func (rt *Runtime) UpdateVirtualHost(domain string, vhost config.VirtualHost) error {
	if err := rt.Config.UpdateVirtualHost(domain, vhost); err != nil {
		return err
	}
	return rt.RefreshFromConfig()
}

func (rt *Runtime) SetOnRateLimitUpdate(cb func(domain string, cfg config.RateLimitingConfig)) {
	rt.onRateLimitUpdate = cb
}

func (rt *Runtime) UpdateVirtualHostRateLimiting(domain string, rate config.RateLimitingConfig) error {
	if err := rt.Config.UpdateVirtualHostRateLimiting(domain, rate); err != nil {
		return err
	}
	rt.onRateLimitUpdate(domain, rate)
	return rt.RefreshFromConfig()
}

func (rt *Runtime) GetVirtualHostSecurityConfig(domain string) *config.SecurityConfigResponse {
	vhost := rt.State().Snapshot.VHostsByDomain[domain]
	if vhost == nil || vhost.Security == nil {
		return nil
	}

	resp := config.SecurityConfigResponse{
		RateLimiting: vhost.Security.RateLimiting,
	}
	return &resp
}

func (rt *Runtime) GetVirtualHost(host string) *config.VirtualHostResponse {
	vhost := rt.State().Snapshot.VHostsByDomain[host]
	if vhost == nil {
		return nil
	}

	return &config.VirtualHostResponse{
		Domain:     vhost.Domain,
		Backends:   append([]string(nil), vhost.Backends...),
		PathRoutes: append([]config.PathRoute(nil), vhost.PathRoutes...),
		Security:   vhost.Security,
	}
}

func (rt *Runtime) SnapshotView() *config.Snapshot {
	return rt.State().Snapshot
}
