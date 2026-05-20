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

type Runtime struct {
	mu sync.RWMutex

	Config   *config.Service
	Snapshot *config.Snapshot

	BackendStatus map[string]*BackendStatus

	LoadBalancer lb.LoadBalancer
	Metrics      *metrics.Metrics
	HTTPClient   *http.Client

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

	rt := &Runtime{
		Config:        cfgService,
		Snapshot:      snapshot,
		Metrics:       metrics.NewMetrics(),
		BackendStatus: make(map[string]*BackendStatus),
		StartTime:     time.Now(),
	}

	for _, b := range snapshot.Raw.Backends {
		rt.Metrics.Backends.Register(b.URL)
	}

	rt.initBackendStatus(snapshot)
	rt.rebuildHTTPClient(snapshot)
	rt.rebuildLoadBalancer(snapshot)

	return rt, nil
}

func (rt *Runtime) rebuildHTTPClient(snapshot *config.Snapshot) {
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

	rt.HTTPClient = &http.Client{
		Transport: transport,
		Timeout:   time.Duration(t.ResponseTimeoutMs) * time.Millisecond,
	}
}

func (rt *Runtime) initBackendStatus(snapshot *config.Snapshot) {
	for _, b := range snapshot.Raw.Backends {
		rt.BackendStatus[b.URL] = &BackendStatus{}
		rt.BackendStatus[b.URL].Active.Store(false)
		rt.BackendStatus[b.URL].ErrorCount.Store(0)
		rt.BackendStatus[b.URL].SetLastError("")
		rt.BackendStatus[b.URL].LastHealthCheck.Store(0)
	}
}

func (rt *Runtime) rebuildLoadBalancer(snapshot *config.Snapshot) {
	rt.LoadBalancer = lb.GetLoadBalancer(snapshot.Raw.LBStrategy, rt.Metrics)
}

func (rt *Runtime) RefreshFromConfig() error {
	rt.mu.Lock()
	defer rt.mu.Unlock()

	snapshot := rt.Config.Snapshot()
	rt.Snapshot = snapshot

	rt.syncBackendStatus(snapshot)
	rt.rebuildHTTPClient(snapshot)
	rt.rebuildLoadBalancer(snapshot)

	return nil
}

func (rt *Runtime) syncBackendStatus(snapshot *config.Snapshot) {
	next := make(map[string]*BackendStatus, len(snapshot.Raw.Backends))
	for _, b := range snapshot.Raw.Backends {
		if b == nil {
			continue
		}
		if current, ok := rt.BackendStatus[b.URL]; ok {
			next[b.URL] = current
			continue
		}
		next[b.URL] = &BackendStatus{}
	}
	rt.BackendStatus = next
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
	rt.mu.RLock()
	defer rt.mu.RUnlock()

	backends := rt.Snapshot.Raw.Backends
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

		if status := rt.BackendStatus[backend.URL]; status != nil {
			item.Active = status.Active.Load()
			item.ErrorCount = status.ErrorCount.Load()
			item.LastError = status.GetLastError()
		}

		resp = append(resp, item)
	}

	return resp
}

func (rt *Runtime) GetBackend(url string) *config.BackendResponse {
	rt.mu.RLock()
	defer rt.mu.RUnlock()

	backend := rt.Snapshot.BackendsByURL[url]
	if backend == nil {
		return nil
	}

	resp := &config.BackendResponse{
		URL:     backend.URL,
		Weight:  backend.Weight,
		Enabled: backend.Enabled,
	}

	if status := rt.BackendStatus[url]; status != nil {
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
	rt.mu.RLock()
	defer rt.mu.RUnlock()

	raw := rt.Snapshot.Raw
	return config.GlobalConfigResponse{
		ProxyPort:  raw.ProxyPort,
		LBStrategy: raw.LBStrategy,
	}
}

func (rt *Runtime) GetVirtualHosts() []config.VirtualHostResponse {
	rt.mu.RLock()
	defer rt.mu.RUnlock()

	resp := make([]config.VirtualHostResponse, 0, len(rt.Snapshot.Raw.VirtualHosts))
	for _, v := range rt.Snapshot.Raw.VirtualHosts {
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
	rt.mu.RLock()
	defer rt.mu.RUnlock()

	if idx := len(host); idx > 0 {
		for i, c := range host {
			if c == ':' {
				host = host[:i]
				break
			}
		}
		_ = idx
	}

	vhost := rt.Snapshot.VHostsByDomain[host]
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
	rt.mu.RLock()
	defer rt.mu.RUnlock()

	vhost := rt.Snapshot.VHostsByDomain[domain]
	if vhost == nil || vhost.Security == nil {
		return nil
	}

	resp := config.SecurityConfigResponse{
		RateLimiting: vhost.Security.RateLimiting,
	}
	return &resp
}

func (rt *Runtime) GetVirtualHost(host string) *config.VirtualHostResponse {
	rt.mu.RLock()
	defer rt.mu.RUnlock()

	vhost := rt.Snapshot.VHostsByDomain[host]
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
	rt.mu.RLock()
	defer rt.mu.RUnlock()
	return rt.Snapshot
}
