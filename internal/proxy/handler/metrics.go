package handler

import (
	"encoding/json"
	"github.com/ktrubilo9/edge-proxy/internal/logger"
	"github.com/ktrubilo9/edge-proxy/internal/proxy/runtime"
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
