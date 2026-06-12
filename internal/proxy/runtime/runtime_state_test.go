package runtime

import (
	"edge-proxy/internal/config"
	"edge-proxy/internal/metrics"
	"net/http"
	"testing"
)

type closeTrackingTransport struct {
	closed bool
}

func (t *closeTrackingTransport) RoundTrip(*http.Request) (*http.Response, error) {
	panic("RoundTrip should not be called")
}

func (t *closeTrackingTransport) CloseIdleConnections() {
	t.closed = true
}

func TestBuildRuntimeStateInitializesComponents(t *testing.T) {
	rt := &Runtime{Metrics: metrics.NewMetrics()}
	snapshot := runtimeTestSnapshot(defaultRuntimeTestTimeouts(), "least-connections", "http://backend-1")

	state := rt.buildRuntimeState(nil, snapshot)

	if state.Snapshot != snapshot {
		t.Fatal("runtime state does not contain the supplied snapshot")
	}
	if state.HTTPClient == nil {
		t.Fatal("runtime state did not initialize the HTTP client")
	}
	if state.LoadBalancer == nil {
		t.Fatal("runtime state did not initialize the load balancer")
	}
}

func TestBuildRuntimeStateReusesComponentsForRoutingChange(t *testing.T) {
	rt := &Runtime{Metrics: metrics.NewMetrics()}
	initialSnapshot := runtimeTestSnapshot(defaultRuntimeTestTimeouts(), "least-connections", "http://backend-1")
	initial := rt.buildRuntimeState(nil, initialSnapshot)

	nextSnapshot := runtimeTestSnapshot(defaultRuntimeTestTimeouts(), "least-connections", "http://backend-2")
	next := rt.buildRuntimeState(initial, nextSnapshot)

	if next == initial {
		t.Fatal("runtime state was not replaced")
	}
	if next.HTTPClient != initial.HTTPClient {
		t.Fatal("HTTP client was replaced even though transport configuration did not change")
	}
	if next.LoadBalancer != initial.LoadBalancer {
		t.Fatal("load balancer was replaced even though its strategy did not change")
	}
}

func TestBuildRuntimeStateReplacesHTTPClientWhenTimeoutsChange(t *testing.T) {
	rt := &Runtime{Metrics: metrics.NewMetrics()}
	initialSnapshot := runtimeTestSnapshot(defaultRuntimeTestTimeouts(), "least-connections", "http://backend-1")
	initial := rt.buildRuntimeState(nil, initialSnapshot)

	updatedTimeouts := defaultRuntimeTestTimeouts()
	updatedTimeouts.ResponseTimeoutMs++
	nextSnapshot := runtimeTestSnapshot(updatedTimeouts, "least-connections", "http://backend-1")
	next := rt.buildRuntimeState(initial, nextSnapshot)

	if next.HTTPClient == initial.HTTPClient {
		t.Fatal("HTTP client was reused after timeout configuration changed")
	}
	if next.LoadBalancer != initial.LoadBalancer {
		t.Fatal("load balancer was replaced by an unrelated timeout change")
	}
}

func TestCloseReplacedHTTPClientOnlyClosesReplacedClient(t *testing.T) {
	transport := &closeTrackingTransport{}
	client := &http.Client{Transport: transport}
	previous := &RuntimeState{HTTPClient: client}

	closeReplacedHTTPClient(previous, &RuntimeState{HTTPClient: client})
	if transport.closed {
		t.Fatal("reused HTTP client had its idle connections closed")
	}

	closeReplacedHTTPClient(previous, &RuntimeState{HTTPClient: &http.Client{}})
	if !transport.closed {
		t.Fatal("replaced HTTP client did not close its idle connections")
	}
}

func runtimeTestSnapshot(
	timeouts config.TimeoutsConfig,
	strategy string,
	backendURL string,
) *config.Snapshot {
	return config.BuildSnapshot(&config.FullConfig{
		LBStrategy: strategy,
		Backends: []*config.BackendConfig{
			{URL: backendURL, Weight: 1, Enabled: true},
		},
		Timeouts: timeouts,
	})
}

func defaultRuntimeTestTimeouts() config.TimeoutsConfig {
	return config.TimeoutsConfig{
		ConnectTimeoutMs:   1000,
		ResponseTimeoutMs:  2000,
		KeepAliveTimeoutMs: 3000,
		IdleConnTimeoutMs:  4000,
	}
}
