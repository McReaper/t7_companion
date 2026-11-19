package cli

import (
	"context"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/mcp"

	"github.com/McReaper/t7_companion/internal/embed"
)

// T7KB_RELEASE_AFTER sets how long the server waits idle: a Go duration, or
// 0/off/never to keep what it loaded; anything else keeps the default.
func TestReleaseAfterReadsTheEnvironment(t *testing.T) {
	for v, want := range map[string]time.Duration{
		"": defaultReleaseAfter, "90s": 90 * time.Second, " 30M ": 30 * time.Minute,
		"0": 0, "off": 0, "Never": 0, "no": 0, "false": 0,
		"soon": defaultReleaseAfter, "-5m": defaultReleaseAfter,
	} {
		t.Setenv("T7KB_RELEASE_AFTER", v)
		if got := releaseAfter(); got != want {
			t.Errorf("T7KB_RELEASE_AFTER=%q: %v, want %v", v, got, want)
		}
	}
}

// The server releases only once idle: no call running, one since the last
// release, the last ended at least the wait ago; then not again until another
// call.
func TestIdleReleaseWaitsForAnIdleServer(t *testing.T) {
	var released atomic.Int32
	r := newIdleRelease(10*time.Minute, func() { released.Add(1) })
	t0 := time.Date(2026, 11, 19, 12, 0, 0, 0, time.UTC)
	at := func(d time.Duration) func() time.Time { return func() time.Time { return t0.Add(d) } }

	if r.check(t0.Add(time.Hour)) {
		t.Fatal("released before any call")
	}
	r.begin()
	if r.check(t0.Add(time.Hour)) {
		t.Fatal("released during a call")
	}
	r.end(at(0))
	if r.check(t0.Add(9 * time.Minute)) {
		t.Fatal("released 9 minutes after the call, want 10")
	}
	if !r.check(t0.Add(10 * time.Minute)) {
		t.Fatal("not released 10 minutes after the call")
	}
	if r.check(t0.Add(time.Hour)) {
		t.Fatal("released again with no call since")
	}
	r.begin()
	r.end(at(2 * time.Hour))
	if !r.check(t0.Add(2*time.Hour + 10*time.Minute)) {
		t.Fatal("not released after the next call")
	}
	if n := released.Load(); n != 2 {
		t.Errorf("released %d times, want 2", n)
	}
}

// Every tool call goes through the count, an error or a refused call too.
func TestIdleReleaseCountsEveryToolCall(t *testing.T) {
	t.Setenv("TA_TOOLS_PATH", "")
	r := newIdleRelease(time.Hour, func() {})
	c, err := client.NewInProcessClient(newMCPServer(nil, nil, r))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c.Close() })
	ctx := context.Background()
	if err := c.Start(ctx); err != nil {
		t.Fatal(err)
	}
	init := mcp.InitializeRequest{}
	init.Params.ProtocolVersion = mcp.LATEST_PROTOCOL_VERSION
	if _, err := c.Initialize(ctx, init); err != nil {
		t.Fatal(err)
	}
	req := mcp.CallToolRequest{}
	req.Params.Name = "gdt_find"
	req.Params.Arguments = map[string]any{"name": "x"}
	before := time.Now()
	if _, err := c.CallTool(ctx, req); err != nil {
		t.Fatal(err)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.used || r.inflight != 0 || r.last.Before(before) {
		t.Errorf("after one call: used %v, in flight %d, last %v (want used, 0, after %v)", r.used, r.inflight, r.last, before)
	}
}

// run checks on its own, and returns once stopped.
func TestIdleReleaseRunsUntilStopped(t *testing.T) {
	released := make(chan struct{}, 1)
	r := newIdleRelease(20*time.Millisecond, func() { released <- struct{}{} })
	r.begin()
	r.end(time.Now)
	stop, done := make(chan struct{}), make(chan struct{})
	go func() { r.run(stop); close(done) }()
	select {
	case <-released:
	case <-time.After(5 * time.Second):
		t.Fatal("run never released")
	}
	close(stop)
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("run didn't return once stopped")
	}
}

// releaseLoaded drops the vectors a search loaded and the open workspaces;
// the next search and the next call load them again.
func TestReleaseLoadedDropsWhatCallsLoaded(t *testing.T) {
	kbDB(t, map[string]string{"wiki::a": "zombie spawner"})
	st, err := openStore()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = st.Close() }()
	st.EnableVectorIndex()
	search := func() {
		t.Helper()
		if _, err := st.SearchHybrid(context.Background(), "zombie", []float32{1, 0, 0}, 5); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("LOCALAPPDATA", t.TempDir())
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	root := t.TempDir()
	for _, d := range []string{"deffiles", "share/raw/techsetdefs_stable"} {
		if err := os.MkdirAll(filepath.Join(root, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(releaseWorkspaces)

	search()
	w1, err := workspace(root)
	if err != nil {
		t.Fatal(err)
	}
	if w, _ := workspace(root); w != w1 {
		t.Fatal("a second call opened the workspace again")
	}
	releaseLoaded(st, embed.Lazy())
	if st.ReleaseVectors() {
		t.Error("the vectors outlived the release")
	}
	if w, _ := workspace(root); w == w1 {
		t.Error("the workspace outlived the release")
	}
	search()
	if !st.ReleaseVectors() {
		t.Error("the search after the release didn't load the vectors")
	}
}
