package handler

import (
	"edge-proxy/internal/config"
	"edge-proxy/internal/testutil"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestProxyHandlerServesTrafficDuringRuntimeUpdates(t *testing.T) {
	backendA := newRuntimeUpdateBackend(t, "A")
	backendB := newRuntimeUpdateBackend(t, "B")

	healthConfig := testutil.DefaultHealthCheckConfig()
	cfg := &config.FullConfig{
		Server: config.ServerConfig{
			ProxyPort:     8080,
			AdminGrpcPort: 50051,
		},
		LoadBalancer: config.LoadBalancingConfig{
			Strategy: "least-connections",
		},
		Backends: []*config.BackendConfig{
			{Id: "backend-a", URL: backendA.URL, Weight: 1, Enabled: true},
			{Id: "backend-b", URL: backendB.URL, Weight: 1, Enabled: true},
		},
		HealthCheck: healthConfig,
		Timeouts: config.TimeoutsConfig{
			ConnectTimeoutMs:   500,
			ResponseTimeoutMs:  2000,
			KeepAliveTimeoutMs: 1000,
			IdleConnTimeoutMs:  1000,
		},
		VirtualHosts: []config.VirtualHost{
			{
				Domain:           "app.local",
				BackendIDs:       []string{"backend-a"},
				SecurityPolicyID: "default",
			},
		},
	}

	rt := newTestRuntime(t, cfg)
	proxyHandler := ProxyHandler(rt)
	initialClient := rt.State().HTTPClient

	const (
		workers           = 12
		requestsPerWorker = 50
		updates           = 80
	)

	start := make(chan struct{})
	errs := make(chan error, workers+1)
	var wg sync.WaitGroup

	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start

			for requestIndex := 0; requestIndex < requestsPerWorker; requestIndex++ {
				req := httptest.NewRequest(http.MethodGet, "http://app.local/", nil)
				req.Host = "app.local"
				rec := httptest.NewRecorder()

				proxyHandler.ServeHTTP(rec, req)

				body := strings.TrimSpace(rec.Body.String())
				if rec.Code != http.StatusOK {
					errs <- fmt.Errorf("proxy status %d: %s", rec.Code, body)
					return
				}
				if body != "A" && body != "B" {
					errs <- fmt.Errorf("unexpected backend response: %q", body)
					return
				}
			}
		}()
	}

	wg.Add(1)
	go func() {
		defer wg.Done()
		<-start

		for updateIndex := 0; updateIndex < updates; updateIndex++ {
			backendID := "backend-a"
			if updateIndex%2 == 1 {
				backendID = "backend-b"
			}

			if err := rt.UpdateVirtualHost("app.local", config.VirtualHost{
				BackendIDs:       []string{backendID},
				SecurityPolicyID: "default",
			}); err != nil {
				errs <- fmt.Errorf("update virtual host: %w", err)
				return
			}
		}
	}()

	close(start)
	wg.Wait()
	close(errs)

	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}

	current := rt.State()
	vhost := current.Snapshot.VHostsByDomain["app.local"]
	if vhost == nil {
		t.Fatal("virtual host disappeared after concurrent updates")
	}
	if len(vhost.BackendIDs) != 1 {
		t.Fatalf("virtual host backend count = %d, want 1", len(vhost.BackendIDs))
	}
	if current.HTTPClient != initialClient {
		t.Fatal("routing updates replaced the HTTP client")
	}
}

func newRuntimeUpdateBackend(t *testing.T, response string) *httptest.Server {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(time.Millisecond)
		_, _ = w.Write([]byte(response))
	}))
	t.Cleanup(server.Close)
	return server
}
