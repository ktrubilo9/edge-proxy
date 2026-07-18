---
title: Admin API guide
description: Authentication, request conventions, and the generated reference for Edge Proxy's runtime control plane.
---

# Admin API guide

Edge Proxy exposes a small HTTP control plane on port `8081`. It forwards
validated requests to the internal gRPC service, persists successful runtime
changes, and publishes the resulting configuration atomically.

The service is pre-alpha. Keep the admin listener on a trusted network and do
not place real tokens in examples, bug reports, or logs.

## Authentication

HTTP bearer authentication is enabled by default. Set `ADMIN_API_TOKEN` and
send it in every request:

```bash
curl http://localhost:8081/api/backend \
  -H "Authorization: Bearer $ADMIN_API_TOKEN"
```

`ADMIN_AUTH_ENABLED=false` disables HTTP authentication and should only be
used in an isolated local environment. The separate gRPC token remains
required.

## Contract sources

- [`openapi.yaml`](https://github.com/ktrubilo9/edge-proxy/blob/main/docs/openapi.yaml)
  is the machine-readable OpenAPI 3.1 contract.
- `cmd/admin-api/main.go` is the route-registration source of truth.
- `internal/admin/handler` defines HTTP request and response behavior.
- `internal/api/admin.proto` defines the snake_case JSON wire schemas.

The repository test compares the 18 registered method/path pairs with the
OpenAPI document so route changes cannot silently leave the contract behind.

## Operations

| Area | Operations |
| --- | --- |
| Backends | `GET, POST /api/backend`; `GET, PUT, DELETE /api/backend/{id}` |
| Virtual hosts | `GET, POST /api/vhost`; `GET, PUT, DELETE /api/vhost/{domain}` |
| Host security | `GET, PUT /api/vhost/{domain}/security` |
| Policies | `GET /api/security/policies`; `PUT /api/security/policies/{id}` |
| Server config | `GET, PUT /api/config/server` |
| Load balancer | `GET, PUT /api/config/lb` |

The generated **API Reference** tab contains schemas, examples, response
codes, and ready-to-adapt cURL, Go, JavaScript, and Python requests.

## HTTP behavior

- Successful operations return JSON with status `200`. Collection endpoints
  return raw arrays rather than protobuf wrapper objects.
- Invalid JSON returns plain text with status `400`.
- Missing or invalid bearer credentials return `401` and a
  `WWW-Authenticate` header.
- Request bodies are limited to 1 MiB; oversized bodies return `413`.
- Current gRPC, validation, persistence, and missing-resource failures surface
  as plain-text `500` responses.
- CORS preflight requests return `204` before authentication. The default
  allowed origin is `http://localhost:3000` and can be changed with
  `ADMIN_ALLOWED_ORIGIN`.

Updates to `server.proxy_port` and `server.admin_grpc_port` are persisted, but
the running listeners are not restarted yet.

## Build the reference locally

The docs use the exact Sourcey version recorded in `package-lock.json`:

```bash
npm ci
npm run docs:validate
npm run docs:build
```

The static site is written to `dist/`. `npm run docs:dev` starts a local
preview. Do not commit generated `dist/` output; GitHub Actions rebuilds it
from the checked-in contract and publishes the artifact through repository-owned
GitHub Pages.
