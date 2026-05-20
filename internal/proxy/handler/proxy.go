package handler

import (
	"context"
	"crypto/rand"
	"edge-proxy/internal/config"
	"edge-proxy/internal/logger"
	"edge-proxy/internal/proxy/runtime"
	"encoding/hex"
	"io"
	"net"
	"net/http"
	"net/url"

	"strings"
	"sync/atomic"
	"time"
)

func ProxyHandler(state *runtime.Runtime) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		state.Metrics.HTTP.RecordMethod(r.Method)
		snapshot := state.SnapshotView()
		originalPath := r.URL.Path
		requestID := ensureRequestID(w, r)

		host := r.Host
		if strings.Contains(host, ":") {
			host = strings.Split(host, ":")[0]
		}

		var targetBackends []string
		var vhost *config.VirtualHost
		var pathRoute *config.PathRoute
		longestPathMatch := -1
		for i := range snapshot.Raw.VirtualHosts {
			vh := &snapshot.Raw.VirtualHosts[i]
			if vh.Domain == host {
				vhost = vh
				targetBackends = vh.Backends
				// Prefer the longest matching route so specific prefixes beat broader catch-all routes.
				for j := range vh.PathRoutes {
					pr := &vh.PathRoutes[j]
					if strings.HasPrefix(r.URL.Path, pr.Path) && len(pr.Path) > longestPathMatch {
						pathRoute = pr
						longestPathMatch = len(pr.Path)
						if len(pr.Backends) > 0 {
							targetBackends = pr.Backends
						}
					}
				}
				break
			}
		}

		logAndReturn := func(statusCode int, errMsg string, backendURL string, fields map[string]interface{}) {
			duration := time.Since(start)
			if fields == nil {
				fields = make(map[string]interface{})
			}
			fields["host"] = host
			fields["duration_ms"] = duration.Milliseconds()

			fields["status_code"] = statusCode
			fields["backend_url"] = backendURL
			fields["error_message"] = errMsg

			if statusCode >= 500 {
				logger.Error("Proxy request failed with 5xx", fields)
			} else if statusCode >= 400 {
				logger.Warn("Proxy request failed with 4xx", fields)
			} else {
				logger.Info("Proxy request succeeded", fields)
			}

			state.Metrics.HTTP.RecordStatusCode(statusCode)
			if statusCode >= 400 {
				atomic.AddUint64(&state.Metrics.FailedRequests, 1)
			}
			http.Error(w, errMsg, statusCode)
		}

		if vhost == nil || len(targetBackends) == 0 {
			logAndReturn(http.StatusForbidden, "Unknown host", "", nil)
			return
		}

		var backends []*config.BackendConfig
		for _, bURL := range targetBackends {
			b := snapshot.BackendsByURL[bURL]
			if b == nil || !b.Enabled {
				continue
			}
			if status := state.BackendStatus[b.URL]; status != nil && status.Active.Load() {
				backends = append(backends, b)
			}
		}

		if len(backends) == 0 {
			logAndReturn(http.StatusServiceUnavailable, "No active backends available", "", nil)
			return
		}

		backendCandidates := append([]*config.BackendConfig(nil), backends...)
		backend := state.LoadBalancer.Next(backendCandidates)
		if backend == nil {
			logAndReturn(http.StatusServiceUnavailable, "All servers are down", "", nil)
			return
		}

		var requestSize int64
		if r.ContentLength > 0 {
			requestSize = r.ContentLength
			state.Metrics.RecordRequestSize(requestSize)
		}

		fields := make(map[string]interface{})
		fields["host"] = host
		fields["request_id"] = requestID
		fields["request_size"] = requestSize
		if pathRoute != nil {
			fields["path_route"] = pathRoute.Path
			fields["strip_prefix"] = pathRoute.StripPrefix
		}

		ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
		defer cancel()

		resp, backend, err := executeProxyRequest(ctx, state, r, pathRoute, host, requestID, backendCandidates)
		duration := time.Since(start)
		fields["duration_ms"] = duration.Milliseconds()

		if err != nil {
			statusCode := http.StatusBadGateway
			fields["error_type"] = "http_client_do"
			fields["error_details"] = err.Error()
			if backend != nil {
				fields["attempted_backend"] = backend.URL
			}
			if netErr, ok := err.(net.Error); ok {
				fields["timeout"] = netErr.Timeout()
				fields["temporary"] = netErr.Temporary()
				if backend != nil && netErr.Timeout() {
					state.Metrics.RecordTimeout(backend.URL)
				}
			}
			if backend != nil {
				state.Metrics.RecordRequestEnd(backend.URL, float64(duration.Nanoseconds())/1e6, true, int32(statusCode))
			}
			logAndReturn(http.StatusBadGateway, "Backend error", "", fields)
			return
		}
		defer resp.Body.Close()
		state.Metrics.HTTP.RecordStatusCode(resp.StatusCode)

		for k, v := range resp.Header {
			for _, vv := range v {
				w.Header().Add(k, vv)
			}
		}
		w.WriteHeader(resp.StatusCode)

		responseSize, err := io.Copy(w, resp.Body)
		if err != nil {
			fields["error_type"] = "response_write"
			fields["error"] = err.Error()
			state.Metrics.RecordRequestEnd(backend.URL, float64(duration.Nanoseconds())/1e6, true, int32(resp.StatusCode))
			logger.Error("Proxy response write failed after headers were sent", fields)
			atomic.AddUint64(&state.Metrics.FailedRequests, 1)
			return
		}
		state.Metrics.RecordRequestEnd(backend.URL, float64(duration.Nanoseconds())/1e6, false, int32(resp.StatusCode))

		state.Metrics.RecordResponseSize(responseSize)

		fields["response_size"] = responseSize
		fields["status"] = resp.StatusCode
		fields["backend_url"] = backend.URL
		logger.LogRequest(start, "", r.Method, originalPath, resp.StatusCode, backend.URL, duration, "", fields)
	}
}

func stripRoutePrefix(requestPath string, routePrefix string) string {
	stripped := strings.TrimPrefix(requestPath, routePrefix)
	if stripped == "" {
		return "/"
	}
	if !strings.HasPrefix(stripped, "/") {
		return "/" + stripped
	}
	return stripped
}

func ensureRequestID(w http.ResponseWriter, r *http.Request) string {
	requestID := r.Header.Get("X-Request-ID")
	if requestID == "" {
		requestID = newRequestID()
		r.Header.Set("X-Request-ID", requestID)
	}
	w.Header().Set("X-Request-ID", requestID)
	return requestID
}

func newRequestID() string {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return hex.EncodeToString([]byte(time.Now().Format("20060102150405.000000000")))
	}
	return hex.EncodeToString(buf)
}

func executeProxyRequest(
	ctx context.Context,
	state *runtime.Runtime,
	original *http.Request,
	pathRoute *config.PathRoute,
	host string,
	requestID string,
	backends []*config.BackendConfig,
) (*http.Response, *config.BackendConfig, error) {
	retryable := isRetryableRequest(original)
	candidates := append([]*config.BackendConfig(nil), backends...)
	var lastErr error
	var lastBackend *config.BackendConfig

	for attempt := 0; len(candidates) > 0; attempt++ {
		backend := state.LoadBalancer.Next(candidates)
		if backend == nil {
			break
		}
		lastBackend = backend

		resp, err := doProxyRequest(ctx, state, original, pathRoute, host, requestID, backend)
		if err == nil {
			return resp, backend, nil
		}

		lastErr = err
		if !retryable || attempt > 0 {
			break
		}

		// Retry idempotent requests once on a different backend to avoid failing fast on a single dead peer.
		candidates = filterOutBackend(candidates, backend.URL)
		if len(candidates) == 0 {
			break
		}

		logger.Warn("Retrying request on alternate backend", map[string]interface{}{
			"request_id":     requestID,
			"failed_backend": backend.URL,
			"host":           host,
			"path":           original.URL.Path,
			"method":         original.Method,
			"error":          err.Error(),
		})
	}

	return nil, lastBackend, lastErr
}

func doProxyRequest(
	ctx context.Context,
	state *runtime.Runtime,
	original *http.Request,
	pathRoute *config.PathRoute,
	host string,
	requestID string,
	backend *config.BackendConfig,
) (*http.Response, error) {
	targetURL, err := url.Parse(backend.URL)
	if err != nil {
		return nil, err
	}

	state.Metrics.IncrementActiveConnections(backend.URL)
	defer state.Metrics.DecrementActiveConnections(backend.URL)

	req := original.Clone(ctx)
	if original.Body != nil && original.GetBody != nil {
		body, bodyErr := original.GetBody()
		if bodyErr != nil {
			return nil, bodyErr
		}
		req.Body = body
	}

	req.URL.Scheme = targetURL.Scheme
	req.URL.Host = targetURL.Host
	req.RequestURI = ""
	req.Host = targetURL.Host
	req.Header.Set("X-Request-ID", requestID)
	req.Header.Set("X-Forwarded-Host", host)
	req.Header.Set("X-Forwarded-Proto", forwardedProto(original))
	if req.Header.Get("X-Forwarded-For") == "" {
		ip, _, _ := net.SplitHostPort(original.RemoteAddr)
		req.Header.Set("X-Forwarded-For", ip)
	}
	if pathRoute != nil && pathRoute.StripPrefix {
		req.URL.Path = stripRoutePrefix(req.URL.Path, pathRoute.Path)
		req.URL.RawPath = req.URL.Path
	}

	return state.HTTPClient.Do(req)
}

func isRetryableRequest(r *http.Request) bool {
	switch r.Method {
	case http.MethodGet, http.MethodHead, http.MethodOptions, http.MethodTrace:
		return true
	default:
		return r.GetBody != nil
	}
}

func filterOutBackend(backends []*config.BackendConfig, targetURL string) []*config.BackendConfig {
	filtered := backends[:0]
	for _, backend := range backends {
		if backend.URL != targetURL {
			filtered = append(filtered, backend)
		}
	}
	return filtered
}

func forwardedProto(r *http.Request) string {
	if r.TLS != nil {
		return "https"
	}
	return "http"
}
