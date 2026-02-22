# Prompt/Response Audit Log

Pario includes an opt-in audit log that captures full LLM request/response bodies, timing, and metadata in a dedicated SQLite database. This is designed for regulated industries that require complete audit trails of LLM interactions.

## Configuration

Add an `audit` section to your `pario.yaml`:

```yaml
audit:
  enabled: true
  db_path: "pario_audit.db"      # Separate DB from usage tracking
  retention_days: 90              # Auto-delete entries older than this
  redact_keys: true               # Hash API keys (store hash + 8-char prefix)
  max_body_size: 1048576          # Truncate bodies larger than 1 MB
  include:                        # What to capture
    - prompts                     # Request bodies
    - responses                   # Response bodies
    - metadata                    # Request headers
  exclude_models:                 # Skip audit for these models
    - gpt-3.5-turbo
```

## CLI Commands

### Search audit entries

```bash
pario audit search --model gpt-4 --since 2025-01-01 --limit 20
pario audit search --key-prefix sk-test --session sess-abc123
```

### Show a single entry

```bash
pario audit show --request-id req-abc123
```

### View statistics

```bash
pario audit stats
```

### Manual cleanup

```bash
pario audit cleanup
```

## MCP Integration

The `pario_audit_search` tool is available via the MCP server, allowing AI assistants to search audit entries with filters for model, date range, API key prefix, and session ID.

## Security Notes

- API keys are hashed with SHA-256; only the first 8 characters are stored as a prefix for search
- The audit database is separate from the main usage database
- Bodies are truncated to `max_body_size` to prevent unbounded storage growth
- Automatic hourly retention cleanup removes entries beyond `retention_days`
- The `include` list controls what data is captured — omit `prompts` or `responses` to skip body storage
- Use `exclude_models` to skip logging for high-volume, low-risk models
