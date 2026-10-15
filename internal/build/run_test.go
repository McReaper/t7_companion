package build

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

// The stage runners start real processes. This test binary stands in for each
// mod tool and for the game: started with T7KB_FAKE_TOOL set, it does what the
// mode says instead of running the tests, and records its working directory,
// arguments and Steam variables in T7KB_FAKE_OUT.
func TestMain(m *testing.M) {
	mode := os.Getenv("T7KB_FAKE_TOOL")
	if mode == "" {
		os.Exit(m.Run())
	}
	if out := os.Getenv("T7KB_FAKE_OUT"); out != "" {
		wd, _ := os.Getwd()
		rec := fmt.Sprintf("cwd=%s\nargs=%s\nSteamAppId=%s\nSteamGameId=%s\n",
			wd, strings.Join(os.Args[1:], " "), os.Getenv("SteamAppId"), os.Getenv("SteamGameId"))
		_ = os.WriteFile(out, []byte(rec), 0o644)
	}
	switch mode {
	case "ok": // a tool that logs a non-fatal error and still exits 0
		fmt.Println("compiling…\n^1ERROR: a lightmap is missing")
	case "fail":
		fmt.Println("noise\r\nSCRIPT ERROR: bad token\nmore noise")
		os.Exit(3)
	case "silent-fail":
		fmt.Println("something the patterns don't know")
		os.Exit(4)
	case "hang", "stay": // a tool past its timeout; a game that started
		time.Sleep(3 * time.Second)
	case "led": // radiant's light bake: it writes the .led, later
		time.Sleep(200 * time.Millisecond)
		_ = os.WriteFile(os.Getenv("T7KB_FAKE_LED"), []byte("led"), 0o644)
	case "quit": // a game Steam refused
		os.Exit(1)
	case "late": // a game that crashed, or someone quit, while it loaded
		time.Sleep(600 * time.Millisecond)
		os.Exit(2)
	}
	os.Exit(0)
}

// fakeTool copies this test binary to dir/name, so it runs as that tool.
func fakeTool(t *testing.T, dir, name string) string {
	t.Helper()
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(self)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	exe := filepath.Join(dir, name)
	if err := os.WriteFile(exe, b, 0o755); err != nil {
		t.Fatal(err)
	}
	return exe
}

// record is what the fake tool wrote to T7KB_FAKE_OUT.
func record(t *testing.T, path string) map[string]string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("the fake tool left no record: %v", err)
	}
	rec := map[string]string{}
	for _, line := range strings.Split(strings.TrimSpace(string(b)), "\n") {
		k, v, _ := strings.Cut(line, "=")
		rec[k] = v
	}
	return rec
}

func sameDir(t *testing.T, a, b string) bool {
	t.Helper()
	ia, err1 := os.Stat(a)
	ib, err2 := os.Stat(b)
	return err1 == nil && err2 == nil && os.SameFile(ia, ib)
}

func TestRunStage(t *testing.T) {
	bin := t.TempDir()
	exe := fakeTool(t, bin, "linker_modtools.exe")
	out := filepath.Join(t.TempDir(), "rec")
	t.Setenv("T7KB_FAKE_OUT", out)

	t.Run("exit 0 keeps a logged error as a note, in the tool's own directory", func(t *testing.T) {
		t.Setenv("T7KB_FAKE_TOOL", "ok")
		var echo bytes.Buffer
		sr := runStage("link", bin, exe, time.Minute, true, &echo, "-language", "english", "/update")
		if !sr.OK || sr.Name != "link" || sr.Note != "ERROR: a lightmap is missing" || sr.Errors != nil {
			t.Errorf("%+v", sr)
		}
		rec := record(t, out)
		if rec["args"] != "-language english /update" {
			t.Errorf("args %q: exec passes them as they are, no shell to mangle /update", rec["args"])
		}
		if !sameDir(t, rec["cwd"], bin) {
			t.Errorf("cwd %q, want the tool's directory %q", rec["cwd"], bin)
		}
		if !strings.HasPrefix(echo.String(), "» linker_modtools.exe -language english /update\n") || !strings.Contains(echo.String(), "compiling…") {
			t.Errorf("verbose output:\n%s", echo.String())
		}
	})

	t.Run("quiet unless verbose", func(t *testing.T) {
		t.Setenv("T7KB_FAKE_TOOL", "ok")
		var echo bytes.Buffer
		runStage("link", bin, exe, time.Minute, false, &echo)
		if echo.Len() != 0 {
			t.Errorf("printed %q", echo.String())
		}
	})

	t.Run("a failure returns its error lines", func(t *testing.T) {
		t.Setenv("T7KB_FAKE_TOOL", "fail")
		sr := runStage("link", bin, exe, time.Minute, false, io.Discard)
		if sr.OK || len(sr.Errors) != 1 || sr.Errors[0] != "SCRIPT ERROR: bad token" {
			t.Errorf("%+v", sr)
		}
	})

	t.Run("an unknown failure returns the whole output", func(t *testing.T) {
		t.Setenv("T7KB_FAKE_TOOL", "silent-fail")
		sr := runStage("link", bin, exe, time.Minute, false, io.Discard)
		if sr.OK || len(sr.Errors) != 3 || sr.Errors[0] != "something the patterns don't know" ||
			!strings.Contains(sr.Errors[1], "4") || sr.Errors[2] != unknownFailure {
			t.Errorf("%+v", sr)
		}
	})

	t.Run("a timeout", func(t *testing.T) {
		t.Setenv("T7KB_FAKE_TOOL", "hang")
		sr := runStage("link", bin, exe, 300*time.Millisecond, false, io.Discard)
		if sr.OK || len(sr.Errors) != 1 || sr.Errors[0] != "timed out after 300ms" {
			t.Errorf("%+v", sr)
		}
	})

	t.Run("a tool that isn't there", func(t *testing.T) {
		sr := runStage("link", bin, filepath.Join(bin, "missing.exe"), time.Minute, false, io.Discard)
		if sr.OK || len(sr.Errors) != 2 || sr.Errors[1] != unknownFailure {
			t.Errorf("%+v", sr)
		}
	})
}

func TestRunLight(t *testing.T) {
	oldTimeout, oldPoll := lightTimeout, lightPoll
	t.Cleanup(func() { lightTimeout, lightPoll = oldTimeout, oldPoll })
	lightTimeout, lightPoll = 2*time.Second, 50*time.Millisecond

	bin := t.TempDir()
	fakeTool(t, bin, "radiant_modtools.exe")
	led := filepath.Join(t.TempDir(), "zm_test.led")
	out := filepath.Join(t.TempDir(), "rec")
	t.Setenv("T7KB_FAKE_OUT", out)
	t.Setenv("T7KB_FAKE_LED", led)

	t.Run("waits for the .led the detached bake writes", func(t *testing.T) {
		t.Setenv("T7KB_FAKE_TOOL", "led")
		sr := runLight(bin, "C:/bo3/map_source/zm/zm_test.map", led, "high")
		if !sr.OK || sr.Name != "light" || sr.Note != "wrote zm_test.led" {
			t.Errorf("%+v", sr)
		}
		rec := record(t, out)
		if rec["args"] != "-ledSilent +high +localprobes +forceclean +recompute C:/bo3/map_source/zm/zm_test.map" {
			t.Errorf("args %q", rec["args"])
		}
		if !sameDir(t, rec["cwd"], bin) {
			t.Errorf("cwd %q, want %q", rec["cwd"], bin)
		}
	})

	t.Run("an old .led doesn't count", func(t *testing.T) {
		t.Setenv("T7KB_FAKE_TOOL", "quit") // exits without baking
		if err := os.WriteFile(led, []byte("old"), 0o644); err != nil {
			t.Fatal(err)
		}
		sr := runLight(bin, "zm_test.map", led, "medium")
		if sr.OK || len(sr.Errors) != 1 || !strings.HasPrefix(sr.Errors[0], "no zm_test.led after 2s") {
			t.Errorf("%+v", sr)
		}
	})

	t.Run("radiant isn't there", func(t *testing.T) {
		sr := runLight(t.TempDir(), "zm_test.map", led, "medium")
		if sr.OK || len(sr.Errors) != 1 || !strings.HasPrefix(sr.Errors[0], "could not launch radiant_modtools: ") {
			t.Errorf("%+v", sr)
		}
	})
}

func TestRunGame(t *testing.T) {
	oldStartup, oldWatch, oldPoll, oldWindows := gameStartup, gameWatch, gamePoll, gameWindows
	t.Cleanup(func() { gameStartup, gameWatch, gamePoll, gameWindows = oldStartup, oldWatch, oldPoll, oldWindows })
	gameStartup, gameWatch, gamePoll = 300*time.Millisecond, time.Second, 20*time.Millisecond
	var asked []int
	gameWindows = func(pid int) []window { asked = append(asked, pid); return nil }

	game := t.TempDir()
	fakeTool(t, game, "BlackOps3.exe")
	out := filepath.Join(t.TempDir(), "rec")
	t.Setenv("T7KB_FAKE_OUT", out)
	t.Setenv("SteamAppId", "1") // Steam set something else: the game's own id wins

	t.Run("a game still up and showing no dialog after the watch launched", func(t *testing.T) {
		t.Setenv("T7KB_FAKE_TOOL", "stay")
		sr := runGame(game, []string{"+set", "fs_game", "zm_test", "+devmap", "zm_test"})
		if !sr.OK || sr.Name != "run" || sr.Note != "launched BlackOps3.exe: no dialog in its first 1s" || sr.Errors != nil || sr.Seconds < 1 || sr.Seconds > 2 {
			t.Errorf("%+v", sr)
		}
		if len(asked) < 10 || asked[0] == os.Getpid() || asked[0] != asked[len(asked)-1] {
			t.Errorf("looked for the game's windows %d times, in %v: every %s, in the game's process", len(asked), asked, gamePoll)
		}
		rec := record(t, out)
		if rec["args"] != "+set fs_game zm_test +devmap zm_test" || rec["SteamAppId"] != bo3AppID || rec["SteamGameId"] != bo3AppID {
			t.Errorf("%v", rec)
		}
		if !sameDir(t, rec["cwd"], game) {
			t.Errorf("cwd %q, want the game's directory %q", rec["cwd"], game)
		}
		time.Sleep(3 * time.Second) // the fake game exits, so its directory can be removed
	})

	t.Run("a dialog fails the stage at once, and says what it says", func(t *testing.T) {
		t.Setenv("T7KB_FAKE_TOOL", "stay")
		polls := 0
		gameWindows = func(int) []window {
			if polls++; polls < 3 {
				return []window{{Class: "CoDBlackOps", Title: "Call of Duty"}}
			}
			return []window{{Class: dialogClass, Title: "Erreur fatale", Controls: []control{{"Button", "OK"}, {"Static", "ERROR: Could not find zone 'fr_zm_test'\n"}}}}
		}
		sr := runGame(game, nil)
		want := []string{`the game opened a dialog, "Erreur fatale": ERROR: Could not find zone 'fr_zm_test' [OK]`}
		if sr.OK || sr.Name != "run" || !slices.Equal(sr.Errors, want) || polls != 3 || sr.Seconds > 0.5 {
			t.Errorf("%+v after %d looks", sr, polls)
		}
		gameWindows = func(int) []window { return nil }
		time.Sleep(3 * time.Second) // the fake game exits, so its directory can be removed
	})

	t.Run("a game that closes while it loads failed", func(t *testing.T) {
		t.Setenv("T7KB_FAKE_TOOL", "late")
		sr := runGame(game, nil)
		if sr.OK || len(sr.Errors) != 1 || sr.Errors[0] != "the game closed 1s after starting (exit status 2)" {
			t.Errorf("%+v", sr)
		}
	})

	t.Run("a game that exits at once failed", func(t *testing.T) {
		t.Setenv("T7KB_FAKE_TOOL", "quit")
		sr := runGame(game, nil)
		if sr.OK || len(sr.Errors) != 1 || !strings.HasPrefix(sr.Errors[0], "the game exited right after starting (exit status 1)") ||
			!strings.Contains(sr.Errors[0], "is Steam running and signed in?") {
			t.Errorf("%+v", sr)
		}
	})

	t.Run("no game there", func(t *testing.T) {
		sr := runGame(t.TempDir(), nil)
		if sr.OK || len(sr.Errors) != 1 || !strings.HasPrefix(sr.Errors[0], "could not launch game: ") {
			t.Errorf("%+v", sr)
		}
	})
}
