package cli

import (
	"context"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// defaultReleaseAfter is how long the MCP server waits with no tool call
// before it hands back what calls loaded: the search vectors (~650 MB), the
// embedding model (~200 MB) and the GDT caches (~250 MB). The next call loads
// what it needs again, ~2.3 s for a search and ~0.6 s for a GDT lookup.
const defaultReleaseAfter = 10 * time.Minute

// releaseAfter is defaultReleaseAfter, or T7KB_RELEASE_AFTER as a Go duration
// (90s, 30m); 0, off or never keeps everything loaded once loaded.
func releaseAfter() time.Duration {
	v := strings.ToLower(strings.TrimSpace(os.Getenv("T7KB_RELEASE_AFTER")))
	switch v {
	case "":
		return defaultReleaseAfter
	case "0", "off", "never", "no", "false":
		return 0
	}
	if d, err := time.ParseDuration(v); err == nil && d > 0 {
		return d
	}
	return defaultReleaseAfter
}

// idleRelease runs release once the server has been idle for after: no call
// running, at least one since the last release, and the last ended that long
// ago. Release holds the lock, so a call starting meanwhile waits for it and
// then loads what it needs.
type idleRelease struct {
	after   time.Duration
	release func()

	mu       sync.Mutex
	inflight int
	last     time.Time // when the last call ended
	used     bool      // a call ran since the last release
}

func newIdleRelease(after time.Duration, release func()) *idleRelease {
	return &idleRelease{after: after, release: release}
}

// wrap counts every call of tools.
func (r *idleRelease) wrap(tools []server.ServerTool) []server.ServerTool {
	out := make([]server.ServerTool, len(tools))
	for i, t := range tools {
		h := t.Handler
		t.Handler = func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			r.begin()
			defer r.end(time.Now)
			return h(ctx, req)
		}
		out[i] = t
	}
	return out
}

func (r *idleRelease) begin() {
	r.mu.Lock()
	r.inflight++
	r.used = true
	r.mu.Unlock()
}

func (r *idleRelease) end(now func() time.Time) {
	r.mu.Lock()
	r.inflight--
	r.last = now()
	r.mu.Unlock()
}

// check releases if the server has been idle long enough at now, and says
// whether it did.
func (r *idleRelease) check(now time.Time) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.inflight > 0 || !r.used || now.Sub(r.last) < r.after {
		return false
	}
	r.used = false
	r.release()
	return true
}

// run checks, a few times per after (at most once a minute), until stop is
// closed.
func (r *idleRelease) run(stop <-chan struct{}) {
	every := min(time.Minute, max(r.after/4, 10*time.Millisecond))
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		select {
		case <-stop:
			return
		case now := <-t.C:
			r.check(now)
		}
	}
}
