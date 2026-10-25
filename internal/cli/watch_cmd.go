package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"time"

	"github.com/spf13/cobra"

	"github.com/McReaper/t7_companion/internal/build"
	"github.com/McReaper/t7_companion/internal/gamelog"
	"github.com/McReaper/t7_companion/internal/level"
)

// watchEvery is how often the watch looks at what it follows; a variable so
// tests can wait less.
var watchEvery = time.Second

// watchSettle is how long a build's records stay as they are before the watch
// reads them, and watchQuiet how long Radiant's console stays quiet before a
// burst of its errors is said; variables so tests can wait less.
var (
	watchSettle = 3 * time.Second
	watchQuiet  = 2 * time.Second
)

// toolRuns says whether a mod tool's process runs, and buildStates are the
// builds t7kb ran on an install; variables so tests can stand in.
var (
	toolRuns    = build.Runs
	buildStates = build.States
)

// source is one thing the watch follows: each tick reads what it gained and
// says what is worth saying.
type source interface{ Tick() }

func newWatchCmd() *cobra.Command {
	var toolsPath string
	c := &cobra.Command{Use: "watch", Short: "Say each problem the game has, each build's verdict and what Radiant reports as it happens, one line each, until stopped",
		Args: cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			ctx, stop := signal.NotifyContext(c.Context(), os.Interrupt)
			defer stop()
			return watch(ctx, toolsPath, c.OutOrStdout(), gameProcs(), toolRuns)
		}}
	c.Flags().StringVar(&toolsPath, "tools-path", "", "BO3 mod-tools root (default: $TA_TOOLS_PATH; $TA_GAME_PATH wins for the game root)")
	return c
}

// watch says to out, as they happen, the game's problems, the verdict of each
// build the mod tools run and what each Radiant whose bridge runs reports,
// until ctx ends.
func watch(ctx context.Context, toolsPath string, out io.Writer, g gamelog.Game, runs func(string) bool) error {
	root, err := gameRoot(toolsPath)
	if err != nil {
		return err
	}
	say := func(s string) { fmt.Fprintln(out, s) }
	sources := []source{&gamelog.Watcher{Root: root, Game: g, Out: say}, &level.Watcher{Machine: radiantMachine, Out: say, Quiet: watchQuiet}}
	if tools := toolsRoot(toolsPath); tools != "" {
		sources = append(sources, &build.RecordWatcher{Root: tools, Runs: runs, Out: say, Settle: watchSettle,
			Builds: func() []build.State { return buildStates(tools) }})
	}
	t := time.NewTicker(watchEvery)
	defer t.Stop()
	for {
		for _, s := range sources {
			s.Tick()
		}
		select {
		case <-ctx.Done():
			return nil
		case <-t.C:
		}
	}
}
