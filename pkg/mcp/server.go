package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"

	"github.com/pario-ai/pario/pkg/budget"
	"github.com/pario-ai/pario/pkg/models"
	"github.com/pario-ai/pario/pkg/tracker"
)

// CacheStatter provides cache statistics without coupling to a concrete cache implementation.
type CacheStatter interface {
	Stats() (models.CacheStats, error)
}

// Server is a minimal MCP server that communicates over stdio using JSON-RPC 2.0.
type Server struct {
	tracker  tracker.Tracker
	cache    CacheStatter
	enforcer *budget.Enforcer
	version  string
}

// New creates a new MCP Server.
func New(t tracker.Tracker, cache CacheStatter, enforcer *budget.Enforcer, version string) *Server {
	return &Server{
		tracker:  t,
		cache:    cache,
		enforcer: enforcer,
		version:  version,
	}
}

// Run reads JSON-RPC requests from r line-by-line and writes responses to w.
// It blocks until r is closed or ctx is cancelled.
func (s *Server) Run(ctx context.Context, r io.Reader, w io.Writer) error {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 1024*1024), 1024*1024)

	for scanner.Scan() {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var req Request
		if err := json.Unmarshal(line, &req); err != nil {
			s.writeResponse(w, Response{
				JSONRPC: "2.0",
				Error:   &RPCError{Code: CodeParseError, Message: "parse error"},
			})
			continue
		}

		resp := s.dispatch(ctx, &req)
		if resp == nil {
			// notification — no response
			continue
		}
		s.writeResponse(w, *resp)
	}
	return scanner.Err()
}

func (s *Server) dispatch(ctx context.Context, req *Request) *Response {
	switch req.Method {
	case "initialize":
		return s.handleInitialize(req)
	case "notifications/initialized":
		return nil // notification, no response
	case "tools/list":
		return s.handleToolsList(req)
	case "tools/call":
		return s.handleToolsCall(ctx, req)
	default:
		return &Response{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error:   &RPCError{Code: CodeMethodNotFound, Message: fmt.Sprintf("unknown method: %s", req.Method)},
		}
	}
}

func (s *Server) handleInitialize(req *Request) *Response {
	return &Response{
		JSONRPC: "2.0",
		ID:      req.ID,
		Result: InitializeResult{
			ProtocolVersion: "2024-11-05",
			ServerInfo:      ServerInfo{Name: "pario", Version: s.version},
			Capabilities:    map[string]any{"tools": map[string]any{}},
		},
	}
}

func (s *Server) handleToolsList(req *Request) *Response {
	return &Response{
		JSONRPC: "2.0",
		ID:      req.ID,
		Result:  ToolsListResult{Tools: allTools},
	}
}

func (s *Server) handleToolsCall(ctx context.Context, req *Request) *Response {
	var params ToolCallParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return &Response{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error:   &RPCError{Code: CodeInvalidParams, Message: "invalid params"},
		}
	}

	handler, ok := toolHandlers[params.Name]
	if !ok {
		return &Response{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result: ToolCallResult{
				Content: []ContentBlock{{Type: "text", Text: fmt.Sprintf("unknown tool: %s", params.Name)}},
				IsError: true,
			},
		}
	}

	result := handler(ctx, s, params.Arguments)
	return &Response{
		JSONRPC: "2.0",
		ID:      req.ID,
		Result:  result,
	}
}

func (s *Server) writeResponse(w io.Writer, resp Response) {
	data, err := json.Marshal(resp)
	if err != nil {
		log.Printf("mcp: marshal error: %v", err)
		return
	}
	data = append(data, '\n')
	if _, err := w.Write(data); err != nil {
		log.Printf("mcp: write error: %v", err)
	}
}
