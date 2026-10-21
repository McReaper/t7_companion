package build

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func writeFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestWithGameLogUnlessTheRunSetsLogfile(t *testing.T) {
	asked := []dvar{{"developer", "2"}}
	got := withGameLog(asked)
	if strings.Join(dvarArgs(got), " ") != "+set developer 2 +set logfile 2" || len(asked) != 1 {
		t.Errorf("logfile 2 goes last, the asked dvars untouched: %v", got)
	}
	off := []dvar{{"LogFile", "0"}}
	if got := withGameLog(off); len(got) != 1 || got[0].value != "0" {
		t.Errorf("the run's own logfile stays: %v", got)
	}
}

func TestNoteRunLogNamesTheLogTheRunWrote(t *testing.T) {
	game := t.TempDir()
	start := time.Now()
	sr := StageResult{OK: true, Note: "launched"}
	noteRunLog(&sr, game, "zm_test", start)
	if sr.Note != "launched; the game wrote no console_mp.log (the run set logfile 0?)" {
		t.Errorf("a run that wrote no log says so: %q", sr.Note)
	}
	sr = StageResult{Errors: []string{"the game exited right after starting"}}
	noteRunLog(&sr, game, "zm_test", start)
	if sr.Note != "" {
		t.Errorf("a game that never got far writes no log, which says nothing more: %q", sr.Note)
	}
	log := filepath.Join(game, "console_mp.log")
	if err := os.WriteFile(log, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	sr = StageResult{Errors: []string{"the game opened a dialog"}}
	noteRunLog(&sr, game, "zm_test", start)
	if sr.Note != "log: "+log+" (game_log reads it)" {
		t.Errorf("the log the run wrote, failed or not: %q", sr.Note)
	}
}
