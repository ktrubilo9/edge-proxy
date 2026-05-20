package metrics

import (
	"math"
	"sync"
	"sync/atomic"
	"time"
)

type Metrics struct {
	StartTime time.Time
	stopChan  chan struct{}

	Backends *BackendMetricsManager
	Security *SecurityMetrics
	HTTP     *HTTPMetrics
	Proxy    *ProxyResourceMetrics

	TotalRequests  uint64
	FailedRequests uint64
	TotalLatencyMs uint64

	MinuteRequests uint64
	MinuteErrors   uint64
}

func NewMetrics() *Metrics {
	m := &Metrics{
		StartTime: time.Now(),
		stopChan:  make(chan struct{}),
		Backends:  &BackendMetricsManager{},
		Security:  &SecurityMetrics{},
		HTTP: &HTTPMetrics{
			StatusCodes: struct {
				mu    sync.RWMutex
				items map[int]uint64
			}{items: make(map[int]uint64)},
			MethodsCount: struct {
				mu    sync.RWMutex
				items map[string]uint64
			}{items: make(map[string]uint64)},
		},
		Proxy: &ProxyResourceMetrics{},
	}

	return m
}

func (m *Metrics) RecordRequestEnd(url string, latencyMs float64, isError bool, code int32) {
	atomic.AddUint64(&m.TotalRequests, 1)
	atomic.AddUint64(&m.TotalLatencyMs, uint64(latencyMs))
	atomic.AddUint64(&m.MinuteRequests, 1)

	if isError {
		atomic.AddUint64(&m.FailedRequests, 1)
		atomic.AddUint64(&m.MinuteErrors, 1)
	}

	if bm := m.Backends.Get(url); bm != nil {
		atomic.AddUint64(&bm.Requests, 1)
		if isError {
			atomic.AddUint64(&bm.Failures, 1)
			atomic.StoreInt64(&bm.LastFailureTime, time.Now().UnixNano())

			if code >= 500 {
				atomic.StoreUint32(&bm.ConsecutiveSuccess, 0)
				atomic.AddUint32(&bm.ConsecutiveFailures, 1)
			}
		} else {
			atomic.AddUint32(&bm.ConsecutiveSuccess, 1)
			atomic.StoreUint32(&bm.ConsecutiveFailures, 0)
		}

		updateEWMA(&bm.LatencyEWMABits, latencyMs, 0.2)

		errRate := 0.0
		if isError {
			errRate = 1.0
		}
		updateEWMA(&bm.ErrorRateEWMABits, errRate, 0.08)
	}
}

func (m *Metrics) RecordRequestSize(size int64) {
	atomic.AddUint64(&m.HTTP.RequestSizeTotal, uint64(size))
}

func (m *Metrics) RecordResponseSize(size int64) {
	atomic.AddUint64(&m.HTTP.ResponseSizeTotal, uint64(size))
}

func (m *Metrics) RecordRateLimitAllow() {
	atomic.AddUint64(&m.Security.RateLimitStats.AllowedRequests, 1)
}

func (m *Metrics) RecordRateLimitBlock() {
	atomic.AddUint64(&m.Security.RateLimitStats.BlockedRequests, 1)
}

func (m *Metrics) RecordTimeout(backend string) {
	if bm := m.Backends.Get(backend); bm != nil {
		atomic.AddUint64(&bm.Timeouts, 1)
	}
}

func (m *Metrics) RecordHealthCheck(backend string, success bool) {
	if bm := m.Backends.Get(backend); bm != nil {
		atomic.AddUint64(&bm.HealthChecks, 1)
		if !success {
			atomic.AddUint64(&bm.FailedHealthChecks, 1)
		}
	}
}

func loadFloat64Bits(p *uint64) float64 {
	return math.Float64frombits(atomic.LoadUint64(p))
}

func StoreFloat64Bits(p *uint64, f float64) {
	atomic.StoreUint64(p, math.Float64bits(f))
}

func updateEWMA(p *uint64, newVal float64, alpha float64) {
	for {
		oldBits := atomic.LoadUint64(p)
		oldVal := math.Float64frombits(oldBits)

		var updated float64
		if oldVal == 0 {
			updated = newVal
		} else {
			updated = alpha*newVal + (1.0-alpha)*oldVal
		}

		newBits := math.Float64bits(updated)
		if atomic.CompareAndSwapUint64(p, oldBits, newBits) {
			return
		}
	}
}

func (m *Metrics) GetRequestsPerBackend() map[string]int32 {
	result := make(map[string]int32)
	m.Backends.Range(func(url string, bm *BackendMetrics) bool {
		result[url] = int32(atomic.LoadUint64(&bm.Requests))
		return true
	})
	return result
}

func (m *Metrics) GetFailedRequestsPerBackend() map[string]int32 {
	result := make(map[string]int32)
	m.Backends.Range(func(url string, bm *BackendMetrics) bool {
		result[url] = int32(atomic.LoadUint64(&bm.Failures))
		return true
	})
	return result
}

func (m *Metrics) GetActiveConnectionsPerBackend() map[string]int32 {
	result := make(map[string]int32)
	m.Backends.Range(func(url string, bm *BackendMetrics) bool {
		result[url] = int32(atomic.LoadUint64(&bm.ActiveConnections))
		return true
	})
	return result
}

func (m *Metrics) GetTimeoutsPerBackend() map[string]int32 {
	result := make(map[string]int32)
	m.Backends.Range(func(url string, bm *BackendMetrics) bool {
		result[url] = int32(atomic.LoadUint64(&bm.Timeouts))
		return true
	})
	return result
}

func (m *Metrics) GetHealthChecksPerBackend() map[string]int32 {
	result := make(map[string]int32)
	m.Backends.Range(func(url string, bm *BackendMetrics) bool {
		result[url] = int32(atomic.LoadUint64(&bm.HealthChecks))
		return true
	})
	return result
}

func (m *Metrics) GetFailedHealthChecksPerBackend() map[string]int32 {
	result := make(map[string]int32)
	m.Backends.Range(func(url string, bm *BackendMetrics) bool {
		result[url] = int32(atomic.LoadUint64(&bm.FailedHealthChecks))
		return true
	})
	return result
}

func (m *Metrics) GetAvgLatencyPerBackends() map[string]float64 {
	result := make(map[string]float64)
	m.Backends.Range(func(url string, bm *BackendMetrics) bool {
		bits := atomic.LoadUint64(&bm.LatencyEWMABits)
		result[url] = math.Float64frombits(bits)
		return true
	})
	return result
}

func (m *Metrics) GetStatusCodes() map[int]uint64 {
	m.HTTP.StatusCodes.mu.RLock()
	defer m.HTTP.StatusCodes.mu.RUnlock()

	result := make(map[int]uint64)
	for k, v := range m.HTTP.StatusCodes.items {
		result[int(k)] = uint64(v)
	}
	return result
}

func (m *Metrics) GetMethodsCount() map[string]uint64 {
	m.HTTP.MethodsCount.mu.RLock()
	defer m.HTTP.MethodsCount.mu.RUnlock()

	result := make(map[string]uint64)
	for k, v := range m.HTTP.MethodsCount.items {
		result[k] = uint64(v)
	}
	return result
}

func (m *Metrics) GetUptime() uint64 {
	return uint64(time.Since(m.StartTime).Seconds())
}

func (m *Metrics) GetTotalRequests() uint64 {
	return atomic.LoadUint64(&m.TotalRequests)
}

func (m *Metrics) GetTotalFailedRequests() uint64 {
	return atomic.LoadUint64(&m.FailedRequests)
}

func (m *Metrics) IncrementActiveConnections(url string) {
	if bm := m.Backends.Get(url); bm != nil {
		atomic.AddUint64(&bm.ActiveConnections, 1)
	}
}

func (m *Metrics) DecrementActiveConnections(url string) {
	if bm := m.Backends.Get(url); bm != nil {
		// Use a CAS loop to avoid underflow under concurrent decrements.
		for {
			current := atomic.LoadUint64(&bm.ActiveConnections)
			if current == 0 {
				return
			}
			if atomic.CompareAndSwapUint64(&bm.ActiveConnections, current, current-1) {
				return
			}
		}
	}
}
