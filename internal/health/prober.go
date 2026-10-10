package health

import (
	"context"
	"errors"
	"fmt"
	"github.com/ktrubilo9/edge-proxy/internal/config"
	"io"
	"net"
	"net/http"
	"strings"
	"time"
)

type Prober interface {
	Probe(
		ctx context.Context,
		backend *config.BackendConfig,
		cfg config.HealthProbeConfig,
	) ProbeResult
}

type HTTPProber struct {
	client *http.Client
}

func NewHTTPProber(client *http.Client) *HTTPProber {
	if client == nil {
		client = &http.Client{}
	}
	return &HTTPProber{client: client}
}

func NewConfiguredHTTPProber(cfg config.HealthTransportConfig) *HTTPProber {
	keepAlive := time.Duration(cfg.KeepAliveMs) * time.Millisecond
	transport := &http.Transport{
		MaxIdleConns:        cfg.MaxIdleConns,
		MaxIdleConnsPerHost: cfg.MaxIdleConnsPerHost,
		MaxConnsPerHost:     cfg.MaxConnsPerHost,
		IdleConnTimeout:     keepAlive,
		ForceAttemptHTTP2:   true,
		DialContext: (&net.Dialer{
			KeepAlive: keepAlive,
		}).DialContext,
	}
	return NewHTTPProber(&http.Client{Transport: transport})
}

func (pr *HTTPProber) CloseIdleConnections() {
	pr.client.CloseIdleConnections()
}

type ProbeResult struct {
	Healthy    bool
	StatusCode int
	Duration   time.Duration
	Err        error
}

func (pr *HTTPProber) Probe(
	ctx context.Context,
	backend *config.BackendConfig,
	cfg config.HealthProbeConfig,
) ProbeResult {
	if backend == nil {
		return ProbeResult{Err: errors.New("health check backend is nil")}
	}

	probeCtx, cancel := context.WithTimeout(
		ctx,
		time.Duration(cfg.TimeoutMs)*time.Millisecond,
	)
	defer cancel()

	req, err := http.NewRequestWithContext(
		probeCtx,
		strings.ToUpper(cfg.Method),
		healthURL(backend.URL, cfg.Path),
		nil,
	)
	if err != nil {
		return ProbeResult{Err: err}
	}

	start := time.Now()

	resp, err := pr.client.Do(req)
	duration := time.Since(start)

	if err != nil {
		return ProbeResult{
			Healthy:  false,
			Duration: duration,
			Err:      err,
		}
	}

	defer resp.Body.Close()
	// Draining a small health response lets the transport reuse its connection.
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4<<10))

	if !isSuccessCode(cfg.SuccessCodes, int32(resp.StatusCode)) {
		return ProbeResult{
			Healthy:    false,
			StatusCode: resp.StatusCode,
			Duration:   duration,
			Err: fmt.Errorf(
				"health check returned non-success status: %d",
				resp.StatusCode,
			),
		}
	}

	return ProbeResult{
		Healthy:    true,
		StatusCode: resp.StatusCode,
		Duration:   duration,
	}
}

func healthURL(backendURL, path string) string {
	return strings.TrimRight(backendURL, "/") + "/" + strings.TrimLeft(path, "/")
}

func isSuccessCode(successCodes []int32, statusCode int32) bool {
	for _, code := range successCodes {
		if statusCode == code {
			return true
		}
	}

	return false
}
