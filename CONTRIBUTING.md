# Contributing

Edge Proxy is an early-stage project. Contributions, experiments, bug reports,
and documentation improvements are welcome.

## Before Writing Code

- For a small fix, open a pull request directly.
- For a larger feature or architectural change, open an issue first so the
  approach can be discussed.
- Security vulnerabilities must be reported according to
  [SECURITY.md](SECURITY.md), not in a public issue.

## Development

Create a local environment:

```bash
cp .env.example .env
docker compose up --build
```

Before opening a pull request, run:

```bash
gofmt -l .
go vet ./...
go test ./...
go test -race ./...
```

## Pull Requests

Keep pull requests focused on one change. Include:

- what problem the change solves;
- tests for new or changed behavior;
- documentation when configuration or APIs change;
- benchmark methodology when making performance claims.

Important project rules:

- configuration and mutable runtime state stay separate;
- runtime updates must remain atomic;
- forwarded headers are trusted only from configured proxies;
- secrets, tokens, request bodies, and full authentication claims must not be logged.

Not sure where to start? Look for issues marked `good first issue` or
`help wanted`, or comment on an issue that interests you.
