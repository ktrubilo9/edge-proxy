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
	HealthManager *health.HealthManager
	ConfigPath    string
	srv           *http.Server
	opsSrv        *http.Server

	clientIPs           *middleware.ClientIPResolver
	ratelimitMiddleware *middleware.RateLimitMiddleware
	proxyCollector      *metrics.ProxyResourceCollector
	backendCollector    *metrics.BackendResourceCollector
	adminGRPCServer     *grpcpkg.Server
	adminListener       net.Listener
	opsListener         net.Listener
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

	proxy.HealthManager = health.NewHealthManager(proxy.Runtime, proxy.Runtime.Metrics)

	clientIPs, err := middleware.NewClientIPResolver(os.Getenv("TRUSTED_PROXY_CIDRS"))
	if err != nil {
		log.Fatal("Failed to configure trusted proxies: ", err)
	}
	proxy.clientIPs = clientIPs
	proxy.ratelimitMiddleware = middleware.NewRateLimitMiddleware(proxy.Runtime, clientIPs)
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
	proxy.Runtime.SetOnBackendHealthCheckRequired(func(backend config.BackendConfig) {
		go proxy.HealthManager.CheckBackend(backend.Id)
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

	opsAddr := os.Getenv("OPS_ADDR")
	if opsAddr == "" {
		opsAddr = "127.0.0.1:9091"
	}
	opsListener, err := net.Listen("tcp", opsAddr)
	if err != nil {
		adminListener.Close()
		adminGRPCServer.Stop()
		return err
	}
	p.opsListener = opsListener

	if err := p.HealthManager.Start(); err != nil {
		return err
	}
	p.proxyCollector.Start()
	p.backendCollector.Start()

	mainHandler := http.HandlerFunc(handler.ProxyHandler(p.Runtime))
	mw := &middleware.MiddlewareChain{}

	mw.Use(p.clientIPs.Middleware)
	mw.Use(p.ratelimitMiddleware.Middleware)

	finalHandler := mw.Apply(mainHandler)
	mux := http.NewServeMux()
	mux.Handle("/", finalHandler)
	mux.HandleFunc("/health", handler.PublicHealthHandler(p.Runtime))
	mux.HandleFunc("/metrics", http.NotFound)
	mux.HandleFunc("/metrics/prometheus", http.NotFound)

	port := strconv.Itoa(p.Runtime.State().Snapshot.Raw.Server.ProxyPort)
	p.srv = &http.Server{
		Addr:              ":" + port,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    1 << 20,
	}

	opsMux := http.NewServeMux()
	opsMux.HandleFunc("/health", handler.HealthHandler(p.Runtime))
	opsMux.HandleFunc("/metrics", handler.MetricsHandler(p.Runtime))
	opsMux.Handle("/metrics/prometheus", handler.MetricsPrometheusHandler(p.Runtime))
	p.opsSrv = &http.Server{
		Addr:              opsAddr,
		Handler:           opsMux,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    64 << 10,
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
	go func() {
		logger.Info("Operational server started", map[string]interface{}{
			"address": p.opsListener.Addr().String(),
		})
		if serveErr := p.opsSrv.Serve(p.opsListener); serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
			logger.Error("Failed to serve operational endpoints", map[string]interface{}{
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
		if p.HealthManager != nil {
			p.HealthManager.Stop()
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
		if p.opsSrv != nil {
			if err := p.opsSrv.Shutdown(ctx); shutdownErr == nil {
				shutdownErr = err
			}
		}
		logger.Stop()
	})

	return shutdownErr
}
