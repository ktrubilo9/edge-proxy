package metrics

import (
	"edge-proxy/internal/view"
	"sync/atomic"
	"time"
)

func (m *Metrics) ToSystemMetricsResponse() view.SystemMetricsResponse {

	return view.SystemMetricsResponse{
		Timestamp:     time.Now(),
		Goroutines:    atomic.LoadUint64(&m.Proxy.GoroutineCount),
		MemoryPercent: m.Proxy.MemoryPercent,
		CpuPercent:    m.Proxy.CPUPercent,
	}
}

func (m *Metrics) ToBackendMetricsResponse(url string) view.BackendMetricsResponse {
	bm := m.Backends.Get(url)
	if bm == nil {
		return view.BackendMetricsResponse{URL: url}
	}

	return view.BackendMetricsResponse{
		URL:                url,
		Requests:           atomic.LoadUint64(&bm.Requests),
		Failures:           atomic.LoadUint64(&bm.Failures),
		Timeouts:           atomic.LoadUint64(&bm.Timeouts),
		HealthChecks:       atomic.LoadUint64(&bm.HealthChecks),
		FailedHealthChecks: atomic.LoadUint64(&bm.FailedHealthChecks),
		LatencyEWMA:        loadFloat64Bits(&bm.LatencyEWMABits),
		ErrorRateEWMA:      loadFloat64Bits(&bm.ErrorRateEWMABits),
		CpuPercent:         loadFloat64Bits(&bm.CpuPercentBits) * 100,
		MemoryPercent:      loadFloat64Bits(&bm.MemoryPercentBits) * 100,
		ActiveConnections:  atomic.LoadUint64(&bm.ActiveConnections),
	}
}

func (m *Metrics) ToAllBackendsResponse() view.BackendsMetricsResponse {
	backends := []view.BackendMetricsResponse{}
	m.Backends.Range(func(url string, _ *BackendMetrics) bool {
		backends = append(backends, m.ToBackendMetricsResponse(url))
		return true
	})
	return view.BackendsMetricsResponse{
		Timestamp: time.Now(),
		Backends:  backends,
	}
}

func (m *Metrics) ToSecurityMetricsResponse() view.SecurityMetricsResponse {
	return view.SecurityMetricsResponse{
		AllowedRequests: atomic.LoadUint64(&m.Security.RateLimitStats.AllowedRequests),
		BlockedRequests: atomic.LoadUint64(&m.Security.RateLimitStats.BlockedRequests),
	}
}

func (m *Metrics) ToHTTPMetricsResponse() view.HTTPMetricsResponse {
	m.HTTP.StatusCodes.mu.RLock()
	m.HTTP.MethodsCount.mu.RLock()
	defer m.HTTP.StatusCodes.mu.RUnlock()
	defer m.HTTP.MethodsCount.mu.RUnlock()

	statusCodes := make(map[int32]uint64, len(m.HTTP.StatusCodes.items))
	for k, v := range m.HTTP.StatusCodes.items {
		statusCodes[int32(k)] = v
	}

	methods := make(map[string]uint64, len(m.HTTP.MethodsCount.items))
	for k, v := range m.HTTP.MethodsCount.items {
		methods[k] = v
	}

	return view.HTTPMetricsResponse{
		TotalRequestSize:  atomic.LoadUint64(&m.HTTP.RequestSizeTotal),
		TotalResponseSize: atomic.LoadUint64(&m.HTTP.ResponseSizeTotal),
		StatusCodes:       statusCodes,
		MethodsCount:      methods,
	}
}
