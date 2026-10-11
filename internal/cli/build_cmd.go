package cli

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/McReaper/t7_companion/internal/build"
)

// buildRun is build.Run; tests swap it to see what each surface asks for.
var buildRun = build.Run

func newBuildCmd() *cobra.Command {
	o := &build.Options{}
	var jsonOut bool
	cmd := &cobra.Command{
		Use:   "build <name>",
		Short: "Headlessly compile/light/link a BO3 map or mod (drives the mod tools)",
		Long: "Runs the Black Ops 3 mod-tools build pipeline for a map (or --mod) without the\n" +
			"GUI Launcher, and prints a compact per-stage result. It calls the same binaries\n" +
			"the Launcher does (cod2map64, radiant_modtools, linker_modtools, gdtdb).\n\n" +
			"Requires a Windows BO3 mod-tools install; the path comes from --tools-path or\n" +
			"$TA_TOOLS_PATH. Example: t7kb build zm_mymap --stages compile,light,link",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runBuild(cmd, o, args[0], jsonOut)
		},
	}
	f := cmd.Flags()
	f.StringVar(&o.ToolsPath, "tools-path", "", "BO3 mod-tools root (default: $TA_TOOLS_PATH)")
	f.StringVar(&o.GamePath, "game-path", "", "BO3 game root (default: $TA_GAME_PATH, then --tools-path)")
	f.BoolVar(&o.Mod, "mod", false, "target is a mod (mods/<name>), not a usermap")
	f.StringVar(&o.Stages, "stages", "compile,light,link", "comma list of stages: compile,light,link,run")
	f.BoolVar(&o.OnlyEnts, "onlyents", false, "fast entity-only compile (-onlyents; invalid after brush edits)")
	f.StringVar(&o.Light, "light", "medium", "light quality: low|medium|high")
	f.StringVar(&o.Language, "language", "english", "linker language")
	f.BoolVar(&o.SkipGDT, "skip-gdt", false, "skip the gdtdb /update pass before building")
	f.BoolVar(&o.GDTRebuild, "gdt-rebuild", false, "run gdtdb /rebuild instead of /update (only if /update reports 0 GDTs and the linker then misses an edited asset)")
	f.BoolVar(&o.FreshXpak, "fresh-xpak", false, "link: write the .xpak from scratch, dropping the data earlier links replaced (it only grows otherwise, and uploads with zone/); use before publishing")
	f.BoolVar(&jsonOut, "json", false, "emit the report as JSON")
	f.BoolVar(&o.Verbose, "verbose", false, "stream each tool's full output as it runs")
	f.StringArrayVar(&o.Dvars, "dvar", nil, "run: start the game with this dvar, name=value (repeatable; e.g. developer=2, logfile=2)")
	f.BoolVar(&o.LauncherDvars, "launcher-dvars", false, "run: start the game with the dvars saved in the mod tools Launcher's Dvars dialog (--dvar overrides them)")
	return cmd
}

func runBuild(cmd *cobra.Command, o *build.Options, name string, jsonOut bool) error {
	stdout := cmd.OutOrStdout()
	rep, err := buildRun(o, name, stdout)
	if err != nil {
		return err
	}
	return finishBuild(stdout, jsonOut, rep)
}

func finishBuild(out io.Writer, asJSON bool, rep build.Report) error {
	if asJSON {
		enc := json.NewEncoder(out)
		enc.SetIndent("", "  ")
		_ = enc.Encode(rep)
	} else {
		printBuildReport(out, rep)
	}
	if !rep.OK {
		return fmt.Errorf("build failed at %s", rep.FailedStage)
	}
	return nil
}

func printBuildReport(out io.Writer, rep build.Report) {
	fmt.Fprintf(out, "build %s (%s)\n", rep.Target, rep.Kind)
	for _, s := range rep.Stages {
		status := "ok  "
		if !s.OK {
			status = "FAIL"
		}
		line := fmt.Sprintf("  %-8s %s  %5.1fs", s.Name, status, s.Seconds)
		if s.Note != "" {
			line += "  " + s.Note
		}
		fmt.Fprintln(out, line)
		for _, e := range s.Errors {
			fmt.Fprintf(out, "      %s\n", e)
		}
		for _, w := range s.Warnings {
			fmt.Fprintf(out, "      warning: %s\n", w)
		}
		if len(s.Missing) > 0 {
			fmt.Fprintln(out, "      linked without (looked up, found in no GDT):")
			for _, m := range s.Missing {
				fmt.Fprintf(out, "        %s\n", m)
			}
		}
	}
	if rep.OK {
		fmt.Fprintln(out, "build OK")
	} else {
		fmt.Fprintf(out, "build FAILED at %s\n", rep.FailedStage)
	}
}
