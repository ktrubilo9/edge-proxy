# Edge Proxy

[![CI](https://github.com/ktrubilo9/edge-proxy/actions/workflows/ci.yml/badge.svg)](https://github.com/ktrubilo9/edge-proxy/actions/workflows/ci.yml)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)
[![Project status: pre-alpha](https://img.shields.io/badge/status-pre--alpha-orange.svg)](#project-status)

Edge Proxy is an experimental, runtime-configurable reverse proxy and edge
gateway written in Go. It routes HTTP traffic by host and path, tracks backend
health, applies per-virtual-host rate limiting, exposes Prometheus metrics, and
accepts authenticated runtime configuration updates.

The project is currently pre-alpha. It is useful for local experiments,
education, benchmarking, and architecture work, but it has not completed a
security audit and should not be treated as a production security boundary.

## What Edge Proxy Does Today

At runtime, Edge Proxy:

- accepts HTTP requests on the public proxy port;
- selects a virtual host by the request `Host` header;
- selects optional path routes using longest-match precedence;
- can strip a matched path prefix before forwarding upstream;
- forwards only to enabled backends that are currently marked active;
- supports `least-connections` and experimental `adaptive` load balancing;
- retries retryable requests once on an alternate backend;
- runs active backend health checks and exposes backend status;
- resolves client IPs using forwarding headers only from trusted proxy CIDRs;
- applies per-virtual-host rate limiting through reusable security policies;
- publishes validated configuration snapshots atomically;
- persists runtime configuration changes to the configured JSON file;
- exposes a token-protected admin HTTP API backed by token-protected gRPC;
- exports operational metrics for Prometheus;
- includes a Docker Compose stack with two mock backends, Prometheus, and
  Grafana.

Requests always use one complete runtime snapshot. During a configuration
update, a request either sees the previous valid snapshot or the next valid
snapshot; it does not observe a partially applied update.

## Architecture

```mermaid
flowchart LR
    Client["Client"] --> Proxy["Reverse proxy"]
    Admin["Admin HTTP API"] --> GRPC["Authenticated gRPC control plane"]
    GRPC --> Runtime["Atomic runtime state"]
    Runtime --> Proxy
    Proxy --> LB["Load balancer"]
    LB --> A["Backend A"]
    LB --> B["Backend B"]
    Health["Health checker"] --> A
    Health --> B
    Proxy --> Metrics["Prometheus metrics"]
    Metrics --> Grafana["Grafana"]
```

```text
cmd/
  admin-api/        HTTP API for runtime configuration
  mock-backend/     Demo backend used by Docker Compose
  reverse-proxy/    Main proxy process

internal/
  admin/            HTTP handlers and gRPC control plane
  api/              Protocol Buffers definition and generated Go code
  config/           Loading, validation, persistence, and snapshots
  health/           Backend health checking
  lb/               Load-balancing strategies
  metrics/          Runtime and Prometheus metrics
  middleware/       Request middleware and client IP resolution
  proxy/            Proxy runtime, orchestration, and handlers
```

## Quick Start

Requirements:

- Go 1.25 or newer, matching `go.mod`;
- Docker and Docker Compose for the complete local stack.

Create a local environment file:

```bash
cp .env.example .env
```

Replace the example secrets in `.env`, then start the stack:

```bash
docker compose up --build
```

The stack exposes:

| Service | Address |
| --- | --- |
| Reverse proxy | `http://localhost:8080` |
| Admin API | `http://localhost:8081` |
| Prometheus | `http://localhost:9090` |
| Grafana | `http://localhost:3000` |

Send a request through the configured virtual host:

```bash
curl -H "Host: app.example.local" http://localhost:8080/
```

Backends may remain unavailable briefly until the first health check succeeds.

## Configuration

The default configuration is stored in `configs/config.json`. Additional
examples are available in `configs/examples/`.

Values prefixed with `env:` are resolved from environment variables:

```json
{
  "url": "env:BACKEND1_URL",
  "weight": 1,
  "enabled": true
}
```

Important fields:

| Field | Purpose |
| --- | --- |
| `server.proxy_port` | Public proxy HTTP port |
| `server.admin_grpc_port` | Internal admin gRPC port |
| `load_balancing.strategy` | Load-balancing strategy: `least-connections` or experimental `adaptive` |
| `backends` | Upstream backend definitions with stable `id` values |
| `virtual_hosts` | Host-based routing policies using `backend_ids` |
| `path_routes` | Optional path-specific backend selection |
| `health_check` | Health-check interval, timeout, path, and status codes |
| `timeouts` | Outbound HTTP transport timeouts |
| `logging` | Log level and asynchronous logging settings |
| `security.policies` | Reusable security policies referenced by `security_policy_id` |

Runtime changes made through the admin API are persisted to the configured JSON
file. Mount that file as a volume when configuration must survive container
replacement.

Path routes are matched as exact paths or slash-delimited subtrees. For example,
`/api` matches `/api` and `/api/users`, but not `/apix`. When multiple routes
match, the longest route wins. `strip_prefix` is applied only after a route has
matched.

Two operational caveats are worth keeping in mind while the project is
pre-alpha:

- `env:` placeholders are resolved when the runtime loads configuration.
  Runtime updates currently persist resolved values back to the JSON file.
- Updating `server.proxy_port` or `server.admin_grpc_port` changes persisted
  configuration, but live listeners are not restarted yet.

## Admin API

Authentication is enabled by default. Configure separate HTTP and gRPC tokens:

```dotenv
ADMIN_AUTH_ENABLED=true
ADMIN_API_TOKEN=replace-with-a-random-value
ADMIN_GRPC_TOKEN=replace-with-another-random-value
```

Set `ADMIN_AUTH_ENABLED=false` only in an isolated local environment. The gRPC
token is always required, and the unencrypted gRPC transport must remain on a
trusted private network.

Main routes:

```text
GET|POST             /api/backend
GET|PUT|DELETE       /api/backend/{id}
GET|POST             /api/vhost
GET|PUT|DELETE       /api/vhost/{domain}
GET|PUT              /api/vhost/{domain}/security
GET                  /api/security/policies
PUT                  /api/security/policies/{id}
GET|PUT              /api/config/server
GET|PUT              /api/config/lb
```

API documentation:

- [OpenAPI 3.1 specification](docs/openapi.yaml)
- [Admin API guide](docs/api-reference.md)
- [Browsable API reference](https://ktrubilo9.github.io/edge-proxy/api/)

The browsable reference is generated from the checked-in specification and
published through this repository's GitHub Pages workflow.

Example:

```bash
curl http://localhost:8081/api/backend \
  -H "Authorization: Bearer $ADMIN_API_TOKEN"
```

## Observability

Prometheus scrapes the internal operational endpoint:

```text
http://reverse-proxy:9091/metrics/prometheus
```

The public proxy port exposes only a minimal `/health` response. Detailed
operational endpoints are kept on the internal `OPS_ADDR`.

When another proxy is placed in front of Edge Proxy, configure
`TRUSTED_PROXY_CIDRS` with only its network ranges. Forwarded client IP headers
from other peers are ignored.

## Development

Run the complete quality suite:

```bash
gofmt -l .
go vet ./...
go test ./...
go test -race ./...
```

Run the proxy outside Docker:

```bash
go run ./cmd/reverse-proxy
```

## Project Status

Edge Proxy is **pre-alpha**. It is suitable for local experiments, education,
benchmarking, and collaborative development. It has not completed a security
audit and should not be presented as a production-ready security boundary.

The project originated from an engineering thesis. The current repository is a
substantial refactor with a consolidated Go module, immutable configuration
snapshots, atomic runtime publication, a backend state registry, and an
authenticated control plane.

## Current Limitations

- Admin gRPC authentication uses bearer tokens, but the transport is plaintext.
  Keep it on a trusted private network.
- Runtime configuration saves currently persist resolved `env:` values instead
  of preserving the original placeholders.
- Updating `server.proxy_port` or `server.admin_grpc_port` persists the new
  value, but running listeners are not restarted.
- Newly added or freshly started backends may return 503 until health checks
  mark them active.
- The adaptive load balancer is implemented but still experimental.
- Request size, URL length, and header limits are not yet configurable per
  virtual host.

## Contributing and Security

- Read [CONTRIBUTING.md](CONTRIBUTING.md) before opening a pull request.
- Use [GitHub Issues](https://github.com/ktrubilo9/edge-proxy/issues) for bugs
  and feature proposals.
- Report vulnerabilities according to [SECURITY.md](SECURITY.md), not through a
  public issue.

## License

Edge Proxy is available under the [MIT License](LICENSE).
