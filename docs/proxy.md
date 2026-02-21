# Reverse Proxy

The core of Pario. A reverse proxy that sits between your applications and LLM providers, transparently forwarding requests while adding tracking, caching, budgeting, and routing.

## How It Works

The proxy listens on a configurable address (default `:8080`) and exposes three route groups:

| Endpoint | Provider | Description |
|----------|----------|-------------|
| `POST /v1/chat/completions` | OpenAI-compatible | Chat completions with full tracking pipeline |
| `POST /v1/messages` | Anthropic | Messages API with full tracking pipeline |
| `* /` | First provider | Raw passthrough via Go's `httputil.ReverseProxy` |

### Request Lifecycle (chat/completions and messages)

```
Client request
  │
  ├─ Extract API key (Authorization: Bearer or x-api-key header)
  ├─ Parse request body (extract model, messages, stream flag)
  ├─ Cache check → return cached response on hit
  ├─ Budget check → reject with 429 if over limit
  ├─ Router resolve → get ordered provider+model fallback chain
  │
  ├─ Fallback loop:
  │   ├─ Rewrite model name in request body
  │   ├─ Forward to upstream provider
  │   ├─ On transport error or 5xx → try next route
  │   └─ On success or 4xx → stop
  │
  ├─ Session resolution (auto-detect or explicit via X-Pario-Session)
  ├─ Usage tracking (record prompt/completion/total tokens)
  ├─ Cache store (on 200 OK, non-streaming)
  └─ Forward response to client
```

### Authentication

The proxy uses the client's API key for **identification** (tracking, budgeting) but authenticates to upstream providers using the **provider's** API key from config. Clients never need provider credentials.

- OpenAI routes: `Authorization: Bearer <key>` → upstream gets provider key
- Anthropic routes: `x-api-key: <key>` → upstream gets provider key
- The `anthropic-version` header is forwarded when present

### Passthrough

Any request not matching `/v1/chat/completions` or `/v1/messages` is reverse-proxied to the first configured provider with no tracking, caching, or budget enforcement.

## CLI

```bash
pario proxy -c pario.yaml
```

| Flag | Default | Description |
|------|---------|-------------|
| `-c, --config` | `pario.yaml` | Path to config file |

The proxy handles graceful shutdown on SIGINT/SIGTERM with a 5-second drain timeout.

## Configuration

```yaml
listen: ":8080"
db_path: "pario.db"
providers:
  - name: openai
    type: openai
    url: https://api.openai.com
    api_key: ${OPENAI_API_KEY}
  - name: anthropic
    type: anthropic
    url: https://api.anthropic.com
    api_key: ${ANTHROPIC_API_KEY}
```

Environment variables in config values are expanded at load time (`${VAR}` syntax).

## Source Files

- `cmd/pario/proxy.go` — CLI command wiring
- `pkg/proxy/proxy.go` — HTTP handlers, fallback loop, upstream helpers
- `pkg/config/config.go` — configuration types and loading
