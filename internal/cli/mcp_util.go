package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// Helpers shared by every toolset's MCP handlers.

// jsonResult is a tool's structured answer as compact JSON: indentation costs the
// agent tokens, and so does HTML-escaping ("<" written as \u003c).
func jsonResult(v any) *mcp.CallToolResult {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return mcp.NewToolResultErrorFromErr("marshal", err)
	}
	return mcp.NewToolResultText(strings.TrimSpace(buf.String()))
}

// jsonHandler adapts an operation returning (value, error) into an MCP handler:
// an error becomes a tool error the agent can read, never a protocol failure.
func jsonHandler(fn func(context.Context, mcp.CallToolRequest) (any, error)) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		v, err := fn(ctx, req)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		return jsonResult(v), nil
	}
}

// decodeArg re-marshals a loosely typed MCP argument into a Go value, rejecting
// unknown keys.
func decodeArg(v any, into any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	return decodeStrict(b, into)
}

func decodeStrict(b []byte, into any) error {
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.DisallowUnknownFields()
	return dec.Decode(into)
}

// ---- MCP
