# Model Routing & Fallback

The router resolves client-requested model names to an ordered chain of upstream provider+model pairs, enabling model aliasing and automatic failover.

## How It Works

When a request arrives, the router checks the `model` field against configured routes:

1. **Match found** — returns the route's ordered target list. Each target specifies a provider name and optional upstream model name.
2. **No match** — returns a single-entry list with the first provider and the original model name (backward compatible).

The proxy then iterates through the resolved routes. On **transport errors** or **5xx responses**, it moves to the next route. On **success** or **4xx errors**, it stops immediately. If all routes fail, the last error response is returned.

### Model Rewriting

The `model` field in the JSON request body is rewritten to match each route's target model before forwarding. If a target's model is empty, the original requested name is used.

## Configuration

```yaml
providers:
  - name: openai
    url: https://api.openai.com
    api_key: ${OPENAI_API_KEY}
  - name: anthropic
    url: https://api.anthropic.com
    api_key: ${ANTHROPIC_API_KEY}

router:
  routes:
    - model: fast                    # client-facing alias
      targets:
        - provider: openai           # try first
          model: gpt-4o-mini
        - provider: anthropic        # fallback
          model: claude-haiku-4-5

    - model: smart
      targets:
        - provider: anthropic
          model: claude-sonnet-4-20250514
        - provider: openai
          model: gpt-4o
```

With this config, a client requesting `model: "fast"` gets routed to `gpt-4o-mini` on OpenAI. If OpenAI returns a 5xx or is unreachable, Pario automatically retries with `claude-haiku-4-5` on Anthropic.

## Retry Behavior

| Condition | Action |
|-----------|--------|
| Transport error (connection refused, timeout) | Retry next route |
| HTTP 5xx (500, 502, 503, etc.) | Retry next route |
| HTTP 4xx (400, 401, 403, 404, 422) | Stop, return to client |
| HTTP 2xx | Stop, return to client |
| All routes exhausted | Return last error response |

## No Routes Configured

When the `router.routes` list is empty or omitted, all requests go to `providers[0]` with the original model name. This preserves the default single-provider behavior.

## Source Files

- `pkg/router/router.go` — route resolution logic
- `pkg/router/router_test.go` — tests for aliasing, fallback, unknown providers
- `pkg/config/config.go` — `RouterConfig`, `RouteConfig`, `RouteTarget` types
