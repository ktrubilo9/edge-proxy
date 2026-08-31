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

	callbackMu                   sync.RWMutex
	onRateLimitUpdate            func(domain string, cfg config.RateLimitingConfig)
	onBackendHealthCheckRequired func(backend config.BackendConfig)
}

type HealthState uint8

const (
	HealthUnknown = iota
	HealthHealthy
	HealthUnhealthy
)

type BackendStatus struct {
	healthState HealthState

	consecutiveFailures uint32
	consecutiveSuccess  uint32

	lastError       string
	lastHealthCheck time.Time
	lastStateChange time.Time

	mu sync.RWMutex
}

type BackendStatusSnapshot struct {
	HealthState          HealthState
	ConsecutiveFailures  uint32
	ConsecutiveSuccesses uint32
	LastError            string
	LastHealthCheck      time.Time
	LastStateChange      time.Time
}

func (s *BackendStatus) Snapshot() BackendStatusSnapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return BackendStatusSnapshot{
		HealthState:          s.healthState,
		ConsecutiveFailures:  s.consecutiveFailures,
		ConsecutiveSuccesses: s.consecutiveSuccess,
		LastError:            s.lastError,
		LastHealthCheck:      s.lastHealthCheck,
		LastStateChange:      s.lastStateChange,
	}
}

func (s *BackendStatus) IsActive() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.healthState == HealthHealthy
}

func (s *BackendStatus) GetLastError() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.lastError
}

func (s *BackendStatus) ApplyProbeResult(
	healthy bool,
	probeErr error,
	htc config.HealthThresholdConfig,
	now time.Time,
) (changed bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	previous := s.healthState
	s.lastHealthCheck = now

	if healthy {
		s.consecutiveFailures = 0
		s.consecutiveSuccess++
		s.lastError = ""
		if s.consecutiveSuccess >= uint32(htc.Healthy) {
			s.healthState = HealthHealthy
		}
	} else {
		s.consecutiveSuccess = 0
		s.consecutiveFailures++
		if probeErr != nil {
			s.lastError = probeErr.Error()
		}
		if s.consecutiveFailures >= uint32(htc.Unhealthy) {
			s.healthState = HealthUnhealthy
		}
	}
	if previous != s.healthState {
		s.lastStateChange = now
		return true
	}
	return false
}

func (s *BackendStatus) Reset() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.healthState = HealthUnknown
	s.consecutiveFailures = 0
	s.consecutiveSuccess = 0
	s.lastError = ""
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

func (state *RuntimeState) BackendStatus(id string) (*BackendStatus, bool) {
	if state == nil {
		return nil, false
	}

	status, ok := state.backendStatuses[id]
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

		// to update
		item := view.BackendResponse{
			Id:      backend.Id,
			URL:     backend.URL,
			Weight:  backend.Weight,
			Enabled: backend.Enabled,
		}

		if status, ok := state.BackendStatus(backend.Id); ok {
			snap := status.Snapshot()
			item.Active = status.IsActive()
			item.ErrorCount = snap.ConsecutiveFailures
			item.LastError = status.GetLastError()
		}

		resp = append(resp, item)
	}

	return resp
}

func (state *RuntimeState) Backend(id string) *view.BackendResponse {
	if state == nil || state.Snapshot == nil {
		return nil
	}

	backend := state.Snapshot.BackendsById[id]
	if backend == nil {
		return nil
	}

	// to update
	resp := &view.BackendResponse{
		Id:      backend.Id,
		URL:     backend.URL,
		Weight:  backend.Weight,
		Enabled: backend.Enabled,
	}
	if status, ok := state.BackendStatus(id); ok {
		snap := status.Snapshot()
		resp.Active = status.IsActive()
		resp.ErrorCount = snap.ConsecutiveFailures
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
		previous.Snapshot.Raw.LoadBalancer.Strategy != snapshot.Raw.LoadBalancer.Strategy {
		balancer = lb.GetLoadBalancer(
			snapshot.Raw.LoadBalancer.Strategy,
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
		nextBackends = next.Snapshot.BackendsById
	}

	for _, backend := range previous.Snapshot.Raw.Backends {
		if backend == nil {
			continue
		}
		if nextBackends == nil || nextBackends[backend.Id] == nil {
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
	if err := rt.applyUpdate(func() error {
		return rt.Config.AddBackend(backend)
	}); err != nil {
		return err
	}

	if backend.Enabled {
		rt.triggerBackendHealthCheck(backend)
	}

	return nil
}

func (rt *Runtime) RemoveBackend(id string) error {
	return rt.applyUpdate(func() error {
		return rt.Config.RemoveBackend(id)
	})
}

func (rt *Runtime) UpdateBackend(id string, url string, weight int32, enabled bool) error {
	previous := rt.Config.GetBackend(id)

	if err := rt.applyUpdate(func() error {
		return rt.Config.UpdateBackend(id, url, weight, enabled)
	}); err != nil {
		return err
	}

	backend := rt.Config.GetBackend(id)
	if backend == nil {
		return nil
	}

	shouldRecheck := backend.Enabled && (previous == nil ||
		!previous.Enabled ||
		previous.URL != backend.URL)

	if shouldRecheck {
		if status, ok := rt.State().BackendStatus(id); ok {
			status.Reset()
		}

		rt.triggerBackendHealthCheck(*backend)
	}

	return nil
}

func (rt *Runtime) GetBackendsResponse() []view.BackendResponse {
	return rt.State().Backends()
}

func (rt *Runtime) GetBackendResponse(id string) *view.BackendResponse {
	return rt.State().Backend(id)
}

func (rt *Runtime) UpdateServerConfig(proxyPort, adminGrpcPort int32) error {
	return rt.applyUpdate(func() error {
		return rt.Config.UpdateServer(int(proxyPort), int(adminGrpcPort))
	})
}

func (rt *Runtime) GetServerConfig() view.ServerConfigResponse {
	raw := rt.State().Snapshot.Raw
	return view.ServerConfigResponse{
		ProxyPort:     raw.Server.ProxyPort,
		AdminGrpcPort: raw.Server.AdminGrpcPort,
	}
}

func (rt *Runtime) UpdateLoadBalancerConfig(strategy string) error {
	return rt.applyUpdate(func() error {
		return rt.Config.UpdateLoadBalancer(strategy)
	})
}

func (rt *Runtime) GetLoadBalancerConfig() view.LoadBalancerConfigResponse {
	raw := rt.State().Snapshot.Raw
	return view.LoadBalancerConfigResponse{
		Strategy: raw.LoadBalancer.Strategy,
	}
}

func (rt *Runtime) GetVirtualHosts() []view.VirtualHostResponse {
	virtualHosts := rt.State().Snapshot.Raw.VirtualHosts
	resp := make([]view.VirtualHostResponse, 0, len(virtualHosts))
	for _, v := range virtualHosts {
		resp = append(resp, view.VirtualHostResponse{
			Domain:           v.Domain,
			BackendIDs:       append([]string(nil), v.BackendIDs...),
			PathRoutes:       append([]config.PathRoute(nil), v.PathRoutes...),
			SecurityPolicyID: v.SecurityPolicyID,
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

	snapshot := rt.State().Snapshot
	vhost := snapshot.VHostsByDomain[host]
	if vhost == nil {
		return view.SecurityConfigResponse{}
	}

	policy := snapshot.PoliciesById[vhost.SecurityPolicyID]
	if policy == nil {
		return view.SecurityConfigResponse{}
	}

	return view.SecurityConfigResponse{
		RateLimiting: policy.RateLimiting,
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

func (rt *Runtime) SetOnBackendHealthCheckRequired(cb func(backend config.BackendConfig)) {
	rt.callbackMu.Lock()
	defer rt.callbackMu.Unlock()
	rt.onBackendHealthCheckRequired = cb
}

func (rt *Runtime) triggerBackendHealthCheck(backend config.BackendConfig) {
	rt.callbackMu.RLock()
	cb := rt.onBackendHealthCheckRequired
	rt.callbackMu.RUnlock()

	if cb != nil {
		cb(backend)
	}
}

func (rt *Runtime) SetVirtualHostSecurityPolicy(domain string, policyID string) error {
	if err := rt.applyUpdate(func() error {
		return rt.Config.SetVirtualHostSecurityPolicy(domain, policyID)
	}); err != nil {
		return err
	}

	security := rt.GetSecurityConfigHost(domain)
	rt.callbackMu.RLock()
	callback := rt.onRateLimitUpdate
	rt.callbackMu.RUnlock()
	if callback != nil {
		callback(domain, security.RateLimiting)
	}

	return nil
}

func (rt *Runtime) UpsertPolicy(policy config.SecurityPolicy) error {
	if err := rt.applyUpdate(func() error {
		return rt.Config.UpsertPolicy(policy)
	}); err != nil {
		return err
	}

	rt.callbackMu.RLock()
	callback := rt.onRateLimitUpdate
	rt.callbackMu.RUnlock()
	if callback != nil {
		for _, vhost := range rt.State().Snapshot.Raw.VirtualHosts {
			if vhost.SecurityPolicyID == policy.Id {
				callback(vhost.Domain, policy.RateLimiting)
			}
		}
	}

	return nil
}

func (rt *Runtime) GetPolicies() []view.SecurityPolicyResponse {
	policies := rt.State().Snapshot.Raw.Security.Policies
	resp := make([]view.SecurityPolicyResponse, 0, len(policies))
	for _, policy := range policies {
		resp = append(resp, view.SecurityPolicyResponse{
			Id:           policy.Id,
			RateLimiting: policy.RateLimiting,
		})
	}
	return resp
}

func (rt *Runtime) GetVirtualHostSecurity(domain string) *view.VirtualHostSecurityResponse {
	snapshot := rt.State().Snapshot
	vhost := snapshot.VHostsByDomain[domain]
	if vhost == nil {
		return nil
	}

	policy := snapshot.PoliciesById[vhost.SecurityPolicyID]
	if policy == nil {
		return nil
	}

	resp := view.VirtualHostSecurityResponse{
		Domain:           vhost.Domain,
		SecurityPolicyID: vhost.SecurityPolicyID,
		Policy: view.SecurityPolicyResponse{
			Id:           policy.Id,
			RateLimiting: policy.RateLimiting,
		},
	}
	return &resp
}

func (rt *Runtime) GetVirtualHost(host string) *view.VirtualHostResponse {
	vhost := rt.State().Snapshot.VHostsByDomain[host]
	if vhost == nil {
		return nil
	}

	return &view.VirtualHostResponse{
		Domain:           vhost.Domain,
		BackendIDs:       append([]string(nil), vhost.BackendIDs...),
		PathRoutes:       append([]config.PathRoute(nil), vhost.PathRoutes...),
		SecurityPolicyID: vhost.SecurityPolicyID,
	}
}

func (rt *Runtime) SnapshotView() *config.Snapshot {
	return rt.State().Snapshot
}

func (rt *Runtime) GetBackend(id string) *config.BackendConfig {
	return rt.Config.GetBackend(id)
}

func (rt *Runtime) GetBackends() []*config.BackendConfig {
	return rt.State().Snapshot.Raw.Backends
}

func (rt *Runtime) GetHealthConfig() config.HealthCheckConfig {
	return rt.State().Snapshot.Raw.HealthCheck
}

func (rt *Runtime) GetBackendStatus(id string) (*BackendStatus, bool) {
	state := rt.State()

	status, ok := state.BackendStatus(id)
	if !ok {
		return nil, false
	}

	return status, true

}
