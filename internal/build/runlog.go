package build

import (
	"strings"
	"time"

	"github.com/McReaper/t7_companion/internal/gamelog"
)

// withGameLog adds logfile, so the game writes console_mp.log for game_log to
// read, unless the run sets it.
func withGameLog(dvars []dvar) []dvar {
	for _, d := range dvars {
		if strings.EqualFold(d.name, "logfile") {
			return dvars
		}
	}
	return append(append([]dvar(nil), dvars...), dvar{"logfile", gamelog.LogDvar})
}

// noteRunLog names in the stage's note the console_mp.log the run wrote, or
// says it wrote none.
func noteRunLog(sr *StageResult, game, name string, start time.Time) {
	switch path := gamelog.Written(game, name, start); {
	case path != "":
		addNote(sr, "log: "+path+" (game_log reads it)")
	case sr.OK:
		addNote(sr, "the game wrote no console_mp.log (the run set logfile 0?)")
	}
}

// addNote adds a clause to the stage's note.
func addNote(sr *StageResult, s string) {
	if sr.Note != "" {
		s = sr.Note + "; " + s
	}
	sr.Note = s
}
