package handler

import (
	"edge-proxy/internal/health"
	"edge-proxy/internal/logger"
	"edge-proxy/internal/proxy/runtime"
	"encoding/json"
	"net/http"
)

func PublicHealthHandler(rt *runtime.Runtime) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		status := "healthy"
		statusCode := http.StatusOK
		if activeBackendCount(rt) == 0 {
			status = "unhealthy"
			statusCode = http.StatusServiceUnavailable
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(statusCode)
		if err := json.NewEncoder(w).Encode(map[string]string{"status": status}); err != nil {
			logger.Error("Failed to encode public health response", map[string]interface{}{
				"error": err.Error(),
			})
		}
	}
}

func HealthHandler(rt *runtime.Runtime) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		logger.Debug("Health endpoint accessed", map[string]interface{}{
			"remote_addr": r.RemoteAddr,
			"user_agent":  r.UserAgent(),
		})

		current := rt.State()
		backends := current.Backends()

		activeCount := 0
		backendStatuses := make([]health.BackendStatus, 0, len(backends))
		for _, b := range backends {
			if b.Enabled && b.Active {
				activeCount++
			}

			currentConnections := int32(0)
			if bm := rt.Metrics.Backends.Get(b.URL); bm != nil {
				currentConnections = int32(bm.ActiveConnections)
			}
			lastHealthCheck := int64(0)
			if status, ok := current.BackendStatus(b.Id); ok {
				lastHealthCheck = status.LastHealthCheck.Load()
			}

			backendStatuses = append(backendStatuses, health.BackendStatus{
				URL:                b.URL,
				Enabled:            b.Enabled,
				Active:             b.Active,
				ErrorCount:         b.ErrorCount,
				LastError:          b.LastError,
				LastHealthCheck:    lastHealthCheck,
				CurrentConnections: currentConnections,
			})
		}

		response := health.HealthResponse{
			Status:      "healthy",
			ActiveCount: activeCount,
			TotalCount:  len(backends),
			Backends:    backendStatuses,
		}

		w.Header().Set("Content-Type", "application/json")
		if activeCount == 0 {
			response.Status = "unhealthy"
			w.WriteHeader(http.StatusServiceUnavailable)
			logger.Warn("Health check reports unhealthy", map[string]interface{}{
				"active_backends": activeCount,
				"total_backends":  len(backends),
			})
		} else {
			w.WriteHeader(http.StatusOK)
			logger.Debug("Health check reports healthy", map[string]interface{}{
				"active_backends": activeCount,
				"total_backends":  len(backends),
			})
		}

		if err := json.NewEncoder(w).Encode(response); err != nil {
			logger.Error("Failed to encode health response", map[string]interface{}{
				"error": err.Error(),
			})
			http.Error(w, "Failed to encode health response", http.StatusInternalServerError)
		}
	}
}

func activeBackendCount(rt *runtime.Runtime) int {
	activeCount := 0
	for _, backend := range rt.GetBackendsResponse() {
		if backend.Enabled && backend.Active {
			activeCount++
		}
	}
	return activeCount
}
