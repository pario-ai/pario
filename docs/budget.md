# Budget Enforcement

Pario enforces token usage limits per API key with configurable daily or monthly budgets. Policies can scope to all models or to a specific model, letting you set different limits for expensive and cheap models. When a client exceeds their budget, requests are rejected with HTTP 429.

## How It Works

Before every proxied request (after cache check, before upstream call), the budget enforcer:

1. Finds all policies matching the client's API key (exact match or wildcard `*`) **and** the request's model
2. For each matching policy, sums `total_tokens` from the tracker since the start of the current period
3. If usage >= `max_tokens` for any policy, returns `ErrBudgetExceeded`

The proxy translates this into:
```json
HTTP 429
{"error":{"message":"token budget exceeded","type":"pario_error","code":429}}
```

### Budget Periods

| Period | Window Start |
|--------|-------------|
| `daily` | Midnight UTC of the current day |
| `monthly` | First day of the current month, midnight UTC |

### Policy Matching

Policies are matched by two dimensions: `api_key` and `model`.

**API key matching:**
- `api_key: "*"` — applies to all clients
- `api_key: "sk-abc123"` — applies only to that specific key

**Model matching:**
- `model` omitted or empty — applies to all models (sums all token usage across every model)
- `model: "gpt-4"` — applies only to requests for that specific model (sums only that model's usage)

Multiple policies can match a single request. **All** matching policies must pass for the request to proceed.

## Per-Model Budgets

The optional `model` field on a policy enables per-model token limits. This is useful when you want to cap expensive models while leaving cheaper ones unrestricted, or when you need different limits per model.

### Example: Cap GPT-4 but Allow Unlimited Haiku

```yaml
budget:
  enabled: true
  policies:
    # Global daily cap across all models
    - api_key: "*"
      max_tokens: 1000000
      period: daily

    # Tighter limit specifically for gpt-4
    - api_key: "*"
      model: gpt-4
      max_tokens: 100000
      period: daily
```

With this configuration:
- A request for `gpt-4` is checked against **both** the global policy (all-model usage) and the gpt-4 policy (gpt-4 usage only)
- A request for `claude-haiku` is checked against **only** the global policy
- If gpt-4 usage hits 100K tokens, gpt-4 requests are blocked even if the global 1M limit has not been reached
- Other models continue to work until the global limit is reached

### Example: Per-Model Limits for a Specific Client

```yaml
budget:
  enabled: true
  policies:
    - api_key: "sk-premium-client"
      max_tokens: 5000000
      period: monthly

    - api_key: "sk-premium-client"
      model: claude-sonnet-4-20250514
      max_tokens: 2000000
      period: monthly

    - api_key: "sk-premium-client"
      model: gpt-4
      max_tokens: 500000
      period: monthly
```

This gives `sk-premium-client` a 5M monthly total, but no more than 2M on Sonnet and 500K on GPT-4.

### How Usage Is Counted

| Policy `model` field | What gets summed |
|---------------------|-----------------|
| Empty / omitted | `SUM(total_tokens)` across **all** models for the key |
| `"gpt-4"` | `SUM(total_tokens)` only for records where `model = 'gpt-4'` |

### Backward Compatibility

The `model` field is fully optional. Existing configurations without `model` continue to work exactly as before — policies with no model match all requests and sum all token usage.

## CLI: `pario budget status`

```bash
# Show budget status for all policies
pario budget status -c pario.yaml

# Filter by specific API key
pario budget status -c pario.yaml --api-key sk-abc123
```

### Output

```
API KEY              MODEL     PERIOD   MAX TOKENS   USED      REMAINING
*                    (all)     daily      1000000     423891      576109
*                    gpt-4     daily       100000      58320       41680
sk-premium-client    (all)     monthly    5000000    1200000     3800000
```

Policies without a model filter display `(all)` in the MODEL column.

## Configuration

```yaml
budget:
  enabled: true
  policies:
    # Global daily limit (all models combined)
    - api_key: "*"
      max_tokens: 1000000
      period: daily

    # Per-model daily limit
    - api_key: "*"
      model: gpt-4
      max_tokens: 100000
      period: daily

    # Per-client monthly limit
    - api_key: "sk-premium-client"
      max_tokens: 5000000
      period: monthly
```

When `enabled: false`, no budget checks are performed.

### Policy Fields

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `api_key` | string | yes | API key to match, or `"*"` for all keys |
| `model` | string | no | Model name to scope this policy to. Omit for all models. |
| `max_tokens` | integer | yes | Maximum tokens allowed in the period |
| `period` | string | yes | `"daily"` or `"monthly"` |

## Enforcement Timing

Budget is checked **before** the upstream call but **after** the cache check. This means:
- Cached responses don't count against the budget (no tokens consumed)
- The check uses historical usage, not the current request's token count
- A request that pushes usage over the limit will succeed, but the next request will be blocked

## Source Files

- `pkg/budget/enforcer.go` — `Enforcer` with `Check(ctx, apiKey, model)` and `Status(ctx, apiKey)` methods
- `pkg/models/budget.go` — `BudgetPolicy` (with `Model` field), `BudgetStatus`, `BudgetPeriod` types
- `pkg/tracker/tracker.go` — `TotalByKey` (all models) and `TotalByKeyAndModel` (single model) queries
- `cmd/pario/budget.go` — CLI budget command
