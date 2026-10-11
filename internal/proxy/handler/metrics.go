package handler

import (
	"encoding/json"
	"net/http"

	"github.com/ktrubilo9/edge-proxy/internal/logger"
	"github.com/ktrubilo9/edge-proxy/internal/proxy/runtime"
)

func MetricsHandler(rt *runtime.Runtime) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		logger.Debug("Metrics endpoint accessed", map[string]interface{}{
			"remote_addr": r.RemoteAddr,
			"user_agent":  r.UserAgent(),
		})

		response := map[string]interface{}{
			"system":     rt.Metrics.ToSystemMetricsResponse(),
			"backends":   rt.Metrics.ToAllBackendsResponse(),
			"rate_limit": rt.Metrics.ToSecurityMetricsResponse(),
			"http":       rt.Metrics.ToHTTPMetricsResponse(),
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(response); err != nil {
			logger.Warn("Failed to encode metrics response", map[string]interface{}{
				"error":       err.Error(),
				"remote_addr": r.RemoteAddr,
			})
		}
	}
}
