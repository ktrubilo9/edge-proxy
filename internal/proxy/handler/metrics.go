package handler

import (
	"edge-proxy/internal/logger"
	"edge-proxy/internal/proxy/runtime"
	"encoding/json"
	"net/http"
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
		json.NewEncoder(w).Encode(response)
	}
}
