# MCP Server

Pario includes a Model Context Protocol (MCP) server that exposes usage tracking, session, budget, and cache data as tools. This allows AI assistants (like Claude) to query Pario's data directly.

## How It Works

The MCP server communicates over **stdio** using JSON-RPC 2.0, one message per line. It implements the MCP protocol:

1. Client sends `initialize` → server responds with capabilities
2. Client sends `notifications/initialized` → server acknowledges (no response)
3. Client calls `tools/list` → server returns available tools
4. Client calls `tools/call` with a tool name and arguments → server returns results

## Available Tools

| Tool | Description | Arguments |
|------|-------------|-----------|
| `pario_stats` | Aggregated token usage by API key and model | `api_key` (optional) |
| `pario_sessions` | List tracked sessions | `api_key` (optional) |
| `pario_session_detail` | Per-request detail with context growth for a session | `session_id` (required) |
| `pario_budget` | Budget status: usage vs limits | `api_key` (optional) |
| `pario_cache_stats` | Cache entries, hits, misses, hit rate | none |

All tools return formatted text tables.

## CLI

```bash
pario mcp -c pario.yaml
```

This starts the MCP server on stdin/stdout. It is designed to be launched by an MCP client (e.g., Claude Desktop, Claude Code) as a subprocess.

### Claude Desktop Integration

Add to your Claude Desktop MCP config:
```json
{
  "mcpServers": {
    "pario": {
      "command": "pario",
      "args": ["mcp", "-c", "/path/to/pario.yaml"]
    }
  }
}
```

## Example Tool Call

Request:
```json
{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"pario_stats","arguments":{"api_key":"sk-abc123"}}}
```

Response:
```json
{"jsonrpc":"2.0","id":1,"result":{"content":[{"type":"text","text":"API Key              Model                     Requests     Prompt Completion      Total\n..."}]}}
```

## Protocol Details

- Protocol version: `2024-11-05`
- Server name: `pario`
- Capabilities: `tools`
- Max message size: 1 MB

## Source Files

- `pkg/mcp/server.go` — JSON-RPC dispatch loop
- `pkg/mcp/tools.go` — tool definitions and handlers
- `pkg/mcp/format.go` — text table formatting
- `pkg/mcp/types.go` — JSON-RPC and MCP protocol types
- `cmd/pario/mcp.go` — CLI command
