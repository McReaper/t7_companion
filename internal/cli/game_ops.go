package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/McReaper/t7_companion/internal/build"
	"github.com/McReaper/t7_companion/internal/gamelog"
)

// Answers are capped: a load prints thousands of lines, and this lands in an
// agent's context; every line carries its number for reading around it.
const (
	gameMaxScript   = 20
	gameMaxProblems = 30
	gameMaxMatches  = 200
	gameMaxContext  = 10
	gameMaxTail     = 500
)

// gameLogArgs is what game_log is asked: the map or mod whose log to sort,
// or a log by path, and the raw lines wanted.
type gameLogArgs struct {
	ToolsPath, Name, Path, Grep string
	Context, Max, Tail, After   int
}

type gameFileOut struct {
	Path     string `json:"path"`
	Modified string `json:"modified"`
}

// gameLogOut is game_log's answer: the log's last launch sorted, the lines asked
// for, and where to read the rest.
type gameLogOut struct {
	gameFileOut
	Opened       string          `json:"opened,omitempty"`
	Lines        string          `json:"lines"`
	MapLoads     []int           `json:"map_loads,omitempty"`
	Loaded       int             `json:"loaded,omitempty"`
	ScriptErrors []gamelog.Line  `json:"script_errors,omitempty"`
	MapProblems  []gamelog.Line  `json:"map_problems,omitempty"`
	Unlisted     string          `json:"unlisted,omitempty"`
	StockLookups string          `json:"stock_lookups_failed,omitempty"`
	Matches      []gamelog.Match `json:"matches,omitempty"`
	MoreMatches  int             `json:"more_matches,omitempty"`
	Tail         []gamelog.Line  `json:"tail,omitempty"`
	Dialogs      []string        `json:"dialogs,omitempty"`
	Others       []gameFileOut   `json:"other_logs,omitempty"`
}

// gameProcs are the running game's processes, for the dialogs they show; a
// variable so tests can stand in.
var gameProcs = func() gamelog.Game { return build.NewGames() }

// gameRoot is the game's root: $TA_GAME_PATH, else the mod-tools root, as the
// build's run stage takes it.
func gameRoot(toolsPath string) (string, error) {
	if g := strings.TrimRight(os.Getenv("TA_GAME_PATH"), `\/`); g != "" {
		return g, nil
	}
	if r := toolsRoot(toolsPath); r != "" {
		return r, nil
	}
	return "", fmt.Errorf("no game path: pass tools_path / --tools-path or set TA_TOOLS_PATH (or TA_GAME_PATH)")
}

// gameLog reads the current console_mp.log, or the one at a.Path.
func gameLog(a gameLogArgs) (*gameLogOut, error) {
	game, err := gameRoot(a.ToolsPath)
	if err != nil && a.Path == "" {
		return nil, err
	}
	cur, others, err := gameLogFile(game, a)
	if err != nil {
		return nil, err
	}
	l, err := gamelog.Read(cur.Path)
	if err != nil {
		return nil, err
	}
	l.Lines = slices.DeleteFunc(l.Lines, func(ln gamelog.Line) bool { return ln.N <= a.After })
	out := &gameLogOut{gameFileOut: fileOut(cur), Opened: l.Opened, Lines: launchLines(l)}
	if a.Path == "" {
		g := gameProcs()
		for _, pid := range g.Running() {
			out.Dialogs = append(out.Dialogs, g.Dialogs(pid)...)
		}
	}
	for _, o := range others {
		out.Others = append(out.Others, fileOut(o))
	}
	isMod := a.Name != "" && dirExists(filepath.Join(game, "mods", a.Name)) && !dirExists(filepath.Join(game, "usermaps", a.Name))
	sortName := a.Name
	if isMod {
		sortName = "" // a mod's run loads no map of its name
	}
	s := gamelog.Sort(l, sortName, gamelog.Zoned(game, a.Name, isMod))
	out.MapLoads, out.Loaded, out.StockLookups = s.MapLoads, s.Loaded, stockLookups(s)
	out.ScriptErrors, out.MapProblems = capLines(s.Script, gameMaxScript), capLines(s.Own, gameMaxProblems)
	if n := len(s.Script) - len(out.ScriptErrors) + len(s.Own) - len(out.MapProblems); n > 0 {
		out.Unlisted = fmt.Sprintf("%d more script errors and problems past the cap: grep for them", n)
	}
	return out, gameLogRaw(out, l, a)
}

// gameLogFile is the log to read: the one at a.Path, else the current one.
func gameLogFile(game string, a gameLogArgs) (gamelog.File, []gamelog.File, error) {
	if a.Path != "" {
		fi, err := os.Stat(a.Path)
		if err != nil {
			return gamelog.File{}, nil, err
		}
		return gamelog.File{Path: a.Path, Modified: fi.ModTime()}, nil, nil
	}
	cur, others, ok := gamelog.Find(game, a.Name)
	if !ok {
		return gamelog.File{}, nil, fmt.Errorf("no %s in %s or its mods/*/: the game writes one only with the logfile dvar "+
			"(t7kb:build's run sets it; by hand, +set logfile 2)", gamelog.LogName, game)
	}
	return cur, others, nil
}

// gameLogRaw adds the raw lines asked for.
func gameLogRaw(out *gameLogOut, l gamelog.Launch, a gameLogArgs) error {
	if a.Grep != "" {
		re, err := regexp.Compile("(?i)" + a.Grep)
		if err != nil {
			return fmt.Errorf("grep: %w", err)
		}
		out.Matches, out.MoreMatches = gamelog.Grep(l, re, clamp(a.Context, 2, gameMaxContext), clamp(a.Max, 50, gameMaxMatches))
	}
	if a.Tail > 0 {
		out.Tail = gamelog.Tail(l, min(a.Tail, gameMaxTail))
	}
	return nil
}

// clamp is v within (0, hi], def when v is 0 or less.
func clamp(v, def, hi int) int {
	if v <= 0 {
		return def
	}
	return min(v, hi)
}

func capLines(ls []gamelog.Line, n int) []gamelog.Line {
	return ls[:min(len(ls), n)]
}

func fileOut(f gamelog.File) gameFileOut {
	return gameFileOut{f.Path, f.Modified.Format(time.DateTime)}
}

// launchLines is the line range read of the last launch.
func launchLines(l gamelog.Launch) string {
	if len(l.Lines) == 0 {
		return fmt.Sprintf("none to read, of %d", l.Total)
	}
	return fmt.Sprintf("%d-%d of %d (the last launch)", l.Lines[0].N, l.Lines[len(l.Lines)-1].N, l.Total)
}

func stockLookups(s gamelog.Summary) string {
	if s.NoiseAll == 0 {
		return ""
	}
	return fmt.Sprintf("%d (%s): stock assets the map doesn't zone, which every load fails to find", s.NoiseAll, gamelog.Kinds(s.Noise))
}

func dirExists(p string) bool {
	fi, err := os.Stat(p)
	return err == nil && fi.IsDir()
}
