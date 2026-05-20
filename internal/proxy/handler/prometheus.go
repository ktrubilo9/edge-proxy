package handler

import (
	"edge-proxy/internal/proxy/runtime"
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

func MetricsPrometheusHandler(rt *runtime.Runtime) http.Handler {
	registry := prometheus.NewRegistry()
	registry.MustRegister(newProxyCollector(rt))
	return promhttp.HandlerFor(registry, promhttp.HandlerOpts{})
}
