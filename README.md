# Edge Proxy

Edge Proxy is a Go-based reverse proxy and small edge-gateway project built around the core concerns of routing, runtime configuration, health checking, rate limiting, and observability. It is organized as a single Go module with separate command entry points and internal packages for the proxy runtime, admin APIs, metrics, configuration, and middleware.

The project is intentionally scoped as an engineering/learning system, not as a replacement for mature production proxies such as NGINX, Traefik, Envoy, or HAProxy. Its value is in showing how the main building blocks of such a system fit together in code: request routing, backend state, configuration updates, admin control, and operational visibility.

## Features

- Host-based reverse proxy routing with virtual hosts.
- Path-based routing with optional prefix stripping.
- Least-connections load balancing.
- Backend health checks with active/inactive status tracking.
- Runtime configuration updates through an admin HTTP API backed by gRPC.
- Per-virtual-host rate limiting.
- Request IDs and forwarded headers for better traceability.
- Prometheus metrics endpoint and provisioned Grafana dashboard.
- Docker Compose setup with two mock backend services.

## Project Scope

This repository demonstrates a complete but deliberately compact proxy system:

- a reverse proxy process that serves traffic and exposes health/metrics endpoints;
- an admin HTTP API process that communicates with the proxy through gRPC;
- file-backed configuration with validation and runtime refresh;
- active backend health checks and in-memory backend state;
- Prometheus metrics and a provisioned Grafana dashboard;
- Docker Compose infrastructure for local development and demos.

It does not currently include production-grade concerns such as TLS termination, authentication and authorization for the admin API, distributed configuration storage, hot reload across multiple proxy instances, advanced load-balancing policies, WAF rules, or service discovery.

## Architecture

```text
cmd/
  admin-api/        HTTP API for managing proxy configuration
  mock-backend/     Demo backend used by docker-compose
  reverse-proxy/    Main reverse proxy process

internal/
  admin/            REST handlers and gRPC admin server/client
  api/              Protocol Buffers definition and generated Go code
  config/           Config loading, defaults, validation, snapshots, storage
  health/           Backend health checking
  lb/               Load balancing strategies
  logger/           Structured logging helpers
  metrics/          Runtime, HTTP, backend, and Prometheus metrics
  middleware/       HTTP middleware chain and rate limiting
  proxy/            Proxy orchestration, runtime state, and request handlers
  ratelimit/        Rate limiter implementation

configs/            Default and example proxy configurations
deploy/             Dockerfiles, Prometheus, and Grafana provisioning
```

## Requirements

- Go 1.25 or newer, matching `go.mod`.
- Docker and Docker Compose for the full local stack.

## Quick Start

Create a local environment file:

```bash
cp .env.example .env
```

Start the full stack:

```bash
docker compose up --build
```

The compose stack starts:

- reverse proxy on `http://localhost:8080`
- admin API on `http://localhost:8081`
- Prometheus on `http://localhost:9090`
- Grafana on `http://localhost:3000`
- two mock backend services inside the Docker network

The default config routes traffic by the `Host` header, so test the proxy with:

```bash
curl -H "Host: app.example.local" http://localhost:8080/
```

Backends are marked active by the health checker. Immediately after startup, the proxy may briefly return `503` until the first health check succeeds.

Health and metrics endpoints:

```bash
curl http://localhost:8080/health
curl http://localhost:8080/metrics
curl http://localhost:8080/metrics/prometheus
```

## Configuration

The default proxy config is stored in `configs/config.json`. Additional examples are available in `configs/examples/`.

Values prefixed with `env:` are resolved from environment variables when the proxy loads the config:

```json
{ "url": "env:BACKEND1_URL", "weight": 1, "enabled": true }
```

Runtime updates made through the admin API are saved back to the configured JSON file. In the default Docker image, that file is inside the container, so changes are useful for local demos but should not be treated as durable configuration unless the config file is mounted as a volume.

Important fields:

- `proxy_port` - HTTP port used by the reverse proxy.
- `lb_strategy` - load balancing strategy. Currently supported: `least-connections`.
- `backends` - upstream backend URLs. Values can reference environment variables with `env:VARIABLE_NAME`.
- `virtual_hosts` - host-based routing rules.
- `path_routes` - optional path-specific routing rules inside a virtual host.
- `health_check` - backend health check path, interval, timeout, and expected status codes.
- `timeouts` - outbound HTTP client timeout settings.
- `logging` - log level and async logging options.
- `security.rate_limiting` - per-host rate limiting configuration.

Example virtual host:

```json
{
  "domain": "app.example.local",
  "backends": ["env:BACKEND1_URL", "env:BACKEND2_URL"],
  "path_routes": [
    {
      "path": "/api",
      "backends": ["env:BACKEND2_URL"]
    }
  ],
  "security": {
    "rate_limiting": {
      "enabled": false,
      "rate_per_ip": 100,
      "burst": 50,
      "window_sec": 60
    }
  }
}
```

## Admin API

The admin API listens on `:8081` in Docker Compose and communicates with the proxy through the internal gRPC admin server.

Available HTTP routes:

```text
GET    /api/backend
POST   /api/backend
GET    /api/backend/{url...}
PUT    /api/backend/{url...}
DELETE /api/backend/{url...}

GET    /api/vhost
POST   /api/vhost
GET    /api/vhost/{domain}
PUT    /api/vhost/{domain}
DELETE /api/vhost/{domain}

GET    /api/vhost/{domain}/security
PUT    /api/vhost/{domain}/security

GET    /api/config/lb
PUT    /api/config/lb
```

Example:

```bash
curl http://localhost:8081/api/backend
```

Add a backend:

```bash
curl -X POST http://localhost:8081/api/backend \
  -H "Content-Type: application/json" \
  -d '{"url":"http://backend-c:3000","weight":1}'
```

Add a virtual host:

```bash
curl -X POST http://localhost:8081/api/vhost \
  -H "Content-Type: application/json" \
  -d '{
    "vhost": {
      "domain": "api.example.local",
      "backends": ["http://backend-c:3000"],
      "security_config": {
        "rate_limiting": {
          "enabled": true,
          "rate_per_ip": 60,
          "burst": 20,
          "window_sec": 60
        }
      }
    }
  }'
```

## Observability

Prometheus scrapes the proxy from:

```text
http://reverse-proxy:8080/metrics/prometheus
```

Grafana is provisioned with a datasource and dashboard from `deploy/grafana/`. The default local admin password is configured through `GF_PASSWORD` in `.env`.

The proxy exports its own HTTP, backend, rate-limit, and resource metrics. Backend CPU and memory panels are populated when upstream services expose the demo `/admin/metrics` endpoint; the included mock backends do this.

## Development

Run quality checks:

```bash
gofmt -l .
go vet ./...
go test ./...
go test -race ./...
```

Run the reverse proxy directly:

```bash
go run ./cmd/reverse-proxy
```

When running outside Docker, provide backend URLs through the environment or adjust `configs/config.json`:

```bash
BACKEND1_URL=http://localhost:3000 BACKEND2_URL=http://localhost:3001 go run ./cmd/reverse-proxy
```

## Project Status

This repository focuses on the backend proxy runtime, admin API, configuration management, and observability stack. It is suitable for local experimentation, engineering-thesis demonstrations, and further development.

The codebase is intentionally smaller and simpler than a production proxy platform. Before any real deployment, review network exposure, authentication for admin APIs, TLS termination, persistent configuration, operational limits, and deployment-specific hardening.

## Academic Context

An earlier version of this project was developed and described as part of an engineering thesis. This repository contains a refactored public version with a consolidated Go module layout and updated documentation.

## License

This project is licensed under the MIT License. See `LICENSE` for details.
