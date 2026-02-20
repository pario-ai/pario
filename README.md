# Pario

**Kubernetes-native token cost control plane for LLM APIs.**

Pario sits between your applications and LLM providers (OpenAI, Anthropic, etc.) to give you full visibility and control over token spend — budgets, caching, routing, and real-time observability.

## Features (Planned)

- **Transparent Proxy** — drop-in replacement for LLM API endpoints
- **Token Budgets** — per-team, per-app, per-model spend limits with CRDs
- **Semantic Caching** — deduplicate similar prompts (SQLite local, Redis distributed)
- **Smart Routing** — route requests across models based on cost/latency/quality
- **Live Observability** — `pario top` for real-time token usage, Prometheus metrics
- **MCP Server** — expose cost data to AI agents via Model Context Protocol

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
