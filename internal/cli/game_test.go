package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/McReaper/t7_companion/internal/gamelog"
)

// gameFixture is a game root with a usermap's log at the root, an older one
// in a mod's folder, a crashes.log, and the usermap's zone.
func gameFixture(t *testing.T) string {
	t.Helper()
	game := t.TempDir()
	t.Setenv("TA_GAME_PATH", game)
	t.Setenv("TA_TOOLS_PATH", "")
	write := func(rel, body string, ago time.Duration) {
		t.Helper()
		p := filepath.Join(game, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		at := time.Now().Add(-ago)
		if err := os.Chtimes(p, at, at); err != nil {
			t.Fatal(err)
		}
	}
	var log strings.Builder
	log.WriteString("logfile opened on Tue Oct 20 01:52:15 2026\n")
	for i := range 40 {
		fmt.Fprintf(&log, "ERROR: problem %d\n", i)
	}
	log.WriteString("Error: Could not find xmodel \"p7_my_crate\".\nError: Could not find fx \"fx/stock\".\n" +
		"******* script error *******\nError: cannot cast undefined to bool\n******* Call stack *******\n" +
		"Error: \tfile 'scripts/zm/zm_test.gsc', line 42 :: \nError: ********************\nModLoad done: zm_test\n")
	write("console_mp.log", log.String(), time.Minute)
	write("usermaps/zm_test/zone_source/zm_test.zone", "xmodel,p7_my_crate\n", time.Hour)
	write("mods/my_mod/console_mp.log", "logfile opened on Mon Oct 19 22:10:03 2026\nError: Could not find xmodel \"p7_my_crate\".\n", time.Hour)
	write("mods/my_mod/zone_source/core_mod.zone", "xmodel,p7_my_crate\n", time.Hour)
	write("crashes.log", "a crash\n", 2*time.Hour)
	return game
}

func TestGameLogSortsTheCurrentLogAndNamesTheOthers(t *testing.T) {
	game := gameFixture(t)
	out, err := gameLog(gameLogArgs{Name: "zm_test"})
	if err != nil {
		t.Fatal(err)
	}
	if out.Path != filepath.Join(game, "console_mp.log") || out.Opened != "Tue Oct 20 01:52:15 2026" || out.Lines != "2-49 of 49 (the last launch)" {
		t.Errorf("the root's log, its last launch: %+v", out.gameFileOut)
	}
	if out.Loaded != 49 || len(out.ScriptErrors) != 1 || out.ScriptErrors[0].N != 44 {
		t.Errorf("loaded at %d, script errors %v", out.Loaded, out.ScriptErrors)
	}
	if len(out.MapProblems) != gameMaxProblems || out.Unlisted != "11 more script errors and problems past the cap: grep for them" {
		t.Errorf("the map's problems, capped (40 Error lines and its zoned xmodel): %d, %q", len(out.MapProblems), out.Unlisted)
	}
	if out.StockLookups != "1 (1 fx): stock assets the map doesn't zone, which every load fails to find" {
		t.Errorf("stock: %q", out.StockLookups)
	}
	if len(out.Others) != 2 || out.Others[0].Path != filepath.Join(game, "mods", "my_mod", "console_mp.log") || out.Others[1].Path != filepath.Join(game, "crashes.log") {
		t.Errorf("the other logs beside it: %v", out.Others)
	}
	if out.Matches != nil || out.Tail != nil {
		t.Errorf("no raw lines unless asked: %+v", out)
	}
}

func TestGameLogOfAModSortsAgainstItsZonesAndWaitsOnNoMap(t *testing.T) {
	game := gameFixture(t)
	if err := os.Remove(filepath.Join(game, "console_mp.log")); err != nil {
		t.Fatal(err)
	}
	out, err := gameLog(gameLogArgs{Name: "my_mod"})
	if err != nil {
		t.Fatal(err)
	}
	if out.Path != filepath.Join(game, "mods", "my_mod", "console_mp.log") || len(out.MapProblems) != 1 || out.Unlisted != "" || out.Loaded != 0 || out.StockLookups != "" {
		t.Errorf("the mod's log, its zoned xmodel a problem: %+v", out)
	}
}

func TestGameLogGrepsAndTails(t *testing.T) {
	gameFixture(t)
	out, err := gameLog(gameLogArgs{Grep: "PROBLEM 3", Context: 1, Max: 2, Tail: 2})
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Matches) != 2 || out.MoreMatches != 9 || out.Matches[0].N != 5 || len(out.Matches[0].Before) != 1 || len(out.Matches[0].After) != 1 {
		t.Errorf("case-insensitive matches with a line each side, the rest counted: %+v, %d", out.Matches, out.MoreMatches)
	}
	if len(out.Tail) != 2 || out.Tail[1].Text != "ModLoad done: zm_test" {
		t.Errorf("tail: %v", out.Tail)
	}
	out, _ = gameLog(gameLogArgs{Grep: "problem", Context: 99, Max: 999, Tail: 9999})
	if len(out.Matches) != 40 || len(out.Matches[39].Before) != gameMaxContext || len(out.Tail) != 48 {
		t.Errorf("the caps: %d matches, %d before, %d tail", len(out.Matches), len(out.Matches[39].Before), len(out.Tail))
	}
	if out.Loaded != 0 {
		t.Errorf("without a name, no map is waited on: %d", out.Loaded)
	}
}

func TestGameLogReadsALogByPath(t *testing.T) {
	game := gameFixture(t)
	t.Setenv("TA_GAME_PATH", "")
	out, err := gameLog(gameLogArgs{Path: filepath.Join(game, "crashes.log")})
	if err != nil || out.Path != filepath.Join(game, "crashes.log") || out.Lines != "1-1 of 1 (the last launch)" || out.Others != nil {
		t.Errorf("a file by path, with no game root needed: %+v %v", out, err)
	}
	empty := filepath.Join(t.TempDir(), "empty.log")
	if err := os.WriteFile(empty, []byte("logfile opened on Tue\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if out, _ := gameLog(gameLogArgs{Path: empty}); out.Lines != "none to read, of 1" {
		t.Errorf("an empty launch: %q", out.Lines)
	}
}

func TestGameLogErrors(t *testing.T) {
	game := gameFixture(t)
	if _, err := gameLog(gameLogArgs{Grep: "("}); err == nil || !strings.HasPrefix(err.Error(), "grep: ") {
		t.Errorf("a bad expression: %v", err)
	}
	if _, err := gameLog(gameLogArgs{Path: filepath.Join(game, "nope.log")}); err == nil {
		t.Error("no such file")
	}
	t.Setenv("TA_GAME_PATH", t.TempDir())
	if _, err := gameLog(gameLogArgs{}); err == nil || !strings.Contains(err.Error(), "+set logfile 2") {
		t.Errorf("no log says how to get one: %v", err)
	}
	t.Setenv("TA_GAME_PATH", "")
	if _, err := gameLog(gameLogArgs{}); err == nil || !strings.HasPrefix(err.Error(), "no game path") {
		t.Errorf("no game root: %v", err)
	}
	if g, err := gameRoot(game); err != nil || g != game {
		t.Errorf("the tools path stands for the game root: %q %v", g, err)
	}
}

func TestGameLogToolAndCommand(t *testing.T) {
	gameFixture(t)
	res, err := gameLogTool().Handler(context.Background(), mcpRequest("game_log", map[string]any{"name": "zm_test", "grep": "script error", "tail": 1}))
	if err != nil || res.IsError {
		t.Fatalf("tool: %v %s", err, resultText(res))
	}
	var out gameLogOut
	if err := json.Unmarshal([]byte(resultText(res)), &out); err != nil || out.Loaded != 49 || len(out.Matches) != 1 || len(out.Tail) != 1 {
		t.Errorf("tool answer: %v %s", err, resultText(res))
	}
	cmd := newGameCmd()
	var b strings.Builder
	cmd.SetOut(&b)
	cmd.SetArgs([]string{"log", "zm_test", "--tail", "1"})
	if err := cmd.Execute(); err != nil || !strings.Contains(b.String(), `"loaded": 49`) || !strings.Contains(b.String(), `"text": "ModLoad done: zm_test"`) {
		t.Errorf("command: %v %s", err, b.String())
	}
	cmd = newGameCmd()
	cmd.SetOut(&b)
	cmd.SetArgs([]string{"log"})
	if err := cmd.Execute(); err != nil {
		t.Errorf("command without a name: %v", err)
	}
}

// fakeProcs are the game's processes for game_log and the watcher.
type fakeProcs struct{ dialogs []string }

func (f fakeProcs) Running() []int {
	if f.dialogs == nil {
		return nil
	}
	return []int{7}
}
func (f fakeProcs) Dialogs(int) []string    { return f.dialogs }
func (f fakeProcs) Exit(int) (uint32, bool) { return 0, true }

func onProcs(t *testing.T, f fakeProcs) {
	t.Helper()
	old := gameProcs
	t.Cleanup(func() { gameProcs = old })
	gameProcs = func() gamelog.Game { return f }
}

func TestGameLogReadsOnlyWhatIsNewAndTheDialogsOpen(t *testing.T) {
	gameFixture(t)
	onProcs(t, fakeProcs{dialogs: []string{`the game opened a dialog, "Erreur fatale": ERROR: x [OK]`}})
	out, err := gameLog(gameLogArgs{Name: "zm_test", After: 43})
	if err != nil {
		t.Fatal(err)
	}
	if out.Lines != "44-49 of 49 (the last launch)" || len(out.MapProblems) != 0 || len(out.ScriptErrors) != 1 || out.Loaded != 49 {
		t.Errorf("only the lines after 43: %+v", out)
	}
	if len(out.Dialogs) != 1 {
		t.Errorf("the running game's dialogs: %q", out.Dialogs)
	}
	if out, _ := gameLog(gameLogArgs{Path: filepath.Join(os.Getenv("TA_GAME_PATH"), "crashes.log")}); out.Dialogs != nil {
		t.Errorf("a log read by path is no running game's: %q", out.Dialogs)
	}
}

func TestGameWatchSaysTheGamesProblemsUntilStopped(t *testing.T) {
	game := gameFixture(t)
	old := gameWatchEvery
	t.Cleanup(func() { gameWatchEvery = old })
	gameWatchEvery = 10 * time.Millisecond
	ctx, cancel := context.WithCancel(context.Background())
	var b safeBuffer
	done := make(chan error, 1)
	go func() { done <- gameWatch(ctx, "", &b, fakeProcs{}) }()
	time.Sleep(50 * time.Millisecond)
	f, err := os.OpenFile(filepath.Join(game, "console_mp.log"), os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = f.WriteString("******* script error *******\nError: a new one\nError: ****************\n")
	f.Close()
	for i := 0; i < 200 && !strings.Contains(b.String(), "script error: a new one, console_mp.log line 50"); i++ {
		time.Sleep(10 * time.Millisecond)
	}
	cancel()
	if err := <-done; err != nil || b.String() != "script error: a new one, console_mp.log line 50\n" {
		t.Errorf("one line for the new error, none for what was there: %v %q", err, b.String())
	}
	t.Setenv("TA_GAME_PATH", "")
	if err := gameWatch(context.Background(), "", &b, fakeProcs{}); err == nil {
		t.Error("no game root")
	}
	cmd := newGameCmd()
	cmd.SetArgs([]string{"watch", "--tools-path", game})
	c, stop := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer stop()
	if err := cmd.ExecuteContext(c); err != nil {
		t.Errorf("the command stops with its context: %v", err)
	}
}

// safeBuffer is a strings.Builder the watcher writes from another goroutine.
type safeBuffer struct {
	mu sync.Mutex
	b  strings.Builder
}

func (s *safeBuffer) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}

func (s *safeBuffer) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.String()
}
