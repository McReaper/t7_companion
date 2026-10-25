package cli

import (
	"bytes"
	"context"
	"encoding/base64"
	"io"
	"os"
	"strings"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/spf13/cobra"

	"github.com/McReaper/t7_companion/internal/journal"
)

// run runs one command line, recorded in the journal when T7KB_DEBUG is on:
// what it was given (its arguments, what it read from its input) and what it
// answered (its output, errors and exit), and the image a map view wrote.
func run(args []string, in io.Reader, out, errs io.Writer) int {
	dir := journal.Dir()
	if dir == "" || !journaled(args) {
		_, code := execute(args, in, out, errs)
		return code
	}
	var read, wrote, failed bytes.Buffer
	start := time.Now()
	cmd, code := execute(args, io.TeeReader(in, &read), io.MultiWriter(out, &wrote), io.MultiWriter(errs, &failed))
	e := journal.Entry{Time: journal.Stamp(start), Via: "cli", Command: commandName(cmd), Args: args,
		Stdin: read.String(), Output: wrote.String(), Errors: failed.String(), Failed: code != 0, MS: time.Since(start).Milliseconds()}
	if code == 0 {
		e.Images = outImage(dir, start, cmd, e.Command)
	}
	_ = journal.Record(dir, e) // the journal is for debugging: a failure to write it fails no command
	return code
}

// outImage copies beside the journal the image map view wrote, its --out a
// PNG whatever its name, and gives its name in the journal's folder.
func outImage(dir string, start time.Time, cmd *cobra.Command, command string) []string {
	if cmd == nil || command != "map view" {
		return nil
	}
	f := cmd.Flags().Lookup("out")
	if f == nil {
		return nil
	}
	b, err := os.ReadFile(f.Value.String())
	if err != nil {
		return nil
	}
	name, err := journal.Image(dir, start, command, 0, b)
	if err != nil {
		return nil
	}
	return []string{name}
}

// journaled tells whether a command line is a call the journal records: not
// the MCP server, which records each tool call itself, nor the watch, a
// listener, nor the interactive browse.
func journaled(args []string) bool {
	for _, a := range args {
		if !strings.HasPrefix(a, "-") {
			return a != "mcp" && a != "watch"
		}
	}
	return len(args) > 0
}

func commandName(cmd *cobra.Command) string {
	if cmd == nil {
		return ""
	}
	return strings.TrimPrefix(strings.TrimPrefix(cmd.CommandPath(), "t7kb"), " ")
}

// journaledTools wraps each tool's handler to record its calls in the
// journal in dir: its arguments, its text, the images it answered, whether
// it failed.
func journaledTools(dir string, tools []server.ServerTool) []server.ServerTool {
	out := make([]server.ServerTool, len(tools))
	for i, t := range tools {
		h, name := t.Handler, t.Tool.Name
		t.Handler = func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			start := time.Now()
			res, err := h(ctx, req)
			_ = journal.Record(dir, toolEntry(dir, name, start, req, res, err))
			return res, err
		}
		out[i] = t
	}
	return out
}

func toolEntry(dir, name string, start time.Time, req mcp.CallToolRequest, res *mcp.CallToolResult, err error) journal.Entry {
	e := journal.Entry{Time: journal.Stamp(start), Via: "mcp", Command: name, Input: req.GetArguments(), MS: time.Since(start).Milliseconds()}
	if err != nil {
		e.Failed, e.Output = true, err.Error()
		return e
	}
	if res == nil {
		return e
	}
	e.Failed = res.IsError
	var text []string
	for _, c := range res.Content {
		switch c := c.(type) {
		case mcp.TextContent:
			text = append(text, c.Text)
		case mcp.ImageContent:
			if b, err := base64.StdEncoding.DecodeString(c.Data); err == nil {
				if f, err := journal.Image(dir, start, name, len(e.Images), b); err == nil {
					e.Images = append(e.Images, f)
				}
			}
		}
	}
	e.Output = strings.Join(text, "\n")
	return e
}
