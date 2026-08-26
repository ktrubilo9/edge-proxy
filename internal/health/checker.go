package health

import (
	"context"
	"edge-proxy/internal/config"
	"edge-proxy/internal/logger"
	"edge-proxy/internal/metrics"
	"edge-proxy/internal/proxy/runtime"
	"fmt"
	"net/http"
	"sync"

	"time"
)

type HealthChecker struct {
	state    *runtime.Runtime
	client   *http.Client
	stopChan chan struct{}
	stopOnce sync.Once
}

func NewHealthChecker(state *runtime.Runtime, config *config.HealthCheckConfig) *HealthChecker {
	return &HealthChecker{
		state:    state,
		client:   &http.Client{},
		stopChan: make(chan struct{}),
	}
}

func (hc *HealthChecker) Start(metrics *metrics.Metrics) {
	healthConfig := hc.state.SnapshotView().Raw.HealthCheck
	logger.Info("Health checker started", map[string]interface{}{
		"interval_sec":      healthConfig.IntervalSeconds,
		"timeout_sec":       healthConfig.TimeoutSeconds,
		"healthy_threshold": healthConfig.HealthyThreshold,
		"path":              healthConfig.Path,
	})

	go func() {
		for {
			intervalSeconds := hc.state.SnapshotView().Raw.HealthCheck.IntervalSeconds

			select {
			case <-time.After(time.Duration(intervalSeconds) * time.Second):
				hc.checkBackends()
			case <-hc.stopChan:
				logger.Info("Health checker stopped", nil)
				return
			}
		}
	}()
}

func (hc *HealthChecker) Stop() {
	logger.Debug("Stopping health checker", nil)
	hc.stopOnce.Do(func() {
		close(hc.stopChan)
	})
}

func (hc *HealthChecker) checkBackends() {
	current := hc.state.State()
	backends := current.Snapshot.Raw.Backends
	for _, b := range backends {
		if b == nil || !b.Enabled {
			continue
		}
		hc.checkBackend(current, b)
	}
}

func (hc *HealthChecker) CheckBackend(b *config.BackendConfig) {
	if b == nil || !b.Enabled {
		return
	}

	current := hc.state.State()
	hc.checkBackend(current, b)
}

func (hc *HealthChecker) checkBackend(current *runtime.RuntimeState, b *config.BackendConfig) {
	status, ok := current.BackendStatus(b.Id)
	if !ok {
		return
	}
	healthConfig := current.Snapshot.Raw.HealthCheck
	healthy := hc.performHealthCheck(b, status, healthConfig)
	oldActive := status.Active.Load()

	updateBackendStatus(status, healthy, healthConfig)
	newActive := status.Active.Load()

	if oldActive != newActive {
		logger.Info("Backend status changed", map[string]interface{}{
			"backend":     b.URL,
			"old_status":  oldActive,
			"new_status":  newActive,
			"healthy":     healthy,
			"error_count": status.ErrorCount.Load(),
		})
	}

	// Log a periodic state snapshot without spamming every check cycle.
	if time.Now().Unix()%30 == 0 {
		logger.Debug("Backend health status", map[string]interface{}{
			"backend":     b.URL,
			"active":      newActive,
			"healthy":     healthy,
			"error_count": status.ErrorCount.Load(),
			"last_error":  status.GetLastError(),
		})
	}
	status.LastHealthCheck.Store(time.Now().Unix())
}

func (hc *HealthChecker) PerformHealthCheck(b *config.BackendConfig) bool {
	current := hc.state.State()
	status, ok := current.BackendStatus(b.Id)
	if !ok {
		return false
	}

	return hc.performHealthCheck(b, status, current.Snapshot.Raw.HealthCheck)
}

func (hc *HealthChecker) performHealthCheck(
	b *config.BackendConfig,
	status *runtime.BackendStatus,
	healthConfig config.HealthCheckConfig,
) bool {
	url := b.URL + healthConfig.Path

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(healthConfig.TimeoutSeconds)*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	metrics := hc.state.Metrics
	if err != nil {
		recordError(status, fmt.Sprintf("Failed to create request: %v", err))
		metrics.RecordHealthCheck(b.URL, false)
		logger.Debug("Health check request creation failed", map[string]interface{}{
			"backend": b.URL,
			"error":   err.Error(),
		})
		return false
	}

	start := time.Now()
	resp, err := hc.client.Do(req)
	duration := time.Since(start)
	if err != nil {
		recordError(status, fmt.Sprintf("Health check failed: %v", err))
		metrics.RecordHealthCheck(b.URL, false)
		logger.Warn("Health check failed", map[string]interface{}{
			"backend":  b.URL,
			"error":    err.Error(),
			"duration": duration.String(),
		})
		return false
	}
	defer resp.Body.Close()

	if !isSuccessCode(healthConfig, int32(resp.StatusCode)) {
		recordError(status, fmt.Sprintf("Unsuccessful status code: %d", resp.StatusCode))
		metrics.RecordHealthCheck(b.URL, false)
		logger.Warn("Health check returned non-success status", map[string]interface{}{
			"backend":     b.URL,
			"status_code": resp.StatusCode,
			"duration":    duration.String(),
		})
		return false
	}

	status.ErrorCount.Store(0)
	metrics.RecordHealthCheck(b.URL, true)
	status.SetLastError("")

	return true
}

func isSuccessCode(healthConfig config.HealthCheckConfig, statusCode int32) bool {
	for _, code := range healthConfig.SuccessCodes {
		if statusCode == code {
			return true
		}
	}
	return false
}

func recordError(status *runtime.BackendStatus, errMsg string) {
	status.ErrorCount.Add(1)
	status.SetLastError(errMsg)
}

func (hc *HealthChecker) UpdateBackendStatus(b *config.BackendConfig, healthy bool) {
	current := hc.state.State()
	status, ok := current.BackendStatus(b.Id)
	if !ok {
		return
	}

	updateBackendStatus(status, healthy, current.Snapshot.Raw.HealthCheck)
}

func updateBackendStatus(
	status *runtime.BackendStatus,
	healthy bool,
	healthConfig config.HealthCheckConfig,
) {
	currentErrorCount := status.ErrorCount.Load()
	if healthy {
		status.Active.Store(true)
	} else {
		if int32(currentErrorCount) >= healthConfig.HealthyThreshold {
			status.Active.Store(false)
		}
	}
}
