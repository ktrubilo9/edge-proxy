package proxy

import (
	"context"
	"edge-proxy/internal/admin/grpc"
	"edge-proxy/internal/config"
	"edge-proxy/internal/health"
	"edge-proxy/internal/logger"
	"edge-proxy/internal/metrics"
	"edge-proxy/internal/middleware"
	"edge-proxy/internal/proxy/handler"
	"edge-proxy/internal/proxy/runtime"
	"errors"
	"log"
	"net"
	"net/http"
	"os"
	"sync"
	"time"

	"strconv"

	grpcpkg "google.golang.org/grpc"
)

type Proxy struct {
	Runtime       *runtime.Runtime
	HealthChecker *health.HealthChecker
	ConfigPath    string
	srv           *http.Server

	ratelimitMiddleware *middleware.RateLimitMiddleware
	proxyCollector      *metrics.ProxyResourceCollector
	backendCollector    *metrics.BackendResourceCollector
	adminGRPCServer     *grpcpkg.Server
	adminListener       net.Listener
	stopOnce            sync.Once
}

func NewProxy(configPath string) *Proxy {
	if configPath == "" {
		configPath = os.Getenv("CONFIG_PATH")
	}
	if configPath == "" {
		if _, err := os.Stat("config.json"); err == nil {
			configPath = "config.json"
		} else {
			configPath = "configs/config.json"
		}
	}

	rt, err := runtime.NewRuntime(configPath)
	if err != nil {
		log.Fatal("Failed to load config: ", err)
	}

	if err := logger.Init(&rt.Config.Snapshot().Raw.Logging); err != nil {
		log.Printf("Failed to initialize logger: %v", err)
	}

	proxy := &Proxy{
		Runtime:    rt,
		ConfigPath: configPath,
	}

	hc := rt.Snapshot.Raw.HealthCheck
	proxy.HealthChecker = health.NewHealthChecker(proxy.Runtime, &hc)
	proxy.ratelimitMiddleware = middleware.NewRateLimitMiddleware(proxy.Runtime)
	proxy.proxyCollector = metrics.NewProxyResourceCollector(proxy.Runtime.Metrics, 5*time.Second)
	proxy.backendCollector = metrics.NewBackendResourceCollector(proxy.Runtime.Metrics, 5*time.Second, time.Second)

	proxy.Runtime.SetOnRateLimitUpdate(func(domain string, cfg config.RateLimitingConfig) {
		proxy.ratelimitMiddleware.UpdateRateLimiter(
			domain,
			int(cfg.RatePerIP),
			int(cfg.Burst),
			int(cfg.WindowSec),
		)
	})
	return proxy
}

func (p *Proxy) Start() error {
	adminGRPCServer, adminListener, err := grpc.NewAdminGRPCServer(p.Runtime)
	if err != nil {
		return err
	}
	p.adminGRPCServer = adminGRPCServer
	p.adminListener = adminListener

	p.HealthChecker.Start(p.Runtime.Metrics)
	p.proxyCollector.Start()
	p.backendCollector.Start()

	mainHandler := http.HandlerFunc(handler.ProxyHandler(p.Runtime))
	mw := &middleware.MiddlewareChain{}

	mw.Use(p.ratelimitMiddleware.Middleware)

	finalHandler := mw.Apply(mainHandler)
	mux := http.NewServeMux()
	mux.Handle("/", finalHandler)
	mux.HandleFunc("/metrics", handler.MetricsHandler(p.Runtime))
	mux.HandleFunc("/health", handler.HealthHandler(p.Runtime))

	mux.Handle("/metrics/prometheus", handler.MetricsPrometheusHandler(p.Runtime))

	port := strconv.Itoa(p.Runtime.Snapshot.Raw.ProxyPort)
	p.srv = &http.Server{
		Addr:    ":" + port,
		Handler: mux,
	}

	logger.Info("Proxy started", map[string]interface{}{
		"port":        port,
		"config_path": p.ConfigPath,
	})
	go func() {
		if serveErr := grpc.ServeAdminGRPC(p.adminGRPCServer, p.adminListener); serveErr != nil {
			logger.Error("Failed to serve admin gRPC server", map[string]interface{}{
				"error": serveErr.Error(),
			})
		}
	}()

	err = p.srv.ListenAndServe()
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

func (p *Proxy) Stop(ctx context.Context) error {
	var shutdownErr error

	p.stopOnce.Do(func() {
		if p.HealthChecker != nil {
			p.HealthChecker.Stop()
		}
		if p.proxyCollector != nil {
			p.proxyCollector.Stop()
		}
		if p.backendCollector != nil {
			p.backendCollector.Stop()
		}
		if p.adminGRPCServer != nil {
			done := make(chan struct{})
			go func() {
				p.adminGRPCServer.GracefulStop()
				close(done)
			}()
			select {
			case <-done:
			case <-ctx.Done():
				p.adminGRPCServer.Stop()
			}
		}
		if p.srv != nil {
			shutdownErr = p.srv.Shutdown(ctx)
		}
		logger.Stop()
	})

	return shutdownErr
}
