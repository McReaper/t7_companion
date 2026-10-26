package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/santhosh-tekuri/jsonschema/v6/kind"
	"golang.org/x/text/language"
	"golang.org/x/text/message"
)

// strictTools makes each tool refuse, before its handler runs, an argument its
// schema doesn't declare or one of the wrong type, with a tool error naming it.
// Dropped silently, a misspelt argument runs the tool on its default (build's
// "stage" for "stages": a full compile and light instead of a link). mcp-go's
// own WithInputSchemaValidation prints the validator's structs instead
// ("<root>: &{[stage]}"), the same for a missing argument as an unknown one.
func strictTools(tools []server.ServerTool) []server.ServerTool {
	out := make([]server.ServerTool, len(tools))
	for i, t := range tools {
		if t.Tool.InputSchema.AdditionalProperties == nil {
			t.Tool.InputSchema.AdditionalProperties = false
		}
		sch, err := compileInputSchema(t.Tool)
		if err != nil {
			panic(err) // a tool's own schema: every test that starts the server fails on it
		}
		h, tool := t.Handler, t.Tool
		t.Handler = func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			if err := checkArgs(sch, tool, req.Params.Arguments); err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			return h(ctx, req)
		}
		out[i] = t
	}
	return out
}

func compileInputSchema(tool mcp.Tool) (*jsonschema.Schema, error) {
	b, err := json.Marshal(tool.InputSchema)
	if err != nil {
		return nil, fmt.Errorf("tool %s: input schema: %w", tool.Name, err)
	}
	doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(b))
	if err != nil {
		return nil, fmt.Errorf("tool %s: input schema: %w", tool.Name, err)
	}
	url := "mem:///t7kb/" + tool.Name + ".json"
	c := jsonschema.NewCompiler()
	if err := c.AddResource(url, doc); err != nil {
		return nil, fmt.Errorf("tool %s: input schema: %w", tool.Name, err)
	}
	return c.Compile(url)
}

// checkArgs validates a call's arguments (none is an empty object, so a
// required one is reported missing) against the tool's schema.
func checkArgs(sch *jsonschema.Schema, tool mcp.Tool, args any) error {
	if args == nil {
		args = map[string]any{}
	}
	b, err := json.Marshal(args)
	if err != nil {
		return fmt.Errorf("%s: arguments: %w", tool.Name, err)
	}
	v, err := jsonschema.UnmarshalJSON(bytes.NewReader(b))
	if err != nil {
		return fmt.Errorf("%s: arguments: %w", tool.Name, err)
	}
	var verr *jsonschema.ValidationError
	if err := sch.Validate(v); !errors.As(err, &verr) {
		return err
	}
	return fmt.Errorf("%s: %s", tool.Name, strings.Join(argProblems(verr, tool), "; "))
}

var english = message.NewPrinter(language.English)

// argProblems says what is wrong with each argument the validator refused, in
// the agent's terms: an unknown argument (with the ones the tool takes), a
// missing one, or one whose value doesn't fit, by its path.
func argProblems(e *jsonschema.ValidationError, tool mcp.Tool) []string {
	if len(e.Causes) > 0 {
		var out []string
		for _, c := range e.Causes {
			out = append(out, argProblems(c, tool)...)
		}
		return out
	}
	at := strings.Join(e.InstanceLocation, ".")
	switch k := e.ErrorKind.(type) {
	case *kind.AdditionalProperties:
		if at == "" {
			return []string{fmt.Sprintf("unknown argument %s (it takes %s)", quoted(k.Properties), strings.Join(argNames(tool), ", "))}
		}
	case *kind.Required:
		if at == "" {
			return []string{"missing argument " + quoted(k.Missing)}
		}
	}
	msg := e.ErrorKind.LocalizedString(english)
	if at != "" {
		msg = "argument " + at + ": " + msg
	}
	return []string{msg}
}

func argNames(tool mcp.Tool) []string {
	names := make([]string, 0, len(tool.InputSchema.Properties))
	for n := range tool.InputSchema.Properties {
		names = append(names, n)
	}
	slices.Sort(names)
	return names
}

func quoted(names []string) string {
	q := make([]string, len(names))
	for i, n := range names {
		q[i] = `"` + n + `"`
	}
	return strings.Join(q, ", ")
}
