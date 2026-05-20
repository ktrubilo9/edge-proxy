package handler

import (
	"edge-proxy/internal/proxy/runtime"
	"strconv"

	"github.com/prometheus/client_golang/prometheus"
)

type proxyCollector struct {
	rt *runtime.Runtime

	goroutinesDesc            *prometheus.Desc
	memoryPercentDesc         *prometheus.Desc
	cpuPercentDesc            *prometheus.Desc
	uptimeSecondsDesc         *prometheus.Desc
	requestsTotalDesc         *prometheus.Desc
	failedRequestsTotalDesc   *prometheus.Desc
	rateLimitAllowedTotalDesc *prometheus.Desc
	rateLimitBlockedTotalDesc *prometheus.Desc
	requestSizeBytesDesc      *prometheus.Desc
	responseSizeBytesDesc     *prometheus.Desc
	httpStatusTotalDesc       *prometheus.Desc
	httpMethodTotalDesc       *prometheus.Desc
	backendRequestsTotalDesc  *prometheus.Desc
	backendFailuresTotalDesc  *prometheus.Desc
	backendTimeoutsTotalDesc  *prometheus.Desc
	backendActiveConnsDesc    *prometheus.Desc
	backendLatencyEWMADesc    *prometheus.Desc
	backendErrorRateEWMADesc  *prometheus.Desc
}

func newProxyCollector(rt *runtime.Runtime) *proxyCollector {
	return &proxyCollector{
		rt:                      rt,
		goroutinesDesc:          prometheus.NewDesc("reverse_proxy_goroutines", "Number of goroutines", nil, nil),
		memoryPercentDesc:       prometheus.NewDesc("reverse_proxy_memory_percent", "Memory usage percent", nil, nil),
		cpuPercentDesc:          prometheus.NewDesc("reverse_proxy_cpu_percent", "CPU usage percent", nil, nil),
		uptimeSecondsDesc:       prometheus.NewDesc("reverse_proxy_uptime_seconds", "Proxy uptime", nil, nil),
		requestsTotalDesc:       prometheus.NewDesc("reverse_proxy_requests_total", "Total requests", nil, nil),
		failedRequestsTotalDesc: prometheus.NewDesc("reverse_proxy_failed_requests_total", "Total failed requests", nil, nil),
		rateLimitAllowedTotalDesc: prometheus.NewDesc(
			"reverse_proxy_rate_limit_allowed_total",
			"Requests allowed by rate limiter",
			nil,
			nil,
		),
		rateLimitBlockedTotalDesc: prometheus.NewDesc(
			"reverse_proxy_rate_limit_blocked_total",
			"Requests blocked by rate limiter",
			nil,
			nil,
		),
		requestSizeBytesDesc:     prometheus.NewDesc("reverse_proxy_request_size_bytes", "Total request size", nil, nil),
		responseSizeBytesDesc:    prometheus.NewDesc("reverse_proxy_response_size_bytes", "Total response size", nil, nil),
		httpStatusTotalDesc:      prometheus.NewDesc("reverse_proxy_http_status_total", "HTTP status codes", []string{"code"}, nil),
		httpMethodTotalDesc:      prometheus.NewDesc("reverse_proxy_http_method_total", "HTTP methods", []string{"method"}, nil),
		backendRequestsTotalDesc: prometheus.NewDesc("reverse_proxy_backend_requests_total", "Backend requests", []string{"backend"}, nil),
		backendFailuresTotalDesc: prometheus.NewDesc("reverse_proxy_backend_failures_total", "Backend failures", []string{"backend"}, nil),
		backendTimeoutsTotalDesc: prometheus.NewDesc("reverse_proxy_backend_timeouts_total", "Backend timeouts", []string{"backend"}, nil),
		backendActiveConnsDesc:   prometheus.NewDesc("reverse_proxy_backend_active_connections", "Backend active connections", []string{"backend"}, nil),
		backendLatencyEWMADesc:   prometheus.NewDesc("reverse_proxy_backend_latency_ewma", "Backend latency EWMA", []string{"backend"}, nil),
		backendErrorRateEWMADesc: prometheus.NewDesc("reverse_proxy_backend_error_rate_ewma", "Backend error rate EWMA", []string{"backend"}, nil),
	}
}

func (c *proxyCollector) Describe(ch chan<- *prometheus.Desc) {
	descs := []*prometheus.Desc{
		c.goroutinesDesc,
		c.memoryPercentDesc,
		c.cpuPercentDesc,
		c.uptimeSecondsDesc,
		c.requestsTotalDesc,
		c.failedRequestsTotalDesc,
		c.rateLimitAllowedTotalDesc,
		c.rateLimitBlockedTotalDesc,
		c.requestSizeBytesDesc,
		c.responseSizeBytesDesc,
		c.httpStatusTotalDesc,
		c.httpMethodTotalDesc,
		c.backendRequestsTotalDesc,
		c.backendFailuresTotalDesc,
		c.backendTimeoutsTotalDesc,
		c.backendActiveConnsDesc,
		c.backendLatencyEWMADesc,
		c.backendErrorRateEWMADesc,
	}
	for _, desc := range descs {
		ch <- desc
	}
}

func (c *proxyCollector) Collect(ch chan<- prometheus.Metric) {
	sys := c.rt.Metrics.ToSystemMetricsResponse()
	backends := c.rt.Metrics.ToAllBackendsResponse()
	rateLimit := c.rt.Metrics.ToSecurityMetricsResponse()
	httpMetrics := c.rt.Metrics.ToHTTPMetricsResponse()

	ch <- prometheus.MustNewConstMetric(c.goroutinesDesc, prometheus.GaugeValue, float64(sys.Goroutines))
	ch <- prometheus.MustNewConstMetric(c.memoryPercentDesc, prometheus.GaugeValue, float64(sys.MemoryPercent))
	ch <- prometheus.MustNewConstMetric(c.cpuPercentDesc, prometheus.GaugeValue, float64(sys.CpuPercent))
	ch <- prometheus.MustNewConstMetric(c.uptimeSecondsDesc, prometheus.CounterValue, float64(c.rt.Metrics.GetUptime()))
	ch <- prometheus.MustNewConstMetric(c.requestsTotalDesc, prometheus.CounterValue, float64(c.rt.Metrics.GetTotalRequests()))
	ch <- prometheus.MustNewConstMetric(c.failedRequestsTotalDesc, prometheus.CounterValue, float64(c.rt.Metrics.GetTotalFailedRequests()))
	ch <- prometheus.MustNewConstMetric(c.rateLimitAllowedTotalDesc, prometheus.CounterValue, float64(rateLimit.AllowedRequests))
	ch <- prometheus.MustNewConstMetric(c.rateLimitBlockedTotalDesc, prometheus.CounterValue, float64(rateLimit.BlockedRequests))
	ch <- prometheus.MustNewConstMetric(c.requestSizeBytesDesc, prometheus.CounterValue, float64(httpMetrics.TotalRequestSize))
	ch <- prometheus.MustNewConstMetric(c.responseSizeBytesDesc, prometheus.CounterValue, float64(httpMetrics.TotalResponseSize))

	for code, count := range httpMetrics.StatusCodes {
		ch <- prometheus.MustNewConstMetric(c.httpStatusTotalDesc, prometheus.CounterValue, float64(count), strconv.Itoa(int(code)))
	}
	for method, count := range httpMetrics.MethodsCount {
		ch <- prometheus.MustNewConstMetric(c.httpMethodTotalDesc, prometheus.CounterValue, float64(count), method)
	}
	for _, backend := range backends.Backends {
		ch <- prometheus.MustNewConstMetric(c.backendRequestsTotalDesc, prometheus.CounterValue, float64(backend.Requests), backend.URL)
		ch <- prometheus.MustNewConstMetric(c.backendFailuresTotalDesc, prometheus.CounterValue, float64(backend.Failures), backend.URL)
		ch <- prometheus.MustNewConstMetric(c.backendTimeoutsTotalDesc, prometheus.CounterValue, float64(backend.Timeouts), backend.URL)
		ch <- prometheus.MustNewConstMetric(c.backendActiveConnsDesc, prometheus.GaugeValue, float64(backend.ActiveConnections), backend.URL)
		ch <- prometheus.MustNewConstMetric(c.backendLatencyEWMADesc, prometheus.GaugeValue, backend.LatencyEWMA, backend.URL)
		ch <- prometheus.MustNewConstMetric(c.backendErrorRateEWMADesc, prometheus.GaugeValue, backend.ErrorRateEWMA, backend.URL)
	}
}
