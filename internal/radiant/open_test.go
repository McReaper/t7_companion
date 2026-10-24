package radiant_test

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/McReaper/t7_companion/internal/radiant"
	"github.com/McReaper/t7_companion/internal/radiant/radianttest"
)

// install is a mod-tools root with Radiant and two maps.
func install(t *testing.T) (root, a, b string) {
	t.Helper()
	t.Cleanup(radiant.SetPollEvery(time.Millisecond))
	root = t.TempDir()
	for _, f := range []string{"bin/radiant_modtools.exe", "map_source/a.map", "map_source/zm/zm_b.map"} {
		writeFile(t, filepath.Join(root, f), "x")
	}
	return root, filepath.Join(root, "map_source", "a.map"), filepath.Join(root, "map_source", "zm", "zm_b.map")
}

func writeFile(t *testing.T, p, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// bridge is the folder of the bridge DLLs t7kb ships beside its binary.
func bridge(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	writeFile(t, radiant.StandaloneDLL(dir), "standalone")
	writeFile(t, radiant.PluginDLL(dir), "plugin")
	return dir
}

// withMTPlugins installs MTPlugins' loader in the root's bin/, its ini saying enabled.
func withMTPlugins(t *testing.T, root, enabled string) {
	t.Helper()
	writeFile(t, filepath.Join(root, "bin", "d3d9.dll"), "proxy")
	writeFile(t, filepath.Join(root, "bin", "RadiantLoader.ini"), "[loader]\r\n; 0 = forward only\r\nenabled="+enabled+"\r\n")
}

func open(t *testing.T, m radiant.Machine, o radiant.OpenOptions) (radiant.Opened, error) {
	t.Helper()
	c, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	return radiant.Open(c, m, o)
}

func reloads(f *radianttest.Fake) []string {
	var out []string
	for _, r := range f.Requests() {
		if strings.HasPrefix(r, "reload ") {
			out = append(out, r)
		}
	}
	return out
}

func TestOpenUsesTheRadiantThatHasTheMapOpen(t *testing.T) {
	root, a, b := install(t)
	other := radianttest.New(11, b)
	other.Modified = true
	has := radianttest.New(12, strings.ToUpper(a)) // Radiant's case, not ours
	got, err := open(t, radianttest.NewMachine(other, has), radiant.OpenOptions{Root: root, Map: a})
	if err != nil || got != (radiant.Opened{PID: 12, Map: strings.ToUpper(a), How: radiant.AlreadyOpen, Plugin: "1.2.0"}) {
		t.Errorf("open = %+v, %v", got, err)
	}
	if r := append(reloads(other), reloads(has)...); len(r) != 0 {
		t.Errorf("reloads = %q, want none", r)
	}
}

func TestOpenReloadsTheOnlyRadiant(t *testing.T) {
	root, a, b := install(t)
	f := radianttest.New(5, b)
	f.LoadAfter = 20 * time.Millisecond
	got, err := open(t, radianttest.NewMachine(f), radiant.OpenOptions{Root: root, Map: a})
	if err != nil || got.How != radiant.Reloaded || got.PID != 5 || got.Map != a {
		t.Errorf("open = %+v, %v", got, err)
	}
	if r := reloads(f); !slices.Equal(r, []string{"reload " + a}) {
		t.Errorf("reloads = %q", r)
	}
}

func TestOpenKeepsUnsavedWorkUnlessToldToDropIt(t *testing.T) {
	root, a, b := install(t)
	f := radianttest.New(5, b)
	f.Modified = true
	m := radianttest.NewMachine(f)
	_, err := open(t, m, radiant.OpenOptions{Root: root, Map: a})
	if err == nil || !strings.Contains(err.Error(), "unsaved changes to "+b) || !strings.Contains(err.Error(), "discard") {
		t.Errorf("err = %v", err)
	}
	if r := reloads(f); len(r) != 0 {
		t.Errorf("reloaded over unsaved work: %q", r)
	}
	got, err := open(t, m, radiant.OpenOptions{Root: root, Map: a, Discard: true})
	if err != nil || got.How != radiant.Reloaded {
		t.Errorf("open with discard = %+v, %v", got, err)
	}
}

func TestOpenLeavesADialogOrAPrefabToTheUser(t *testing.T) {
	root, a, b := install(t)
	for name, set := range map[string]func(*radianttest.Fake){
		`the dialog "Find"`: func(f *radianttest.Fake) { f.Dialog = "Find" },
		"inside a prefab":   func(f *radianttest.Fake) { f.PrefabDepth = 1 },
	} {
		f := radianttest.New(5, b)
		set(f)
		_, err := open(t, radianttest.NewMachine(f), radiant.OpenOptions{Root: root, Map: a, Discard: true})
		if err == nil || !strings.Contains(err.Error(), name) {
			t.Errorf("%s: err = %v", name, err)
		}
	}
}

func TestOpenAmongSeveralRadiantsNeedsAPid(t *testing.T) {
	root, a, b := install(t)
	one, two := radianttest.New(21, b), radianttest.New(22, "")
	m := radianttest.NewMachine(one, two)
	_, err := open(t, m, radiant.OpenOptions{Root: root, Map: a})
	if err == nil || !strings.Contains(err.Error(), "pid 21: "+b) || !strings.Contains(err.Error(), "pid 22: no map") {
		t.Errorf("err = %v, want both Radiants named", err)
	}
	got, err := open(t, m, radiant.OpenOptions{Root: root, Map: a, PID: 22})
	if err != nil || got.PID != 22 || got.How != radiant.Reloaded || len(reloads(one)) != 0 {
		t.Errorf("open in pid 22 = %+v, %v", got, err)
	}
	if _, err := open(t, m, radiant.OpenOptions{Root: root, Map: b, PID: 23}); err == nil || !strings.Contains(err.Error(), "no Radiant with pid 23") {
		t.Errorf("unknown pid: err = %v", err)
	}
}

func TestOpenAMapThatIsNotThere(t *testing.T) {
	root, _, _ := install(t)
	_, err := open(t, radianttest.NewMachine(), radiant.OpenOptions{Root: root, Map: filepath.Join(root, "map_source", "nope.map")})
	if err == nil || !strings.Contains(err.Error(), "no map at") {
		t.Errorf("err = %v", err)
	}
}

// starting makes the Radiant a Start starts: its bridge listens once loaded,
// after a while, and it then loads the map it was started on; mods are the
// modules it lists besides its exe.
func starting(listen, load time.Duration, mods ...string) func(int, string, string, []string) *radianttest.Started {
	return func(pid int, exe, _ string, args []string) *radianttest.Started {
		f := radianttest.New(pid, "")
		time.AfterFunc(load, func() { f.Update(func(f *radianttest.Fake) { f.Map = args[0] }) })
		return &radianttest.Started{Fake: f, ListenAfter: listen, Unloaded: true, Modules: append([]string{exe}, mods...)}
	}
}

func TestOpenStartsRadiantOnTheMapAndInjectsTheBridge(t *testing.T) {
	root, a, _ := install(t)
	dir := bridge(t)
	m := radianttest.NewMachine()
	m.OnStart = starting(20*time.Millisecond, 40*time.Millisecond)
	got, err := open(t, m, radiant.OpenOptions{Root: root, Map: a, Bridge: dir})
	if err != nil || got.How != radiant.Launched || got.Map != a || got.PID == 0 {
		t.Errorf("open = %+v, %v", got, err)
	}
	bin := filepath.Join(root, "bin")
	if s := m.Starts(); len(s) != 1 || !slices.Equal(s[0], []string{filepath.Join(bin, "radiant_modtools.exe"), bin, a}) {
		t.Errorf("starts = %q, want Radiant from bin/ on the map", s)
	}
	if in := m.Injected(); len(in) != 1 || in[0][1] != radiant.StandaloneDLL(dir) || m.Controls() != nil {
		t.Errorf("without MTPlugins the standalone bridge is injected: %q, controls %q", in, m.Controls())
	}
	if _, err := os.Stat(filepath.Join(bin, "plugins")); err == nil {
		t.Error("nothing is written into the install")
	}
}

func TestOpenLoadsThePluginThroughMTPlugins(t *testing.T) {
	for name, tc := range map[string]struct {
		installed, loaded string        // the plugin in bin/plugins, "" for none; its module listed when loaded
		controlAfter      time.Duration // when the host's control window opens
		want              []string
	}{
		"none installed":    {want: []string{"plugins list", "plugins rescan", "plugins enable T7RadiantBridge"}},
		"ours, loaded":      {installed: "plugin", loaded: "yes", want: []string{"plugins list", "plugins enable T7RadiantBridge"}},
		"another, loaded":   {installed: "older", loaded: "yes", want: []string{"plugins list", "plugins disable T7RadiantBridge", "plugins rescan", "plugins enable T7RadiantBridge"}},
		"another, unloaded": {installed: "older", want: []string{"plugins list", "plugins rescan", "plugins enable T7RadiantBridge"}},
		// the host loads seconds before it takes commands: nothing is sent, or copied, until it does
		"the control window opens late": {installed: "older", loaded: "yes", controlAfter: 30 * time.Millisecond,
			want: []string{"plugins list", "plugins disable T7RadiantBridge", "plugins rescan", "plugins enable T7RadiantBridge"}},
	} {
		t.Run(name, func(t *testing.T) {
			root, a, _ := install(t)
			withMTPlugins(t, root, "1")
			dst := filepath.Join(root, "bin", "plugins", "T7RadiantBridge", "T7RadiantBridge.dll")
			if tc.installed != "" {
				writeFile(t, dst, tc.installed)
			}
			mods := []string{filepath.Join(root, "bin", "plugins", "RadiantPlugins.dll")}
			if tc.loaded != "" {
				mods = append(mods, dst)
			}
			m := radianttest.NewMachine()
			start := starting(0, 0, mods...)
			m.OnStart = func(pid int, exe, dir string, args []string) *radianttest.Started {
				s := start(pid, exe, dir, args)
				s.ControlAfter = tc.controlAfter
				return s
			}
			got, err := open(t, m, radiant.OpenOptions{Root: root, Map: a, Bridge: bridge(t)})
			if err != nil || got.How != radiant.Launched {
				t.Fatalf("open = %+v, %v", got, err)
			}
			var cmds []string
			for _, c := range m.Controls() {
				cmds = append(cmds, c[1])
			}
			if !slices.Equal(cmds, tc.want) || m.Injected() != nil {
				t.Errorf("MTPlugins hosts the plugin, never alongside an injected one: %q, injected %q", cmds, m.Injected())
			}
			if b, _ := os.ReadFile(dst); string(b) != "plugin" {
				t.Errorf("the install's plugin is t7kb's: %q", b)
			}
		})
	}
}

func TestOpenLoadsTheBridgeIntoARadiantRunningWithoutIt(t *testing.T) {
	root, a, b := install(t)
	dir := bridge(t)
	f := radianttest.New(21, b)
	m := radianttest.NewMachine()
	m.Run(&radianttest.Started{Fake: f, Unloaded: true, Modules: []string{"radiant_modtools.exe"}})
	got, err := open(t, m, radiant.OpenOptions{Root: root, Map: a, Bridge: dir})
	if err != nil || got.How != radiant.Reloaded || got.PID != 21 || len(reloads(f)) != 1 {
		t.Errorf("open = %+v, %v; reloads %q", got, err, reloads(f))
	}
	if in := m.Injected(); len(in) != 1 || in[0][0] != "21" {
		t.Errorf("injected %q", in)
	}

	m = radianttest.NewMachine()
	m.Run(&radianttest.Started{Fake: radianttest.New(31, a), Unloaded: true})
	if got, err := open(t, m, radiant.OpenOptions{Root: root, Map: a, Bridge: dir}); err != nil || got.How != radiant.AlreadyOpen {
		t.Errorf("one with the map open already: %+v, %v", got, err)
	}

	m = radianttest.NewMachine()
	m.Run(&radianttest.Started{Fake: radianttest.New(41, b), Unloaded: true})
	m.Run(&radianttest.Started{Fake: radianttest.New(42, b), Unloaded: true})
	if _, err := open(t, m, radiant.OpenOptions{Root: root, Map: a, Bridge: dir}); err == nil || !strings.Contains(err.Error(), "2 Radiants run without the bridge (pids [41 42])") {
		t.Errorf("several: %v", err)
	}
	if got, err := open(t, m, radiant.OpenOptions{Root: root, Map: a, Bridge: dir, PID: 42}); err != nil || got.PID != 42 {
		t.Errorf("the pid picks one: %+v, %v", got, err)
	}
	if _, err := open(t, m, radiant.OpenOptions{Root: root, Map: a, Bridge: dir, PID: 77}); err == nil || !strings.Contains(err.Error(), "no Radiant with pid 77 runs") {
		t.Errorf("an unknown pid: %v", err)
	}
}

func TestOpenSaysWhyRadiantCouldNotStart(t *testing.T) {
	for name, tc := range map[string]struct {
		noBridge, noExe bool
		mtplugins       string // MTPlugins installed, enabled=…
		onStart         func(int, string, string, []string) *radianttest.Started
		want            string
	}{
		"no bridge beside t7kb": {noBridge: true, onStart: starting(0, 0), want: "Radiant bridge isn't beside it"},
		"no plugin beside t7kb": {noBridge: true, mtplugins: "1", onStart: starting(0, 0, "RadiantPlugins.dll"), want: "Radiant bridge isn't beside it"},
		"no Radiant":            {noExe: true, want: "no Radiant at"},
		"exits":                 {want: "exited before its bridge answered"},
		"never loads": {onStart: func(int, string, string, []string) *radianttest.Started {
			return &radianttest.Started{Fake: radianttest.New(0, ""), Silent: true}
		}, want: "its bridge never answered"},
		"map never opens":      {onStart: starting(0, time.Hour), want: `the map Radiant has open is still ""`},
		"MTPlugins' host late": {mtplugins: "1", onStart: starting(0, 0), want: "its host never answered in Radiant"},
		"MTPlugins turned off": {mtplugins: "0", onStart: starting(0, time.Hour), want: `the map Radiant has open is still ""`},
		"an old bridge in it": {onStart: func(pid int, exe, _ string, _ []string) *radianttest.Started {
			return &radianttest.Started{Fake: radianttest.New(0, ""), Unloaded: true, Modules: []string{exe, filepath.Join("t7kb", "T7RadiantBridge-1.0.0.dll")}}
		}, want: "runs the bridge T7RadiantBridge-1.0.0.dll, which doesn't answer: close Radiant"},
	} {
		t.Run(name, func(t *testing.T) {
			root, a, _ := install(t)
			if tc.noExe {
				os.Remove(filepath.Join(root, "bin", "radiant_modtools.exe"))
			}
			if tc.mtplugins != "" {
				withMTPlugins(t, root, tc.mtplugins)
			}
			dir := bridge(t)
			if tc.noBridge {
				dir = t.TempDir()
			}
			m := radianttest.NewMachine()
			m.OnStart = tc.onStart
			c, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
			defer cancel()
			_, err := radiant.Open(c, m, radiant.OpenOptions{Root: root, Map: a, Bridge: dir})
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("err = %v, want %q", err, tc.want)
			}
		})
	}
}

func TestOpenNamesTheDialogThatHoldsUpTheLoad(t *testing.T) {
	root, a, b := install(t)
	f := radianttest.New(5, b)
	f.Handle("reload", func(f *radianttest.Fake, _ string, _ []byte) radianttest.Reply {
		f.Dialog = "Map has errors"
		return radianttest.OK("", nil)
	})
	c, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	_, err := radiant.Open(c, radianttest.NewMachine(f), radiant.OpenOptions{Root: root, Map: a})
	if err == nil || !strings.Contains(err.Error(), `the dialog "Map has errors" is open over the load`) {
		t.Errorf("err = %v", err)
	}
}

func TestOpenPassesOnARefusedReload(t *testing.T) {
	root, a, b := install(t)
	f := radianttest.New(5, b)
	f.Handle("reload", func(*radianttest.Fake, string, []byte) radianttest.Reply {
		return radianttest.Err("busy", "a mouse button is held")
	})
	if _, err := open(t, radianttest.NewMachine(f), radiant.OpenOptions{Root: root, Map: a}); !radiant.IsCode(err, "busy") {
		t.Errorf("err = %v", err)
	}
}

func TestConnectFindsTheOnlyRadiantOrSaysThereIsNone(t *testing.T) {
	if _, err := radiant.Connect(ctx(t), radianttest.NewMachine(), 0); err != radiant.ErrNoRadiant {
		t.Errorf("no Radiant: err = %v", err)
	}
	c, err := radiant.Connect(ctx(t), radianttest.NewMachine(radianttest.New(8, "")), 0)
	if err != nil || c.Hello.PID != 8 {
		t.Fatalf("one Radiant: %v", err)
	}
	c.Close()
	if _, err := radiant.Connect(ctx(t), radianttest.NewMachine(radianttest.New(8, "")), 9); err == nil || !strings.Contains(err.Error(), "pid 9") {
		t.Errorf("wrong pid: err = %v", err)
	}
}

func TestListSkipsARadiantThatWillNotTalk(t *testing.T) {
	mute := radianttest.New(3, "")
	mute.Handle("hello", func(*radianttest.Fake, string, []byte) radianttest.Reply { return radianttest.Err("version", "2") })
	busy := radianttest.New(4, "")
	busy.Handle("state", func(*radianttest.Fake, string, []byte) radianttest.Reply { return radianttest.Err("busy", "x") })
	ok := radianttest.New(5, `C:\a.map`)
	insts, err := radiant.List(ctx(t), radianttest.NewMachine(mute, busy, ok))
	if err != nil || len(insts) != 1 || insts[0].PID != 5 || insts[0].State.Map != `C:\a.map` {
		t.Errorf("list = %+v, %v", insts, err)
	}
}
