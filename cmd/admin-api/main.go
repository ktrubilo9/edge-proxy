package main

import (
	"edge-proxy/internal/admin"
	"edge-proxy/internal/admin/handler"
	"edge-proxy/internal/api/adminpb"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func main() {
	proxyAddr := os.Getenv("PROXY_ADDR")
	if proxyAddr == "" {
		proxyAddr = "reverse-proxy:50051"
	}

	authEnabled := true
	if raw := os.Getenv("ADMIN_AUTH_ENABLED"); raw != "" {
		parsed, err := strconv.ParseBool(raw)
		if err != nil {
			log.Fatalf("Invalid ADMIN_AUTH_ENABLED value: %v", err)
		}
		authEnabled = parsed
	}

	adminToken := os.Getenv("ADMIN_API_TOKEN")
	if authEnabled && adminToken == "" {
		log.Fatal("ADMIN_API_TOKEN is required when admin authentication is enabled")
	}

	grpcToken := os.Getenv("ADMIN_GRPC_TOKEN")
	if grpcToken == "" {
		log.Fatal("ADMIN_GRPC_TOKEN is required")
	}

	conn, err := grpc.NewClient(
		proxyAddr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithUnaryInterceptor(admin.NewGRPCAuthClientInterceptor(grpcToken)),
	)
	if err != nil {
		log.Fatalf("Failed to connect to proxy admin server: %v", err)
	}
	defer conn.Close()

	proxyClient := adminpb.NewProxyAdminClient(conn)

	mux := http.NewServeMux()

	// Backend management
	mux.HandleFunc("GET /api/backend", handler.HandleBackendsList(proxyClient))
	mux.HandleFunc("POST /api/backend", handler.HandleBackendAdd(proxyClient))
	mux.HandleFunc("GET /api/backend/{url...}", handler.HandleBackendGet(proxyClient))
	mux.HandleFunc("PUT /api/backend/{url...}", handler.HandleBackendUpdate(proxyClient))
	mux.HandleFunc("DELETE /api/backend/{url...}", handler.HandleBackendDelete(proxyClient))

	// Virtual host management
	mux.HandleFunc("GET /api/vhost", handler.HandleVhostsList(proxyClient))
	mux.HandleFunc("POST /api/vhost", handler.HandleVhostAdd(proxyClient))
	mux.HandleFunc("GET /api/vhost/{domain}", handler.HandleVhostGet(proxyClient))
	mux.HandleFunc("PUT /api/vhost/{domain}", handler.HandleVhostUpdate(proxyClient))
	mux.HandleFunc("DELETE /api/vhost/{domain}", handler.HandleVhostDelete(proxyClient))
	mux.HandleFunc("GET /api/vhost/{domain}/security", handler.HandleSecurityConfigGet(proxyClient))
	mux.HandleFunc("PUT /api/vhost/{domain}/security", handler.HandleSecurityConfigUpdate(proxyClient))

	// Global proxy settings
	mux.HandleFunc("GET /api/config/lb", handler.HandleGlobalConfig(proxyClient))
	mux.HandleFunc("PUT /api/config/lb", handler.HandleGlobalConfig(proxyClient))

	handlerWithAuth := admin.NewAuthMiddleware(authEnabled, adminToken, mux)
	handlerWithCORS := corsMiddleware(handlerWithAuth)
	handlerWithBodyLimit := http.MaxBytesHandler(handlerWithCORS, 1<<20)

	srv := &http.Server{
		Addr:              ":8081",
		Handler:           handlerWithBodyLimit,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    64 << 10,
	}

	log.Println("Admin API listening on :8081")
	if err := srv.ListenAndServe(); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}

func corsMiddleware(next http.Handler) http.Handler {
	allowedOrigin := os.Getenv("ADMIN_ALLOWED_ORIGIN")
	if allowedOrigin == "" {
		allowedOrigin = "http://localhost:3000"
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Keep the default CORS target local for development, but allow overriding it without code changes.
		w.Header().Set("Access-Control-Allow-Origin", allowedOrigin)
		w.Header().Set("Access-Control-Allow-Credentials", "true")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Add("Vary", "Origin")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}
