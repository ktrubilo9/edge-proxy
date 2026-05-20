package handler

import (
	"edge-proxy/internal/health"
	"edge-proxy/internal/logger"
	"edge-proxy/internal/proxy/runtime"
	"encoding/json"
	"net/http"
)

func HealthHandler(rt *runtime.Runtime) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		logger.Debug("Health endpoint accessed", map[string]interface{}{
			"remote_addr": r.RemoteAddr,
			"user_agent":  r.UserAgent(),
		})

		backends := rt.GetBackends()

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
			if status := rt.BackendStatus[b.URL]; status != nil {
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

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(response); err != nil {
			logger.Error("Failed to encode health response", map[string]interface{}{
				"error": err.Error(),
			})
			http.Error(w, "Failed to encode health response", http.StatusInternalServerError)
		}
	}
}
