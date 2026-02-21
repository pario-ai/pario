# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project

Pario — a Kubernetes-native token cost control plane for LLM APIs. It sits between your applications and LLM providers to track, budget, cache, and route token usage.

## Language & Toolchain

- Go 1.23+ with modules (`github.com/pario-ai/pario`)
- CLI framework: cobra (`github.com/spf13/cobra`)
- Linting: `golangci-lint`
- Releases: GoReleaser
- CI: GitHub Actions
- Environment variables via `.env` (gitignored)

## Build Commands

- `make build` — compile binary to `bin/pario`
- `make test` — run all tests with race detector
- `make lint` — run golangci-lint
- `make run` — build and run
- `make clean` — remove bin/ and dist/

## Architecture

```
cmd/pario/        — CLI entrypoint (cobra subcommands: proxy, stats, top, mcp, cache, budget)
cmd/operator/     — K8s operator (future)
pkg/proxy/        — reverse proxy for LLM APIs
pkg/tracker/      — token usage tracking
pkg/cache/sqlite/ — local semantic cache
pkg/cache/redis/  — distributed semantic cache
pkg/budget/       — budget enforcement & policies
pkg/router/       — model routing logic
pkg/metrics/      — Prometheus metrics
pkg/mcp/          — MCP server integration
pkg/config/       — configuration loading
pkg/models/       — shared domain types
api/v1alpha1/     — CRD type definitions
deploy/           — Helm charts, Dockerfiles
configs/examples/ — example configuration files
```

## Conventions

- Keep packages small and focused; avoid circular imports
- Use table-driven tests
- All exported functions need doc comments
- Error wrapping with `fmt.Errorf("context: %w", err)`
- Version injected via ldflags at build time

## Code Quality

- After finishing any implementation work, always run `make lint` and `make test` before considering the task complete
- Fix all lint errors and test failures before moving on — never leave broken code
- A PostToolUse hook in `.claude/settings.json` automatically runs `make lint` after every file edit/write
- If the lint hook reports errors, fix them immediately before making further changes
