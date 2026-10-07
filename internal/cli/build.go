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
	toolsPath     string
	gamePath      string
	isMod         bool
	stages        string
	onlyEnts      bool
	light         string
	language      string
	skipGDT       bool
	gdtRebuild    bool
	freshXpak     bool // link: write the .xpak files from scratch
	jsonOut       bool
	verbose       bool
	dvars         []string // run: name=value dvars to start the game with
	launcherDvars bool     // run: also the Launcher's saved Dvar Options
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
	f.BoolVar(&o.freshXpak, "fresh-xpak", false, "link: write the .xpak from scratch, dropping the data earlier links replaced (it only grows otherwise, and uploads with zone/); use before publishing")
	f.BoolVar(&o.jsonOut, "json", false, "emit the report as JSON")
	f.BoolVar(&o.verbose, "verbose", false, "stream each tool's full output as it runs")
	f.StringArrayVar(&o.dvars, "dvar", nil, "run: start the game with this dvar, name=value (repeatable; e.g. developer=2, logfile=2)")
	f.BoolVar(&o.launcherDvars, "launcher-dvars", false, "run: start the game with the dvars saved in the mod tools Launcher's Dvars dialog (--dvar overrides them)")
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

// The stage runners, as variables so tests can drive runBuildReport's pipeline
// without the mod tools.
var (
	stageRunner = runStage
	lightRunner = runLight
	gameRunner  = runGame
)

// runBuildReport runs the mod-tools pipeline and returns the per-stage report. A
// non-nil error is only a preflight/validation failure (bad args, no tools path,
// tools not found) before any stage ran — stage failures are carried in the report
// (rep.OK / rep.FailedStage). stdout receives per-tool output only when o.verbose.
func runBuildReport(o *buildOpts, name string, stdout io.Writer) (buildReport, error) {
	p, err := newBuildPlan(o, name, stdout)
	if err != nil {
		return buildReport{}, err
	}
	rep := buildReport{Target: name, Kind: "usermap", OK: true}
	if o.isMod {
		rep.Kind = "mod"
	}
	// In pipeline order; each returns false when it doesn't apply. The build stops
	// at the first stage that fails.
	for _, stage := range []func() (stageResult, bool){p.gdtStage, p.compileStage, p.lightStage, p.linkStage, p.gameStage} {
		sr, applies := stage()
		if !applies {
			continue
		}
		rep.Stages = append(rep.Stages, sr)
		if !sr.OK {
			rep.OK, rep.FailedStage = false, sr.Name
			break
		}
	}
	return rep, nil
}

// buildPlan is a validated build: where the tools and game are, which stages
// run, and the map's source and outputs.
type buildPlan struct {
	o                   *buildOpts
	name                string
	stdout              io.Writer
	tools, game, bin    string
	stages              map[string]bool
	quality             string
	mapSrc, d3dbsp, led string
	dvars               []dvar // the game's dvars, for the run stage
}

func newBuildPlan(o *buildOpts, name string, stdout io.Writer) (*buildPlan, error) {
	if len(name) < 2 {
		return nil, fmt.Errorf("map/mod name %q is too short", name)
	}
	tools := strings.TrimRight(firstNonEmpty(o.toolsPath, os.Getenv("TA_TOOLS_PATH")), `\/`)
	if tools == "" {
		return nil, fmt.Errorf("no mod-tools path: pass --tools-path or set TA_TOOLS_PATH")
	}
	p := &buildPlan{o: o, name: name, stdout: stdout, tools: tools, bin: filepath.Join(tools, "bin")}
	p.game = strings.TrimRight(firstNonEmpty(o.gamePath, os.Getenv("TA_GAME_PATH"), tools), `\/`)
	if _, err := os.Stat(filepath.Join(p.bin, "linker_modtools.exe")); err != nil {
		return nil, fmt.Errorf("mod tools not found at %s — build needs a Windows BO3 mod-tools install", p.bin)
	}
	var err error
	if p.stages, err = parseStages(o.stages); err != nil {
		return nil, err
	}
	if p.quality, err = normalizeLight(o.light); err != nil {
		return nil, err
	}
	if p.dvars, err = gameDvars(o); err != nil {
		return nil, err
	}
	pp := name[:2]
	p.mapSrc = filepath.Join(p.game, "map_source", pp, name+".map")
	p.d3dbsp = filepath.Join(p.game, "share", "raw", "maps", pp, name+".d3dbsp")
	p.led = filepath.Join(p.game, "share", "raw", "maps", pp, name+".led")
	return p, nil
}

// gdtStage runs gdtdb /update first, as the Launcher does before every build group.
func (p *buildPlan) gdtStage() (stageResult, bool) {
	usesAssets := p.stages["compile"] || p.stages["light"] || p.stages["link"]
	if p.o.skipGDT || !usesAssets {
		return stageResult{}, false
	}
	// Run gdtdb from its OWN directory, like the stock Launcher: it ties recorded asset paths to
	// its cwd, so a different cwd makes a Launcher-built db flag every asset as a phantom duplicate.
	gdtdbDir := filepath.Join(p.tools, "gdtdb")
	arg := "/update"
	if p.o.gdtRebuild {
		arg = "/rebuild" // recovery: /update does index GDTs edited outside APE (verified); this is for a db that lost everything
	}
	return stageRunner("gdt", gdtdbDir, filepath.Join(gdtdbDir, "gdtdb.exe"), 10*time.Minute, p.o.verbose, p.stdout, arg), true
}

// compileStage runs cod2map (maps only).
func (p *buildPlan) compileStage() (stageResult, bool) {
	if !p.stages["compile"] {
		return stageResult{}, false
	}
	if p.o.isMod {
		return stageSkipped("compile", "not applicable to a mod"), true
	}
	args := []string{"-platform", "pc", "-navmesh", "-navvolume"}
	if p.o.onlyEnts {
		args = []string{"-platform", "pc", "-onlyents"}
	}
	args = append(args, "-loadFrom", p.mapSrc, p.d3dbsp)
	start := time.Now()
	sr := stageRunner("compile", p.bin, filepath.Join(p.bin, "cod2map64.exe"), 15*time.Minute, p.o.verbose, p.stdout, args...)
	switch {
	case !sr.OK:
	case !fileMTime(p.d3dbsp).After(start.Add(-2 * time.Second)): // cod2map exits 0 even when it skips navmesh; trust the .d3dbsp mtime
		sr.OK = false
		sr.Errors = append(sr.Errors, "cod2map reported success but "+filepath.Base(p.d3dbsp)+" was not written")
	default:
		sr.Note = "wrote " + filepath.Base(p.d3dbsp)
	}
	return sr, true
}

// lightStage bakes the lighting (maps only).
func (p *buildPlan) lightStage() (stageResult, bool) {
	if !p.stages["light"] {
		return stageResult{}, false
	}
	if p.o.isMod {
		return stageSkipped("light", "not applicable to a mod"), true
	}
	return lightRunner(p.bin, p.mapSrc, p.led, p.quality), true
}

// linkStage links the fast files.
func (p *buildPlan) linkStage() (stageResult, bool) {
	if !p.stages["link"] {
		return stageResult{}, false
	}
	if !p.o.freshXpak {
		return p.linkTarget(), true
	}
	aside, err := setXpaksAside(p.zoneDir())
	if err != nil {
		return stageResult{Name: "link", Errors: []string{err.Error()}}, true
	}
	sr := p.linkTarget()
	sr.Note = joinNotes(sr.Note, aside.settle(sr.OK))
	return sr, true
}

func (p *buildPlan) linkTarget() stageResult {
	if p.o.isMod {
		return p.linkMod()
	}
	start := time.Now()
	sr := p.link("-modsource", p.name)
	errorlog := filepath.Join(p.game, "usermaps", p.name, "zone_source", "all", "assetinfo", p.name+".errorlog")
	if note, ok := warningsOnly(errorlog, start); !sr.OK && ok {
		sr.OK, sr.Errors, sr.Note = true, nil, note
	}
	return sr
}

func (p *buildPlan) link(args ...string) stageResult {
	args = append([]string{"-language", p.o.language}, args...)
	// 60 min: a map's first link converts every image it packs (~23 min for the ZM Advanced template)
	return stageRunner("link", p.bin, filepath.Join(p.bin, "linker_modtools.exe"), 60*time.Minute, p.o.verbose, p.stdout, args...)
}

// modZones are the zones a mod can have, linked one by one in this order, as
// the Launcher does (`-fs_game <mod> -modsource <zone>` for each present).
var modZones = []string{"core_mod", "mp_mod", "cp_mod", "zm_mod"}

func (p *buildPlan) linkMod() stageResult {
	total := stageResult{Name: "link", OK: true}
	var linked []string
	for _, z := range modZones {
		if fileMTime(filepath.Join(p.game, "mods", p.name, "zone_source", z+".zone")).IsZero() {
			continue
		}
		sr := p.link("-fs_game", p.name, "-modsource", z)
		total.Seconds = round1(total.Seconds + sr.Seconds)
		if !sr.OK {
			total.OK = false
			for _, e := range sr.Errors {
				total.Errors = append(total.Errors, z+": "+e)
			}
			return total
		}
		linked = append(linked, z)
	}
	if len(linked) == 0 {
		total.OK = false
		total.Errors = []string{"no core_mod, mp_mod, cp_mod or zm_mod .zone in mods/" + p.name + "/zone_source"}
		return total
	}
	total.Note = "linked " + strings.Join(linked, ", ")
	return total
}

// warningsOnly reads the errorlog the linker wrote for this link (modified
// since start): it exits non-zero on warnings too, which the errorlog prefixes
// with ^3, while an error is a ^1 line. With no error, the link succeeded.
func warningsOnly(errorlog string, since time.Time) (string, bool) {
	if fileMTime(errorlog).Before(since.Add(-2 * time.Second)) {
		return "", false // not written by this link
	}
	b, err := os.ReadFile(errorlog)
	if err != nil {
		return "", false
	}
	var warning string
	for _, l := range strings.Split(string(b), "\n") {
		l = strings.TrimSpace(l)
		switch {
		case strings.HasPrefix(l, "^1"):
			return "", false
		case warning == "" && strings.HasPrefix(l, "^3"):
			warning = strings.TrimPrefix(l, "^3")
		}
	}
	return "linker warnings only, the fastfile is built: " + warning, true
}

// gameStage starts the game on the build.
func (p *buildPlan) gameStage() (stageResult, bool) {
	if !p.stages["run"] {
		return stageResult{}, false
	}
	return gameRunner(p.game, append(dvarArgs(p.dvars), gameArgs(p.name, p.o.isMod)...)), true
}

// gameDvars are the dvars the run stage starts the game with: the Launcher's
// saved ones if asked, overridden by the explicit ones.
func gameDvars(o *buildOpts) ([]dvar, error) {
	extra, err := parseDvars(o.dvars)
	if err != nil {
		return nil, err
	}
	var base []dvar
	if o.launcherDvars {
		if base, err = readLauncherDvars(); err != nil {
			return nil, err
		}
	}
	return mergeDvars(base, extra), nil
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
// bo3AppID is Black Ops III's Steam app id.
const bo3AppID = "311210"

// gameStartup is how long the game must stay up for run to count as launched:
// without Steam attached it shows "you must launch Steam" and exits at once.
var gameStartup = 8 * time.Second

func runGame(game string, args []string) stageResult {
	exe := filepath.Join(game, "BlackOps3.exe")
	c := exec.Command(exe, args...)
	c.Dir = game
	c.Env = gameEnv(os.Environ())
	if err := c.Start(); err != nil {
		return stageResult{Name: "run", OK: false, Errors: []string{"could not launch game: " + err.Error()}}
	}
	exited := make(chan error, 1)
	go func() { exited <- c.Wait() }()
	select {
	case err := <-exited:
		msg := "the game exited right after starting"
		if err != nil {
			msg += " (" + err.Error() + ")"
		}
		return stageResult{Name: "run", OK: false, Errors: []string{msg + " — is Steam running and signed in? A game it shows \"you must launch Steam\" in closes at once"}}
	case <-time.After(gameStartup):
		return stageResult{Name: "run", OK: true, Note: "launched " + filepath.Base(exe)}
	}
}

// gameArgs are the Launcher's own run arguments (captured from the stock
// modlauncher.exe starting the game): fs_game is the map's name for a usermap
// too — without it the game never mounts usermaps/<map>/ and +devmap leaves it
// on the main menu. A mod is mounted with fs_game and loaded from the menu, so it
// gets no +devmap.
func gameArgs(name string, isMod bool) []string {
	args := []string{"+set", "fs_game", name}
	if !isMod {
		args = append(args, "+devmap", name)
	}
	return args
}

// gameEnv is the environment the game starts with: Steam starts the mod tools
// Launcher with SteamAppId/SteamGameId set and the game inherits them; started
// from anywhere else, BlackOps3.exe can't attach to the running Steam client and
// refuses with "you must launch Steam to play the game" (verified on a real
// install: the same command runs once SteamAppId=311210 is set).
func gameEnv(env []string) []string {
	out := make([]string, 0, len(env)+2)
	for _, kv := range env {
		k, _, _ := strings.Cut(kv, "=")
		if !strings.EqualFold(k, "SteamAppId") && !strings.EqualFold(k, "SteamGameId") {
			out = append(out, kv)
		}
	}
	return append(out, "SteamAppId="+bo3AppID, "SteamGameId="+bo3AppID)
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
