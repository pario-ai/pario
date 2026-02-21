# Prompt Cache

Pario caches exact-match prompt/response pairs in SQLite to avoid redundant upstream calls for identical requests.

## How It Works

1. On each non-streaming request, Pario computes a SHA-256 hash of the model name + serialized messages array.
2. **Cache hit** — returns the stored response immediately with `X-Pario-Cache: hit`. No upstream call is made.
3. **Cache miss** — forwards to the provider, stores the response on success (200 OK), returns it with `X-Pario-Cache: miss`.

### What Gets Cached

- Only non-streaming requests (`stream: false` or omitted)
- Only successful responses (HTTP 200)
- Both OpenAI `/v1/chat/completions` and Anthropic `/v1/messages` responses

### Cache Key

```
SHA-256( model + JSON(messages) )
```

The key is scoped by model, so the same prompt sent to different models produces different cache entries. The primary key in SQLite is `(prompt_hash, model)`.

### TTL

Each entry is stored with a TTL (configurable, default 1 hour). On read, entries older than their TTL are treated as misses. Expired entries remain in the database until explicitly cleared.

## CLI: `pario cache`

```bash
# Show cache statistics
pario cache stats -c pario.yaml

# Clear all entries
pario cache clear -c pario.yaml

# Clear only expired entries
pario cache clear --expired -c pario.yaml
```

### Stats Output

```
Entries: 142
Hits:    891
Misses:  256
```

Hit/miss counters are in-memory (`atomic.Int64`) and reset when the proxy restarts.

## Configuration

```yaml
cache:
  enabled: true    # set to false to disable caching entirely
  ttl: 1h          # time-to-live for cached responses
```

When `enabled: false`, the proxy skips all cache lookups and stores.

## Limitations

- **Exact match only** — even a single character difference in messages produces a different hash. No semantic similarity matching.
- **Local to one instance** — SQLite is per-process. In multi-replica deployments, each pod has its own cache.
- **No streaming** — streamed responses are not cached.
- **In-memory counters** — hit/miss stats reset on restart.

## Source Files

- `pkg/cache/sqlite/cache.go` — `Cache` struct with Get/Put/Stats/Clear/Close
- `pkg/models/cache.go` — `CacheStats` type
- `cmd/pario/cache.go` — CLI cache commands
