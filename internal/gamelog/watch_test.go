package gamelog

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// fakeGame is the game's processes for a watcher.
type fakeGame struct {
	running []int
	dialogs map[int][]string
	exits   map[int]uint32
}

func (g *fakeGame) Running() []int           { return g.running }
func (g *fakeGame) Dialogs(pid int) []string { return g.dialogs[pid] }
func (g *fakeGame) Exit(pid int) (uint32, bool) {
	code, ok := g.exits[pid]
	return code, ok
}

// watching is a watcher over a game root, and what it said.
type watching struct {
	t    *testing.T
	root string
	game *fakeGame
	w    *Watcher
	said []string
}

func watch(t *testing.T) *watching {
	t.Helper()
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "usermaps", "zm_test", "zone_source", "zm_test.zone"), testZone)
	g := &fakeGame{dialogs: map[int][]string{}, exits: map[int]uint32{}}
	wt := &watching{t: t, root: root, game: g}
	wt.w = &Watcher{Root: root, Game: g, Out: func(s string) { wt.said = append(wt.said, s) }}
	return wt
}

// tick ticks and returns what the watcher said during it.
func (wt *watching) tick() []string {
	before := len(wt.said)
	wt.w.Tick()
	if len(wt.said) == before {
		return nil
	}
	return wt.said[before:]
}

// write writes a log afresh, its time after the watch began.
func (wt *watching) write(rel, body string) {
	wt.t.Helper()
	p := filepath.Join(wt.root, rel)
	writeFile(wt.t, p, body)
	setTime(wt.t, p, time.Now().Add(time.Second))
}

func (wt *watching) appendTo(rel, body string) {
	wt.t.Helper()
	f, err := os.OpenFile(filepath.Join(wt.root, rel), os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		wt.t.Fatal(err)
	}
	defer f.Close()
	if _, err := f.WriteString(body); err != nil {
		wt.t.Fatal(err)
	}
}

const sre = "******* script error *******\nError: cannot cast undefined to bool\n******* Call stack *******\n" +
	"Error: \tfile 'scripts/zm/zm_test.gsc', line 77 :: \nError: ************************************\n"

func TestWatchSaysWhatALogGainsFromThenOn(t *testing.T) {
	wt := watch(t)
	old := filepath.Join(wt.root, LogName)
	writeFile(t, old, "logfile opened on Mon Oct 19 22:10:03 2026\n"+sre)
	setTime(t, old, time.Now().Add(-time.Hour))
	if said := wt.tick(); said != nil {
		t.Fatalf("a log already there is followed from its end: %q", said)
	}
	wt.appendTo(LogName, "Error: Could not find fx \"fx/stock\".\n"+sre+sre)
	want := "script error: cannot cast undefined to bool | file 'scripts/zm/zm_test.gsc', line 77 ::, console_mp.log line 8"
	if said := wt.tick(); len(said) != 1 || said[0] != want {
		t.Errorf("a script error once, at the file's line, stock noise silent:\n%q\nwant %q", said, want)
	}
	wt.appendTo(LogName, "ERROR: half a line")
	if said := wt.tick(); said != nil {
		t.Errorf("a line not ended yet waits: %q", said)
	}

	// a new launch rewrites the log: read from its start, its errors new again
	wt.write(LogName, "Build 0\nlogfile opened on Tue Oct 20 01:52:15 2026\n"+sre)
	if said := wt.tick(); len(said) != 1 || !strings.HasSuffix(said[0], "console_mp.log line 3") {
		t.Errorf("the next launch's script error: %q", said)
	}
}

func TestWatchSaysTheMapsOwnProblemsOnceItLoaded(t *testing.T) {
	wt := watch(t)
	wt.tick()
	wt.write(LogName, "logfile opened on Tue Oct 20 01:52:15 2026\n------- Game Initialization -------\n"+
		"Error: Could not find xmodel \"P7_My_Crate\".\nError: Could not find fx \"fx/stock\".\nModLoad done: zm_test\n")
	want := "zm_test loaded (console_mp.log line 5), but the game didn't find 1 asset(s) its zone lists: P7_My_Crate; game_log zm_test has the lines"
	if said := wt.tick(); len(said) != 1 || said[0] != want {
		t.Errorf("load:\n%q\nwant %q", said, want)
	}
	// the user reloads it with /devmap: the same launch, a new load
	wt.appendTo(LogName, "------- Game Initialization -------\nERROR: the sun is missing\nModLoad done: zm_test\n")
	want = "zm_test loaded (console_mp.log line 8) after 1 Error line(s) of no known kind; game_log zm_test has them"
	if said := wt.tick(); len(said) != 1 || said[0] != want {
		t.Errorf("a /devmap reload, its own load:\n%q\nwant %q", said, want)
	}
	wt.appendTo(LogName, "------- Game Initialization -------\nModLoad done: zm_test\n")
	if said := wt.tick(); said != nil {
		t.Errorf("a clean load says nothing: %q", said)
	}
}

func TestWatchSaysAFatalErrorOnceThoughItsDialogRepeatsIt(t *testing.T) {
	wt := watch(t)
	wt.game.running = []int{7}
	wt.tick()
	wt.write(LogName, "logfile opened on Tue Oct 20 22:36:40 2026\nCom_ERROR: ERROR: Could not find zone 'fr_zm_test'\n")
	wt.game.dialogs[7] = []string{`the game opened a dialog, "Erreur fatale": ERROR: Could not find zone 'fr_zm_test' [OK]`}
	if said := wt.tick(); len(said) != 1 || said[0] != "fatal error, console_mp.log line 2: ERROR: Could not find zone 'fr_zm_test'" {
		t.Errorf("the log's line, not its dialog's too: %q", said)
	}
	wt.game.dialogs[7] = append(wt.game.dialogs[7], `the game opened a dialog, "Mode Sans échec": Votre jeu ne s'est pas éteint correctement. [Oui] [Non]`)
	if said := wt.tick(); len(said) != 1 || !strings.HasPrefix(said[0], `BlackOps3.exe (pid 7): the game opened a dialog, "Mode Sans échec"`) {
		t.Errorf("a dialog the log doesn't say: %q", said)
	}
	if said := wt.tick(); said != nil {
		t.Errorf("a dialog once: %q", said)
	}
}

func TestWatchSaysAGameThatEndedWithAnErrorCode(t *testing.T) {
	wt := watch(t)
	wt.game.running = []int{7, 8}
	wt.tick()
	wt.game.running = nil
	if said := wt.tick(); said != nil {
		t.Errorf("an ending not read yet says nothing: %q", said)
	}
	wt.game.exits = map[int]uint32{7: 0xC0000005, 8: 0}
	if said := wt.tick(); len(said) != 1 || said[0] != "BlackOps3.exe (pid 7) ended with exit code 0xC0000005: the game's crashes.log may say why" {
		t.Errorf("a crash, not a clean quit: %q", said)
	}
	if said := wt.tick(); said != nil {
		t.Errorf("once: %q", said)
	}
}

func TestWatchFollowsAModsLogAndStopsAtItsCap(t *testing.T) {
	wt := watch(t)
	wt.tick()
	var b strings.Builder
	b.WriteString("logfile opened on Tue Oct 20 01:52:15 2026\n")
	for i := range maxEvents + 5 {
		fmt.Fprintf(&b, "******* script error *******\nError: problem %d\nError: ****************\n", i)
	}
	wt.write("mods/my_mod/console_mp.log", b.String())
	said := wt.tick()
	if len(said) != maxEvents || !strings.HasSuffix(said[0], "mods/my_mod/console_mp.log line 2") ||
		said[maxEvents-1] != "more problems in mods/my_mod/console_mp.log since this launch of the game: game_log has them" {
		t.Errorf("%d events then one pointer to game_log: %d, first %q, last %q", maxEvents, len(said), said[0], said[len(said)-1])
	}
}

func TestSameStart(t *testing.T) {
	if !sameStart([]byte("abc"), []byte("abcdef")) || sameStart([]byte("abd"), []byte("abcdef")) || !sameStart(nil, []byte("x")) {
		t.Error("compared as far as both go")
	}
}

// A new launch's log can outgrow the old one between two ticks: its header
// tells it apart, and its blank lines don't stall the read.
func TestWatchTellsALongerNewLaunchByItsHeader(t *testing.T) {
	wt := watch(t)
	wt.tick()
	wt.write(LogName, "logfile opened on Mon Oct 19 22:10:03 2026\n\nERROR: one\n")
	wt.tick()
	wt.write(LogName, "logfile opened on Tue Oct 20 01:52:15 2026\n\n\n"+sre+"filler line that makes this launch longer than the last\n")
	if said := wt.tick(); len(said) != 1 || !strings.HasSuffix(said[0], "console_mp.log line 4") {
		t.Errorf("the longer new launch, read from its start past its blank lines: %q", said)
	}
}
