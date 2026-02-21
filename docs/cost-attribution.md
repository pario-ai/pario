# Cost Attribution & Chargeback

Pario can attribute token costs to teams, projects, and environments for internal billing and chargeback.

## Configuration

Add an `attribution` section to your `pario.yaml`:

```yaml
attribution:
  enabled: true
  pricing:
    - model: gpt-4
      prompt_cost_per_1k: 0.03
      completion_cost_per_1k: 0.06
    - model: gpt-3.5-turbo
      prompt_cost_per_1k: 0.0005
      completion_cost_per_1k: 0.0015
  key_labels:
    sk-backend-team:
      team: backend
      project: api
      env: production
```

## Request Headers

Attach labels per-request using headers:

- `X-Pario-Team` — team name
- `X-Pario-Project` — project name
- `X-Pario-Env` — environment (e.g., production, staging)

If headers are not set, Pario falls back to `key_labels` config mapping based on the API key.

## CLI

```bash
# Show costs for current month
pario cost -c pario.yaml

# Filter by team
pario cost -c pario.yaml --team backend

# Filter by project and custom date range
pario cost -c pario.yaml --project api --since 2025-01-01
```

## MCP Tool

The `pario_cost_report` tool is available via the MCP server:

```json
{
  "name": "pario_cost_report",
  "arguments": {
    "team": "backend",
    "since": "2025-01-01"
  }
}
```

Returns a formatted table with team, project, model, request count, tokens, and estimated cost.
