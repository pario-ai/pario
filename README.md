# Pario

**Kubernetes-native token cost control plane for LLM APIs.**

Pario sits between your applications and LLM providers (OpenAI, Anthropic, etc.) to give you full visibility and control over token spend — budgets, caching, routing, and real-time observability.

## Features

- **[Transparent Proxy](docs/proxy.md)** — drop-in replacement for OpenAI and Anthropic API endpoints
- **[Token Tracking](docs/tracking.md)** — per-key, per-model usage tracking with session detection
- **[Token Budgets](docs/budget.md)** — per-team, per-app, per-model spend limits
- **[Semantic Caching](docs/cache.md)** — deduplicate similar prompts (SQLite local, Redis distributed)
- **[Smart Routing](docs/routing.md)** — route requests across models with fallback chains
- **[Cost Attribution](docs/cost-attribution.md)** — team/project cost breakdowns with per-model pricing
- **[Audit Log](docs/audit-log.md)** — opt-in full request/response logging for compliance and debugging
- **[MCP Server](docs/mcp-server.md)** — expose stats, budgets, costs, and audit data to AI agents via Model Context Protocol
- **Live Observability** — `pario top` for real-time token usage, Prometheus metrics

## Architecture

```
┌─────────────┐     ┌───────────────────────────────────────┐     ┌──────────────┐
│  Your App   │────▶│                 Pario                  │────▶│  LLM Provider│
│             │◀────│  proxy · budget · cache · router       │◀────│  (OpenAI etc)│
└─────────────┘     └───────────────────────────────────────┘     └──────────────┘
                         │           │           │
                    ┌────▼───┐  ┌────▼───┐  ┌───▼────┐
                    │Metrics │  │ SQLite │  │  CRDs  │
                    │(Prom)  │  │ Cache  │  │(Budget)│
                    └────────┘  └────────┘  └────────┘
```

## Quickstart

```bash
# Install (once releases are available)
# brew install pario-ai/tap/pario

# Or build from source
make build
./bin/pario --help
```

## Development

```bash
make build   # compile to bin/pario
make test    # run tests
make lint    # run golangci-lint
make clean   # remove build artifacts
```

## License

TBD
