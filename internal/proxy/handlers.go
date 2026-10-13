package proxy

import (
	"net/http"

	"github.com/ktrubilo9/edge-proxy/internal/middleware"
	"github.com/ktrubilo9/edge-proxy/internal/proxy/handler"
)

// publicHandler builds the HTTP handler served on the public proxy port.
//
// Only the routes listed here are reachable from the public port. Operational
// routes such as /metrics and /metrics/prometheus are deliberately absent, so
// detailed health and metrics data cannot leak to untrusted clients.
//
// The /health route is reserved for every HTTP method. Read-only endpoints are
// wrapped in readOnlyHandler so that unsupported methods get 405 instead of
// falling through to the catch-all "/" proxy route, which would forward the
// request to a backend.
func (p *Proxy) publicHandler() http.Handler {
	mainHandler := http.HandlerFunc(handler.ProxyHandler(p.Runtime))

	chain := &middleware.MiddlewareChain{}
	chain.Use(p.clientIPs.Middleware)
	chain.Use(p.ratelimitMiddleware.Middleware)
	finalHandler := chain.Apply(mainHandler)

	mux := http.NewServeMux()
	mux.Handle("/", finalHandler)
	mux.Handle("/health", readOnlyHandler(handler.PublicHealthHandler(p.Runtime), http.MethodGet, http.MethodHead))
	mux.HandleFunc("/metrics", http.NotFound)
	mux.HandleFunc("/metrics/prometheus", http.NotFound)

	return mux
}

// operationalHandler builds the HTTP handler served on the operational port.
//
// This port is not exposed publicly and is the only place where detailed health
// information and metrics are available. Every route is read-only.
func (p *Proxy) operationalHandler() http.Handler {
	mux := http.NewServeMux()
	mux.Handle("/health", readOnlyHandler(handler.HealthHandler(p.Runtime), http.MethodGet, http.MethodHead))
	mux.Handle("/metrics", readOnlyHandler(handler.MetricsHandler(p.Runtime), http.MethodGet, http.MethodHead))
	mux.Handle("/metrics/prometheus", readOnlyHandler(handler.MetricsPrometheusHandler(p.Runtime), http.MethodGet, http.MethodHead))

	return mux
}

// readOnlyHandler restricts next to the given HTTP methods.
//
// Methods outside the allowed set receive 405 Method Not Allowed with an Allow
// header listing the supported methods. The route stays registered for all
// methods so that the mux keeps reserving the path: registering only
// "GET /health" would let "POST /health" fall through to a broader pattern and
// be proxied to a backend.
func readOnlyHandler(next http.Handler, methods ...string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		for _, method := range methods {
			if r.Method == method {
				next.ServeHTTP(w, r)
				return
			}
		}

		w.Header().Set("Allow", joinMethods(methods))
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
	})
}

func joinMethods(methods []string) string {
	if len(methods) == 0 {
		return ""
	}

	joined := methods[0]
	for _, method := range methods[1:] {
		joined += ", " + method
	}
	return joined
}
