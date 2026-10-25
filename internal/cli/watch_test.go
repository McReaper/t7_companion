package cli

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestWatchSaysTheGamesProblemsAndEachBuildUntilStopped(t *testing.T) {
	game := gameFixture(t)
	oldEvery, oldSettle := watchEvery, watchSettle
	t.Cleanup(func() { watchEvery, watchSettle = oldEvery, oldSettle })
	watchEvery, watchSettle = 10*time.Millisecond, 0
	ctx, cancel := context.WithCancel(context.Background())
	var b safeBuffer
	done := make(chan error, 1)
	go func() { done <- watch(ctx, game, &b, fakeProcs{}, func(string) bool { return false }) }()
	time.Sleep(50 * time.Millisecond)
	f, err := os.OpenFile(filepath.Join(game, "console_mp.log"), os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = f.WriteString("******* script error *******\nError: a new one\nError: ****************\n")
	f.Close()
	deps := filepath.Join(game, "usermaps", "zm_test", "zone_source", "all", "assetinfo", "zm_test.deps")
	if err := os.MkdirAll(filepath.Dir(deps), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(deps, []byte("version,1,1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	at := time.Now().Add(time.Second) // past the watch's start whatever the file system's clock
	if err := os.Chtimes(deps, at, at); err != nil {
		t.Fatal(err)
	}
	want := "script error: a new one, console_mp.log line 50\nlink of usermaps/zm_test ok\n"
	for i := 0; i < 200 && b.String() != want; i++ {
		time.Sleep(10 * time.Millisecond)
	}
	cancel()
	if err := <-done; err != nil || b.String() != want {
		t.Errorf("a line for the new error and one for the link, none for what was there: %v\n got %q\nwant %q", err, b.String(), want)
	}
	t.Setenv("TA_GAME_PATH", "")
	if err := watch(context.Background(), "", &b, fakeProcs{}, nil); err == nil || !strings.Contains(err.Error(), "no game path") {
		t.Errorf("no game root: %v", err)
	}
	cmd := newWatchCmd()
	cmd.SetArgs([]string{"--tools-path", game})
	cmd.SetOut(&b)
	c, stop := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer stop()
	if err := cmd.ExecuteContext(c); err != nil {
		t.Errorf("the command stops with its context: %v", err)
	}
}
