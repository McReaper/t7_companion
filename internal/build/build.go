package build

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// Options is one build: the target's install, which stages run, and how.
type Options struct {
	ToolsPath     string
	GamePath      string
	Mod           bool
	Stages        string
	OnlyEnts      bool
	Light         string
	Language      string
	SkipGDT       bool
	GDTRebuild    bool
	FreshXpak     bool // link: write the .xpak files from scratch
	Verbose       bool
	Dvars         []string // run: name=value dvars to start the game with
	LauncherDvars bool     // run: also the Launcher's saved Dvar Options
}

// StageResult is what one stage did; Report.Stages lists them in pipeline order.
type StageResult struct {
	Name     string   `json:"name"`
	OK       bool     `json:"ok"`
	Seconds  float64  `json:"seconds"`
	Errors   []string `json:"errors,omitempty"`
	Note     string   `json:"note,omitempty"`
	Warnings []string `json:"warnings,omitempty"` // compile: cod2map's WARNING lines and .errlog; link: every ^3 entry of the errorlogs it wrote
	Missing  []string `json:"missing,omitempty"`  // link: looked up, found in no GDT, linked without
	output   string   // what the tool printed
}

// Report is a build's result: OK, or the stage that failed.
type Report struct {
	Target      string        `json:"target"`
	Kind        string        `json:"kind"`
	OK          bool          `json:"ok"`
	FailedStage string        `json:"failed_stage,omitempty"`
	Stages      []StageResult `json:"stages"`
}

// The stage runners, as variables so tests can drive runBuildReport's pipeline
// without the mod tools.
var (
	stageRunner = runStage
	lightRunner = runLight
	gameRunner  = runGame
)

// Run runs the mod-tools pipeline and returns the per-stage report. A
// non-nil error is only a preflight/validation failure (bad args, no tools path,
// tools not found) before any stage ran — stage failures are carried in the report
// (rep.OK / rep.FailedStage). stdout receives per-tool output only when o.verbose.
func Run(o *Options, name string, stdout io.Writer) (Report, error) {
	p, err := newBuildPlan(o, name, stdout)
	if err != nil {
		return Report{}, err
	}
	rep := Report{Target: name, Kind: "usermap", OK: true}
	if o.Mod {
		rep.Kind = "mod"
	}
	// In pipeline order; each returns false when it doesn't apply. The build stops
	// at the first stage that fails.
	for _, stage := range []func() (StageResult, bool){p.gdtStage, p.compileStage, p.lightStage, p.linkStage, p.gameStage} {
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
	o                   *Options
	name                string
	stdout              io.Writer
	tools, game, bin    string
	stages              map[string]bool
	quality             string
	mapSrc, d3dbsp, led string
	dvars               []dvar // the game's dvars, for the run stage
	language            string // the link's, and why when it wasn't asked for
	languageNote        string
}

func newBuildPlan(o *Options, name string, stdout io.Writer) (*buildPlan, error) {
	if len(name) < 2 {
		return nil, fmt.Errorf("map/mod name %q is too short", name)
	}
	tools := strings.TrimRight(firstNonEmpty(o.ToolsPath, os.Getenv("TA_TOOLS_PATH")), `\/`)
	if tools == "" {
		return nil, fmt.Errorf("no mod-tools path: pass --tools-path or set TA_TOOLS_PATH")
	}
	p := &buildPlan{o: o, name: name, stdout: stdout, tools: tools, bin: filepath.Join(tools, "bin")}
	p.game = strings.TrimRight(firstNonEmpty(o.GamePath, os.Getenv("TA_GAME_PATH"), tools), `\/`)
	if _, err := os.Stat(filepath.Join(p.bin, "linker_modtools.exe")); err != nil {
		return nil, fmt.Errorf("mod tools not found at %s — build needs a Windows BO3 mod-tools install", p.bin)
	}
	var err error
	if p.stages, err = parseStages(o.Stages); err != nil {
		return nil, err
	}
	if p.quality, err = normalizeLight(o.Light); err != nil {
		return nil, err
	}
	if p.dvars, err = gameDvars(o); err != nil {
		return nil, err
	}
	p.language, p.languageNote = linkLanguage(o.Language, p.game)
	pp := name[:2]
	p.mapSrc = filepath.Join(p.game, "map_source", pp, name+".map")
	p.d3dbsp = filepath.Join(p.game, "share", "raw", "maps", pp, name+".d3dbsp")
	p.led = filepath.Join(p.game, "share", "raw", "maps", pp, name+".led")
	return p, nil
}

// gdtStage runs gdtdb /update first, as the Launcher does before every build group.
func (p *buildPlan) gdtStage() (StageResult, bool) {
	usesAssets := p.stages["compile"] || p.stages["light"] || p.stages["link"]
	if p.o.SkipGDT || !usesAssets {
		return StageResult{}, false
	}
	// Run gdtdb from its OWN directory, like the stock Launcher: it ties recorded asset paths to
	// its cwd, so a different cwd makes a Launcher-built db flag every asset as a phantom duplicate.
	gdtdbDir := filepath.Join(p.tools, "gdtdb")
	arg := "/update"
	if p.o.GDTRebuild {
		arg = "/rebuild" // recovery: /update does index GDTs edited outside APE (verified); this is for a db that lost everything
	}
	return stageRunner("gdt", gdtdbDir, filepath.Join(gdtdbDir, "gdtdb.exe"), 10*time.Minute, p.o.Verbose, p.stdout, arg), true
}

// compileStage runs cod2map (maps only).
func (p *buildPlan) compileStage() (StageResult, bool) {
	if !p.stages["compile"] {
		return StageResult{}, false
	}
	if p.o.Mod {
		return stageSkipped("compile", "not applicable to a mod"), true
	}
	args := []string{"-platform", "pc", "-navmesh", "-navvolume"}
	if p.o.OnlyEnts {
		args = []string{"-platform", "pc", "-onlyents"}
	}
	args = append(args, "-loadFrom", p.mapSrc, p.d3dbsp)
	start := time.Now()
	sr := stageRunner("compile", p.bin, filepath.Join(p.bin, "cod2map64.exe"), 15*time.Minute, p.o.Verbose, p.stdout, args...)
	switch {
	case !sr.OK:
	case !fileMTime(p.d3dbsp).After(start.Add(-2 * time.Second)): // cod2map exits 0 even when it skips navmesh; trust the .d3dbsp mtime
		sr.OK = false
		sr.Errors = append(sr.Errors, "cod2map reported success but "+filepath.Base(p.d3dbsp)+" was not written")
	default:
		sr.Note = "wrote " + filepath.Base(p.d3dbsp)
		compileVerdict(&sr, sr.output, strings.TrimSuffix(p.mapSrc, filepath.Ext(p.mapSrc))+".errlog", start)
	}
	return sr, true
}

// lightStage bakes the lighting (maps only).
func (p *buildPlan) lightStage() (StageResult, bool) {
	if !p.stages["light"] {
		return StageResult{}, false
	}
	if p.o.Mod {
		return stageSkipped("light", "not applicable to a mod"), true
	}
	return lightRunner(p.bin, p.mapSrc, p.led, p.quality), true
}

// gameStage starts the game on the build, and names the log it wrote.
func (p *buildPlan) gameStage() (StageResult, bool) {
	if !p.stages["run"] {
		return StageResult{}, false
	}
	start := time.Now()
	sr := gameRunner(p.game, append(dvarArgs(withGameLog(p.dvars)), gameArgs(p.name, p.o.Mod)...))
	noteRunLog(&sr, p.game, p.name, start)
	return sr, true
}

// gameDvars are the dvars the run stage starts the game with: the Launcher's
// saved ones if asked, overridden by the explicit ones.
func gameDvars(o *Options) ([]dvar, error) {
	extra, err := parseDvars(o.Dvars)
	if err != nil {
		return nil, err
	}
	var base []dvar
	if o.LauncherDvars {
		if base, err = readLauncherDvars(); err != nil {
			return nil, err
		}
	}
	return mergeDvars(base, extra), nil
}

// runStage runs one tool to completion, capturing combined output, and returns a
// stageResult whose OK reflects the exit code and whose Errors hold any parsed
// error lines. dir is the working directory (cwd=bin for the mod tools).
func runStage(label, dir, exe string, timeout time.Duration, verbose bool, stdout io.Writer, args ...string) StageResult {
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
	sr := StageResult{Name: label, Seconds: round1(time.Since(start).Seconds()), output: buf.String()}
	switch {
	case ctx.Err() == context.DeadlineExceeded:
		sr.OK = false
		sr.Errors = []string{fmt.Sprintf("timed out after %s", timeout)}
	case err != nil:
		sr.OK = false
		sr.Errors = failureLines(buf.String(), err)
	default:
		sr.OK = true
		if e := extractErrors(buf.String()); len(e) > 0 {
			sr.Note = e[0] // a non-fatal ERROR/WARNING the tool logged but still exited 0 on
		}
	}
	return sr
}

// How long runLight waits for the .led, and how often it looks; variables so
// tests can wait less.
var (
	lightTimeout = 30 * time.Minute
	lightPoll    = 2 * time.Second
)

// runLight launches the LED bake and waits by polling for the .led output — the
// radiant_modtools GUI-subsystem process detaches and gives no usable exit code.
func runLight(bin, mapSrc, led, quality string) StageResult {
	start := time.Now()
	prior := fileMTime(led)
	exe := filepath.Join(bin, "radiant_modtools.exe")
	c := exec.Command(exe, "-ledSilent", "+"+quality, "+localprobes", "+forceclean", "+recompute", mapSrc)
	c.Dir = bin
	if err := c.Start(); err != nil {
		return StageResult{Name: "light", OK: false, Seconds: round1(time.Since(start).Seconds()),
			Errors: []string{"could not launch radiant_modtools: " + err.Error()}}
	}
	go func() { _ = c.Wait() }() // reap; the worker bakes detached

	deadline := time.Now().Add(lightTimeout)
	for time.Now().Before(deadline) {
		if mt := fileMTime(led); mt.After(prior) && mt.After(start) {
			return StageResult{Name: "light", OK: true, Seconds: round1(time.Since(start).Seconds()),
				Note: "wrote " + filepath.Base(led)}
		}
		time.Sleep(lightPoll)
	}
	return StageResult{Name: "light", OK: false, Seconds: round1(time.Since(start).Seconds()),
		Errors: []string{"no " + filepath.Base(led) + " after " + lightTimeout.String() + " — radiant may have failed silently or the map has no lighting"}}
}

func stageSkipped(name, why string) StageResult {
	return StageResult{Name: name, OK: true, Note: "skipped (" + why + ")"}
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

// unknownFailure ends a failed tool's result when no line of its output
// matched extractErrors: the patterns t7kb knows are incomplete, and the fix
// for that is upstream.
const unknownFailure = "no line of this output matched an error t7kb knows: once you find the cause, " +
	"offer the user t7kb:contribute to teach t7kb's build tool this error"

// failureLines is what a failed tool said: its actionable error lines or,
// when none matches, its whole console output — an unknown message still
// reaches the agent — then the exit status and unknownFailure.
func failureLines(out string, err error) []string {
	if errs := extractErrors(out); len(errs) > 0 {
		return errs
	}
	var lines []string
	for _, raw := range strings.Split(strings.ReplaceAll(out, "\r", ""), "\n") {
		if l := stripColor(strings.TrimSpace(raw)); l != "" {
			lines = append(lines, l)
		}
	}
	return append(lines, err.Error(), unknownFailure)
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
