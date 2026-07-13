package config

import (
	"fmt"
	"net/url"
	"strings"
)

type Validator interface {
	Validate(cfg *FullConfig) error
}

type DefaultValidator struct{}

func ValidateConfig(cfg *FullConfig) error {
	return DefaultValidator{}.Validate(cfg)
}

func (v DefaultValidator) Validate(cfg *FullConfig) error {
	if cfg == nil {
		return fmt.Errorf("config cannot be nil")
	}

	if cfg.Server.ProxyPort < 1 || cfg.Server.ProxyPort > 65535 {
		return fmt.Errorf("invalid proxy port: %d", cfg.Server.ProxyPort)
	}
	if cfg.Server.AdminGrpcPort < 1 || cfg.Server.AdminGrpcPort > 65535 {
		return fmt.Errorf("invalid admin grpc port: %d", cfg.Server.AdminGrpcPort)
	}

	if len(cfg.Backends) == 0 {
		return fmt.Errorf("no backends configured")
	}

	backendIDs := make(map[string]struct{}, len(cfg.Backends))
	backendURLs := make(map[string]struct{}, len(cfg.Backends))
	enabledBackends := 0

	for _, backend := range cfg.Backends {
		if backend == nil {
			return fmt.Errorf("backend config cannot be nil")
		}
		if backend.Id == "" {
			return fmt.Errorf("backend id cannot be empty")
		}
		if backend.URL == "" {
			return fmt.Errorf("backend URL cannot be empty")
		}

		parsed, err := url.Parse(backend.URL)
		if err != nil || parsed.Scheme == "" || parsed.Host == "" {
			return fmt.Errorf("invalid backend URL: %s", backend.URL)
		}
		if parsed.Scheme != "http" && parsed.Scheme != "https" {
			return fmt.Errorf("unsupported backend URL scheme: %s", parsed.Scheme)
		}

		if _, exists := backendIDs[backend.Id]; exists {
			return fmt.Errorf("duplicate backend id: %s", backend.Id)
		}
		backendIDs[backend.Id] = struct{}{}

		if _, exists := backendURLs[backend.URL]; exists {
			return fmt.Errorf("duplicate backend URL: %s", backend.URL)
		}
		backendURLs[backend.URL] = struct{}{}

		if backend.Weight <= 0 {
			return fmt.Errorf("backend %s weight must be positive", backend.URL)
		}

		if backend.Enabled {
			enabledBackends++
		}
	}

	if enabledBackends == 0 {
		return fmt.Errorf("at least one backend must be enabled")
	}

	if cfg.Logging.BufferSize < 0 {
		return fmt.Errorf("logging buffer size cannot be negative")
	}

	if cfg.Logging.Level != "" {
		switch strings.ToLower(cfg.Logging.Level) {
		case "debug", "info", "warn", "error":
		default:
			return fmt.Errorf("invalid logging level: %s", cfg.Logging.Level)
		}
	}

	allowedStrategies := map[string]struct{}{
		"least-connections": {},
		"adaptive":          {},
	}

	if _, ok := allowedStrategies[cfg.LoadBalancer.Strategy]; !ok {
		return fmt.Errorf("invalid load balancer strategy: %s", cfg.LoadBalancer.Strategy)
	}

	if cfg.HealthCheck.Path == "" {
		return fmt.Errorf("health check path cannot be empty")
	}
	if cfg.HealthCheck.IntervalSeconds <= 0 {
		return fmt.Errorf("health check interval must be positive")
	}
	if cfg.HealthCheck.TimeoutSeconds <= 0 {
		return fmt.Errorf("health check timeout must be positive")
	}
	if cfg.HealthCheck.HealthyThreshold <= 0 {
		return fmt.Errorf("healthy threshold must be positive")
	}
	if len(cfg.HealthCheck.SuccessCodes) == 0 {
		return fmt.Errorf("health check success codes cannot be empty")
	}
	for _, code := range cfg.HealthCheck.SuccessCodes {
		if code < 100 || code > 599 {
			return fmt.Errorf("invalid health check success code: %d", code)
		}
	}

	if cfg.Timeouts.ConnectTimeoutMs <= 0 ||
		cfg.Timeouts.ResponseTimeoutMs <= 0 ||
		cfg.Timeouts.KeepAliveTimeoutMs <= 0 ||
		cfg.Timeouts.IdleConnTimeoutMs <= 0 {
		return fmt.Errorf("timeouts must be positive")
	}

	domains := make(map[string]struct{}, len(cfg.VirtualHosts))
	policyIDs := make(map[string]struct{}, len(cfg.Security.Policies))

	for _, policy := range cfg.Security.Policies {
		if policy.Id == "" {
			return fmt.Errorf("security policy id cannot be empty")
		}
		if _, exists := policyIDs[policy.Id]; exists {
			return fmt.Errorf("duplicate security policy id: %s", policy.Id)
		}
		policyIDs[policy.Id] = struct{}{}

		if policy.RateLimiting.Enabled {
			if policy.RateLimiting.RatePerIP <= 0 {
				return fmt.Errorf("rate_per_ip must be positive for security policy %s", policy.Id)
			}
			if policy.RateLimiting.Burst <= 0 {
				return fmt.Errorf("burst must be positive for security policy %s", policy.Id)
			}
			if policy.RateLimiting.WindowSec <= 0 {
				return fmt.Errorf("window_sec must be positive for security policy %s", policy.Id)
			}
		}
	}

	for _, vhost := range cfg.VirtualHosts {
		if vhost.Domain == "" {
			return fmt.Errorf("virtual host domain cannot be empty")
		}
		if _, exists := domains[vhost.Domain]; exists {
			return fmt.Errorf("duplicate virtual host domain: %s", vhost.Domain)
		}
		domains[vhost.Domain] = struct{}{}

		if len(vhost.BackendIDs) == 0 && len(vhost.PathRoutes) == 0 {
			return fmt.Errorf("virtual host %s must define backends or path routes", vhost.Domain)
		}

		if _, ok := policyIDs[vhost.SecurityPolicyID]; !ok {
			return fmt.Errorf("virtual host %s references unknown security policy: %s", vhost.Domain, vhost.SecurityPolicyID)
		}

		for _, backendID := range vhost.BackendIDs {
			if _, ok := backendIDs[backendID]; !ok {
				return fmt.Errorf("virtual host %s references unknown backend: %s", vhost.Domain, backendID)
			}
		}

		for _, route := range vhost.PathRoutes {
			if route.Path == "" {
				return fmt.Errorf("path route path cannot be empty for virtual host %s", vhost.Domain)
			}
			if !strings.HasPrefix(route.Path, "/") {
				return fmt.Errorf("path route %s on virtual host %s must start with /", route.Path, vhost.Domain)
			}
			if len(route.BackendIDs) == 0 && len(vhost.BackendIDs) == 0 {
				return fmt.Errorf("path route %s on virtual host %s must define backend_ids when the virtual host has no default backends", route.Path, vhost.Domain)
			}

			for _, backendID := range route.BackendIDs {
				if _, ok := backendIDs[backendID]; !ok {
					return fmt.Errorf("path route %s on virtual host %s references unknown backend: %s", route.Path, vhost.Domain, backendID)
				}
			}
		}
	}

	return nil
}
