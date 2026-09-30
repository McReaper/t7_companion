package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

// build drives the Black Ops 3 mod-tools compile/light/link pipeline headlessly,
// so an agent can build a map or mod without the GUI Launcher and get back a
// compact per-stage summary (plus the first actionable error) instead of the
// hundreds of lines each tool spews. It shells the same binaries the Launcher
// does — cod2map64, radiant_modtools, linker_modtools, gdtdb — with the non-obvious
// things they need baked in: each tool runs with cwd = its own exe directory like the
// Launcher (cod2map needs cwd=bin to load default_navmesh_settings.json; gdtdb ties its
// recorded asset paths to cwd, so a different cwd makes it flag every asset as a phantom
// duplicate), args are passed via exec (no shell, so no MSYS/`/update` mangling), and the
// detached light step is waited on by polling for its .led output rather than a synchronous
// exit code.

type buildOpts struct {
	toolsPath  string
	gamePath   string
	isMod      bool
	stages     string
	onlyEnts   bool
	light      string
	language   string
	skipGDT    bool
	gdtRebuild bool
	jsonOut    bool
	verbose    bool
}

type stageResult struct {
	Name    string   `json:"name"`
	OK      bool     `json:"ok"`
	Seconds float64  `json:"seconds"`
	Errors  []string `json:"errors,omitempty"`
	Note    string   `json:"note,omitempty"`
}

type buildReport struct {
	Target      string        `json:"target"`
	Kind        string        `json:"kind"`
	OK          bool          `json:"ok"`
	FailedStage string        `json:"failed_stage,omitempty"`
	Stages      []stageResult `json:"stages"`
}

func newBuildCmd() *cobra.Command {
	o := &buildOpts{}
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
			return runBuild(cmd, o, args[0])
		},
	}
	f := cmd.Flags()
	f.StringVar(&o.toolsPath, "tools-path", "", "BO3 mod-tools root (default: $TA_TOOLS_PATH)")
	f.StringVar(&o.gamePath, "game-path", "", "BO3 game root (default: $TA_GAME_PATH, then --tools-path)")
	f.BoolVar(&o.isMod, "mod", false, "target is a mod (mods/<name>), not a usermap")
	f.StringVar(&o.stages, "stages", "compile,light,link", "comma list of stages: compile,light,link,run")
	f.BoolVar(&o.onlyEnts, "onlyents", false, "fast entity-only compile (-onlyents; invalid after brush edits)")
	f.StringVar(&o.light, "light", "medium", "light quality: low|medium|high")
	f.StringVar(&o.language, "language", "english", "linker language")
	f.BoolVar(&o.skipGDT, "skip-gdt", false, "skip the gdtdb /update pass before building")
	f.BoolVar(&o.gdtRebuild, "gdt-rebuild", false, "run gdtdb /rebuild instead of /update (only if /update reports 0 GDTs and the linker then misses an edited asset)")
	f.BoolVar(&o.jsonOut, "json", false, "emit the report as JSON")
	f.BoolVar(&o.verbose, "verbose", false, "stream each tool's full output as it runs")
	return cmd
}

func runBuild(cmd *cobra.Command, o *buildOpts, name string) error {
	stdout := cmd.OutOrStdout()
	rep, err := runBuildReport(o, name, stdout)
	if err != nil {
		return err
	}
	return finishBuild(stdout, o.jsonOut, rep)
}

// runBuildReport runs the mod-tools pipeline and returns the per-stage report. A
// non-nil error is only a preflight/validation failure (bad args, no tools path,
// tools not found) before any stage ran — stage failures are carried in the report
// (rep.OK / rep.FailedStage). stdout receives per-tool output only when o.verbose.
func runBuildReport(o *buildOpts, name string, stdout io.Writer) (buildReport, error) {
	if len(name) < 2 {
		return buildReport{}, fmt.Errorf("map/mod name %q is too short", name)
	}

	tools := firstNonEmpty(o.toolsPath, os.Getenv("TA_TOOLS_PATH"))
	tools = strings.TrimRight(tools, `\/`)
	if tools == "" {
		return buildReport{}, fmt.Errorf("no mod-tools path: pass --tools-path or set TA_TOOLS_PATH")
	}
	game := firstNonEmpty(o.gamePath, os.Getenv("TA_GAME_PATH"), tools)
	game = strings.TrimRight(game, `\/`)

	bin := filepath.Join(tools, "bin")
	if _, err := os.Stat(filepath.Join(bin, "linker_modtools.exe")); err != nil {
		return buildReport{}, fmt.Errorf("mod tools not found at %s — build needs a Windows BO3 mod-tools install", bin)
	}

	stages, err := parseStages(o.stages)
	if err != nil {
		return buildReport{}, err
	}
	quality, err := normalizeLight(o.light)
	if err != nil {
		return buildReport{}, err
	}

	pp := name[:2]
	mapSrc := filepath.Join(game, "map_source", pp, name+".map")
	d3dbsp := filepath.Join(game, "share", "raw", "maps", pp, name+".d3dbsp")
	led := filepath.Join(game, "share", "raw", "maps", pp, name+".led")

	rep := buildReport{Target: name, Kind: "usermap", OK: true}
	if o.isMod {
		rep.Kind = "mod"
	}

	run := func(sr stageResult) bool {
		rep.Stages = append(rep.Stages, sr)
		if !sr.OK {
			rep.OK = false
			rep.FailedStage = sr.Name
		}
		return sr.OK
	}

	// gdtdb /update first (the Launcher does this before every build group).
	if !o.skipGDT && (stages["compile"] || stages["light"] || stages["link"]) {
		gdtdbDir := filepath.Join(tools, "gdtdb")
		gdtdb := filepath.Join(gdtdbDir, "gdtdb.exe")
		// Run gdtdb from its OWN directory, like the stock Launcher: it ties recorded asset paths to
		// its cwd, so a different cwd makes a Launcher-built db flag every asset as a phantom duplicate.
		gdtArg := "/update"
		if o.gdtRebuild {
			gdtArg = "/rebuild" // recovery: /update does index GDTs edited outside APE (verified); this is for a db that lost everything
		}
		if !run(runStage("gdt", gdtdbDir, gdtdb, 10*time.Minute, o.verbose, stdout, gdtArg)) {
			return rep, nil
		}
	}

	// Compile and light only apply to maps.
	if stages["compile"] {
		if o.isMod {
			run(stageSkipped("compile", "not applicable to a mod"))
		} else {
			args := []string{"-platform", "pc"}
			if o.onlyEnts {
				args = append(args, "-onlyents")
			} else {
				args = append(args, "-navmesh", "-navvolume")
			}
			args = append(args, "-loadFrom", mapSrc, d3dbsp)
			start := time.Now()
			sr := runStage("compile", bin, filepath.Join(bin, "cod2map64.exe"), 15*time.Minute, o.verbose, stdout, args...)
			// cod2map exits 0 even when it skips navmesh; trust the .d3dbsp mtime.
			if sr.OK && !fileMTime(d3dbsp).After(start.Add(-2*time.Second)) {
				sr.OK = false
				sr.Errors = append(sr.Errors, "cod2map reported success but "+filepath.Base(d3dbsp)+" was not written")
			} else if sr.OK {
				sr.Note = "wrote " + filepath.Base(d3dbsp)
			}
			if !run(sr) {
				return rep, nil
			}
		}
	}

	if stages["light"] {
		if o.isMod {
			run(stageSkipped("light", "not applicable to a mod"))
		} else if !run(runLight(bin, mapSrc, led, quality)) {
			return rep, nil
		}
	}

	if stages["link"] {
		linker := filepath.Join(bin, "linker_modtools.exe")
		var args []string
		if o.isMod {
			args = []string{"-language", o.language, "-fs_game", name, "-modsource", name}
		} else {
			args = []string{"-language", o.language, "-modsource", name}
		}
		if !run(runStage("link", bin, linker, 20*time.Minute, o.verbose, stdout, args...)) {
			return rep, nil
		}
	}

	if stages["run"] {
		run(runGame(game, name, o.isMod))
	}

	return rep, nil
}

// runStage runs one tool to completion, capturing combined output, and returns a
// stageResult whose OK reflects the exit code and whose Errors hold any parsed
// error lines. dir is the working directory (cwd=bin for the mod tools).
func runStage(label, dir, exe string, timeout time.Duration, verbose bool, stdout io.Writer, args ...string) stageResult {
	start := time.Now()
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	c := exec.CommandContext(ctx, exe, args...)
	c.Dir = dir
	var buf bytes.Buffer
	if verbose {
		fmt.Fprintf(stdout, "» %s %s\n", filepath.Base(exe), strings.Join(args, " "))
		c.Stdout = io.MultiWriter(&buf, stdout)
		c.Stderr = io.MultiWriter(&buf, stdout)
	} else {
		c.Stdout = &buf
		c.Stderr = &buf
	}

	err := c.Run()
	sr := stageResult{Name: label, Seconds: round1(time.Since(start).Seconds())}
	switch {
	case ctx.Err() == context.DeadlineExceeded:
		sr.OK = false
		sr.Errors = []string{fmt.Sprintf("timed out after %s", timeout)}
	case err != nil:
		sr.OK = false
		sr.Errors = extractErrors(buf.String())
		if len(sr.Errors) == 0 {
			sr.Errors = []string{err.Error()}
		}
	default:
		sr.OK = true
		if e := extractErrors(buf.String()); len(e) > 0 {
			sr.Note = e[0] // a non-fatal ERROR/WARNING the tool logged but still exited 0 on
		}
	}
	return sr
}

// runLight launches the LED bake and waits by polling for the .led output — the
// radiant_modtools GUI-subsystem process detaches and gives no usable exit code.
func runLight(bin, mapSrc, led, quality string) stageResult {
	start := time.Now()
	prior := fileMTime(led)
	exe := filepath.Join(bin, "radiant_modtools.exe")
	c := exec.Command(exe, "-ledSilent", "+"+quality, "+localprobes", "+forceclean", "+recompute", mapSrc)
	c.Dir = bin
	if err := c.Start(); err != nil {
		return stageResult{Name: "light", OK: false, Seconds: round1(time.Since(start).Seconds()),
			Errors: []string{"could not launch radiant_modtools: " + err.Error()}}
	}
	go func() { _ = c.Wait() }() // reap; the worker bakes detached

	deadline := time.Now().Add(30 * time.Minute)
	for time.Now().Before(deadline) {
		if mt := fileMTime(led); mt.After(prior) && mt.After(start) {
			return stageResult{Name: "light", OK: true, Seconds: round1(time.Since(start).Seconds()),
				Note: "wrote " + filepath.Base(led)}
		}
		time.Sleep(2 * time.Second)
	}
	return stageResult{Name: "light", OK: false, Seconds: round1(time.Since(start).Seconds()),
		Errors: []string{"no " + filepath.Base(led) + " after 30m — radiant may have failed silently or the map has no lighting"}}
}

// runGame launches the game and returns immediately (fire-and-forget).
func runGame(game, name string, isMod bool) stageResult {
	exe := filepath.Join(game, "BlackOps3.exe")
	var args []string
	if isMod {
		args = append(args, "+set", "fs_game", name)
	}
	args = append(args, "+devmap", name)
	c := exec.Command(exe, args...)
	c.Dir = game
	if err := c.Start(); err != nil {
		return stageResult{Name: "run", OK: false, Errors: []string{"could not launch game: " + err.Error()}}
	}
	go func() { _ = c.Wait() }()
	return stageResult{Name: "run", OK: true, Note: "launched " + filepath.Base(exe)}
}

func finishBuild(out io.Writer, asJSON bool, rep buildReport) error {
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

func printBuildReport(out io.Writer, rep buildReport) {
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
	}
	if rep.OK {
		fmt.Fprintln(out, "build OK")
	} else {
		fmt.Fprintf(out, "build FAILED at %s\n", rep.FailedStage)
	}
}

func stageSkipped(name, why string) stageResult {
	return stageResult{Name: name, OK: true, Note: "skipped (" + why + ")"}
}

// extractErrors pulls the actionable error lines out of a tool's noisy output:
// the linker's UNRECOVERABLE/SCRIPT/ERR(...) lines and any ERROR:/unresolved
// external, with Quake-style ^N color codes stripped. Capped so the summary
// stays compact.
func extractErrors(out string) []string {
	var errs []string
	seen := map[string]bool{}
	for _, raw := range strings.Split(out, "\n") {
		t := stripColor(strings.TrimSpace(strings.ReplaceAll(raw, "\r", "")))
		if t == "" {
			continue
		}
		low := strings.ToLower(t)
		hit := strings.Contains(t, "UNRECOVERABLE ERROR") ||
			strings.HasPrefix(t, "ERROR:") ||
			strings.Contains(t, "SCRIPT ERROR") ||
			strings.HasPrefix(t, "ERR(") ||
			strings.Contains(low, "error linking") ||
			strings.Contains(low, "unresolved external") ||
			strings.Contains(low, "could not find scriptparsetree")
		if hit && !seen[t] {
			seen[t] = true
			errs = append(errs, t)
			if len(errs) >= 8 {
				break
			}
		}
	}
	return errs
}

func stripColor(s string) string {
	if !strings.Contains(s, "^") {
		return s
	}
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '^' && i+1 < len(s) && s[i+1] >= '0' && s[i+1] <= '9' {
			i++
			continue
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

func parseStages(s string) (map[string]bool, error) {
	out := map[string]bool{}
	for _, p := range strings.Split(s, ",") {
		p = strings.TrimSpace(strings.ToLower(p))
		if p == "" {
			continue
		}
		switch p {
		case "compile", "light", "link", "run":
			out[p] = true
		default:
			return nil, fmt.Errorf("unknown stage %q (want compile,light,link,run)", p)
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("no stages selected")
	}
	return out, nil
}

func normalizeLight(q string) (string, error) {
	switch strings.ToLower(q) {
	case "low", "medium", "high":
		return strings.ToLower(q), nil
	default:
		return "", fmt.Errorf("light quality %q must be low, medium, or high", q)
	}
}

func fileMTime(path string) time.Time {
	if fi, err := os.Stat(path); err == nil {
		return fi.ModTime()
	}
	return time.Time{}
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

func round1(f float64) float64 { return float64(int(f*10+0.5)) / 10 }
