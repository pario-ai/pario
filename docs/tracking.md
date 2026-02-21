# Token Usage Tracking

Every non-streaming request proxied through Pario is recorded in a SQLite database with per-request token counts, enabling usage analysis and budget enforcement.

## What Gets Tracked

Each successful (200 OK) proxied request produces a `UsageRecord`:

| Field | Description |
|-------|-------------|
| `api_key` | The client's API key (identification, not the provider key) |
| `model` | The model name from the provider's response |
| `session_id` | Auto-detected or explicitly provided session |
| `prompt_tokens` | Input tokens consumed |
| `completion_tokens` | Output tokens generated |
| `total_tokens` | Sum of prompt + completion |
| `created_at` | UTC timestamp |

Records are stored in the `usage_records` SQLite table with an index on `(api_key, created_at)` for efficient time-range queries.

## Session Tracking

Pario groups related requests into sessions automatically or by explicit client control.

### Auto-Detection

If no `X-Pario-Session` header is sent, Pario finds the most recent session for the client's API key. If the last activity was within the configured `gap_timeout` (default 30 minutes), the request is added to that session. Otherwise, a new session is created.

Session IDs are formatted as `sess_YYYYMMDD_<random>` (e.g., `sess_20260221_a3f9c2`).

### Explicit Sessions

Send `X-Pario-Session: my-session-id` to force a specific session. Pario creates the session row if it doesn't exist. The response always echoes the session ID back via the same header.

### Session Counters

Each session tracks:
- `request_count` — incremented on every request
- `total_tokens` — running sum of tokens
- `started_at` / `last_activity` — time range

### Context Growth

When viewing session details, each request shows a `context_growth` field — the difference in prompt tokens between consecutive requests. This reveals how quickly the conversation context is expanding.

## CLI: `pario stats`

```bash
# Usage summary by API key and model
pario stats -c pario.yaml

# Filter by API key
pario stats -c pario.yaml --api-key sk-client-123

# List sessions
pario stats -c pario.yaml --sessions

# Session detail with context growth
pario stats -c pario.yaml --session-id sess_20260221_a3f9c2
```

### Output Examples

**Usage summary:**
```
API KEY     MODEL      REQUESTS  PROMPT  COMPLETION  TOTAL
sk-abc123   gpt-4           42    8400        2100  10500
sk-abc123   claude-3         8    1600         400   2000
```

**Session detail:**
```
#   TIME                 PROMPT  COMPLETION  TOTAL  CONTEXT GROWTH
1   2026-02-21T10:00:00     120          30    150  -
2   2026-02-21T10:01:15     180          25    205  +60
3   2026-02-21T10:02:30     240          35    275  +60
```

## Configuration

```yaml
db_path: "pario.db"           # SQLite database for usage records and sessions
session:
  gap_timeout: 30m            # inactivity gap to start a new session
```

## Source Files

- `pkg/tracker/tracker.go` — `Tracker` interface and `SQLiteTracker` implementation
- `pkg/models/usage.go` — `UsageRecord`, `Session`, `SessionRequest`, `UsageSummary` types
- `cmd/pario/stats.go` — CLI stats command
