// Package journal is the debug record T7KB_DEBUG turns on: each command and
// each MCP tool call t7kb serves, what it was given and what it answered, one
// JSON line each in journal.jsonl, and the images it answered beside it, so
// what an agent did can be read back as it happened rather than as it tells.
// It is off unless asked for, and writes nothing then.
package journal

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Env is the variable that turns the journal on: a folder to write it in, or
// 1 for debug/ beside the binary.
const Env = "T7KB_DEBUG"

// File is the journal's name in its folder.
const File = "journal.jsonl"

// maxField bounds a text field of an entry: a whole map's map_find, a page of
// a document, fit; a runaway output is cut.
const maxField = 1 << 20

// Dir is the folder the journal writes to, "" when it is off.
func Dir() string {
	v := strings.TrimSpace(os.Getenv(Env))
	switch strings.ToLower(v) {
	case "", "0", "false", "off", "no":
		return ""
	case "1", "true", "on", "yes":
		if exe, err := os.Executable(); err == nil {
			return filepath.Join(filepath.Dir(exe), "debug")
		}
		return "debug"
	}
	return v
}

// Entry is one call.
type Entry struct {
	Time    string         `json:"time"`
	Via     string         `json:"via"`             // cli or mcp
	Command string         `json:"command"`         // "map view", or the tool's name
	Args    []string       `json:"args,omitempty"`  // a command's, as typed
	Input   map[string]any `json:"input,omitempty"` // a tool's arguments
	Stdin   string         `json:"stdin,omitempty"` // what a command read from its input (map edit --ops -)
	Output  string         `json:"output,omitempty"`
	Errors  string         `json:"errors,omitempty"` // a command's stderr
	Failed  bool           `json:"failed,omitempty"` // a command that exited non-zero, a tool error
	MS      int64          `json:"ms"`
	Images  []string       `json:"images,omitempty"` // files beside the journal
}

var mu sync.Mutex // one process's calls append one at a time

// Record appends the entry to the journal in dir, the folder made if needed.
func Record(dir string, e Entry) error {
	e.Stdin, e.Output, e.Errors = cut(e.Stdin), cut(e.Output), cut(e.Errors)
	b, err := json.Marshal(e)
	if err != nil {
		return err
	}
	mu.Lock()
	defer mu.Unlock()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(filepath.Join(dir, File), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	if _, err := f.Write(append(b, '\n')); err != nil {
		_ = f.Close()
		return err
	}
	return f.Close()
}

// Stamp is an entry's time, as it is written and as its images are named.
func Stamp(t time.Time) string { return t.Format("2006-01-02T15:04:05.000") }

// Image writes an image a call answered beside the journal in dir, named by
// the call's time, command and its place among the call's images, and gives
// the file's name.
func Image(dir string, at time.Time, command string, i int, data []byte) (string, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	name := fmt.Sprintf("%s-%s-%d.png", at.Format("20060102-150405.000"), strings.NewReplacer(" ", "_", "/", "_", `\`, "_").Replace(command), i)
	return name, os.WriteFile(filepath.Join(dir, name), data, 0o644)
}

func cut(s string) string {
	if len(s) <= maxField {
		return s
	}
	return s[:maxField] + fmt.Sprintf("\n… (%d more bytes)", len(s)-maxField)
}
