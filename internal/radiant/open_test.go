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

// install is a mod-tools root with Radiant, the bridge plugin and two maps.
func install(t *testing.T, plugin bool) (root, a, b string) {
	t.Helper()
	t.Cleanup(radiant.SetPollEvery(time.Millisecond))
	root = t.TempDir()
	files := []string{"bin/radiant_modtools.exe", "map_source/a.map", "map_source/zm/zm_b.map"}
	if plugin {
		files = append(files, "bin/plugins/T7RadiantBridge/T7RadiantBridge.dll")
	}
	for _, f := range files {
		p := filepath.Join(root, f)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root, filepath.Join(root, "map_source", "a.map"), filepath.Join(root, "map_source", "zm", "zm_b.map")
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
	root, a, b := install(t, true)
	other := radianttest.New(11, b)
	other.Modified = true
	has := radianttest.New(12, strings.ToUpper(a)) // Radiant's case, not ours
	got, err := open(t, radianttest.NewMachine(other, has), radiant.OpenOptions{Root: root, Map: a})
	if err != nil || got != (radiant.Opened{PID: 12, Map: strings.ToUpper(a), How: radiant.AlreadyOpen, Plugin: "1.0.0"}) {
		t.Errorf("open = %+v, %v", got, err)
	}
	if r := append(reloads(other), reloads(has)...); len(r) != 0 {
		t.Errorf("reloads = %q, want none", r)
	}
}

func TestOpenReloadsTheOnlyRadiant(t *testing.T) {
	root, a, b := install(t, true)
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
	root, a, b := install(t, true)
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
	root, a, b := install(t, true)
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
	root, a, b := install(t, true)
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
	root, _, _ := install(t, true)
	_, err := open(t, radianttest.NewMachine(), radiant.OpenOptions{Root: root, Map: filepath.Join(root, "map_source", "nope.map")})
	if err == nil || !strings.Contains(err.Error(), "no map at") {
		t.Errorf("err = %v", err)
	}
}

// starting is an OnStart that runs a Radiant whose bridge listens after a
// while and which then loads the map it was started on.
func starting(listen, load time.Duration) func(int, string, string, []string) *radianttest.Started {
	return func(pid int, _, _ string, args []string) *radianttest.Started {
		f := radianttest.New(pid, "")
		time.AfterFunc(load, func() { f.Update(func(f *radianttest.Fake) { f.Map = args[0] }) })
		return &radianttest.Started{Fake: f, ListenAfter: listen}
	}
}

func TestOpenStartsRadiantOnTheMapWhenNoneRuns(t *testing.T) {
	root, a, _ := install(t, true)
	m := radianttest.NewMachine()
	m.OnStart = starting(20*time.Millisecond, 40*time.Millisecond)
	got, err := open(t, m, radiant.OpenOptions{Root: root, Map: a})
	if err != nil || got.How != radiant.Launched || got.Map != a || got.PID == 0 {
		t.Errorf("open = %+v, %v", got, err)
	}
	bin := filepath.Join(root, "bin")
	if s := m.Starts(); len(s) != 1 || !slices.Equal(s[0], []string{filepath.Join(bin, "radiant_modtools.exe"), bin, a}) {
		t.Errorf("starts = %q, want Radiant from bin/ on the map", s)
	}
}

func TestOpenSaysWhyRadiantCouldNotStart(t *testing.T) {
	for name, tc := range map[string]struct {
		plugin  bool
		noExe   bool
		onStart func(int, string, string, []string) *radianttest.Started
		want    string
	}{
		"no plugin":  {want: "plugin is not installed"},
		"no Radiant": {plugin: true, noExe: true, want: "no Radiant at"},
		"exits":      {plugin: true, want: "exited before its bridge answered"},
		"never loads": {plugin: true, onStart: func(int, string, string, []string) *radianttest.Started {
			return &radianttest.Started{Fake: radianttest.New(0, ""), Silent: true}
		}, want: "its bridge never answered"},
		"map never opens": {plugin: true, onStart: starting(0, time.Hour), want: `the map Radiant has open is still ""`},
	} {
		t.Run(name, func(t *testing.T) {
			root, a, _ := install(t, tc.plugin)
			if tc.noExe {
				os.Remove(filepath.Join(root, "bin", "radiant_modtools.exe"))
			}
			m := radianttest.NewMachine()
			m.OnStart = tc.onStart
			c, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
			defer cancel()
			_, err := radiant.Open(c, m, radiant.OpenOptions{Root: root, Map: a})
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("err = %v, want %q", err, tc.want)
			}
		})
	}
}

func TestOpenNamesTheDialogThatHoldsUpTheLoad(t *testing.T) {
	root, a, b := install(t, true)
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
	root, a, b := install(t, true)
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
