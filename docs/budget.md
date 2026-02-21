# Budget Enforcement

Pario enforces token usage limits per API key with configurable daily or monthly budgets. When a client exceeds their budget, requests are rejected with HTTP 429.

## How It Works

Before every proxied request (after cache check, before upstream call), the budget enforcer:

1. Finds all policies matching the client's API key (exact match or wildcard `*`)
2. For each policy, sums `total_tokens` from the tracker since the start of the current period
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

Policies are matched by `api_key` and optionally by `model`:
- `api_key: "*"` — applies to all clients
- `api_key: "sk-abc123"` — applies only to that specific key
- `model: "gpt-4"` — applies only to requests for that model
- `model` omitted or empty — applies to all models (sums all token usage)

Multiple policies can match a single key. All must pass for the request to proceed.

## CLI: `pario budget status`

```bash
# Show budget status for all policies
pario budget status -c pario.yaml

# Filter by specific API key
pario budget status -c pario.yaml --api-key sk-abc123
```

### Output

```
API KEY   MODEL    PERIOD   MAX TOKENS   USED    REMAINING
*         (all)    daily      1000000   423891      576109
*         gpt-4    daily       100000    58320       41680
```

## Configuration

```yaml
budget:
  enabled: true
  policies:
    - api_key: "*"
      max_tokens: 1000000
      period: daily

    - api_key: "*"
      model: gpt-4
      max_tokens: 100000
      period: daily

    - api_key: "sk-premium-client"
      max_tokens: 5000000
      period: monthly
```

When `enabled: false`, no budget checks are performed.

## Enforcement Timing

Budget is checked **before** the upstream call but **after** the cache check. This means:
- Cached responses don't count against the budget (no tokens consumed)
- The check uses historical usage, not the current request's token count
- A request that pushes usage over the limit will succeed, but the next request will be blocked

## Source Files

- `pkg/budget/enforcer.go` — `Enforcer` with Check/Status methods
- `pkg/models/budget.go` — `BudgetPolicy`, `BudgetStatus`, `BudgetPeriod` types
- `cmd/pario/budget.go` — CLI budget command
