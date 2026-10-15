package level

import (
	"context"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/McReaper/t7_companion/internal/radiant/radianttest"
)

// A bake switches the 3D view to real lighting, runs Radiant's bake, waits
// for the console line that ends it (not the ones before), shoots, and puts
// the user's render back.
func TestViewBakesThenShootsUnderRealLighting(t *testing.T) {
	for _, render := range []string{"", "real", "Real Lighting"} {
		f, c, snap := viewing(t, 401)
		f.Update(func(f *radianttest.Fake) { f.Render = "Fullbright" })
		shot, err := View(context.Background(), c, snap, ViewOptions{Bake: true, Render: render, Size: 160})
		if err != nil {
			t.Fatalf("render %q: %v", render, err)
		}
		if shot.Baked != "All Probes Done (0.02 s)" || f.Baked != 1 || shot.View != "3d" {
			t.Errorf("render %q: baked %q, %d bakes, view %s", render, shot.Baked, f.Baked, shot.View)
		}
		reqs := f.Requests()
		at := func(prefix string) int {
			return slices.IndexFunc(reqs, func(r string) bool { return strings.HasPrefix(r, prefix) })
		}
		real, sub, bake, capture := at("command View/Render Method/Real Lighting"), at("subscribe console"), at("bake"), at("capture")
		if real < 0 || real >= sub || sub >= bake || bake >= capture {
			t.Errorf("render %q: real lighting, subscribe, bake, then capture: %q", render, reqs)
		}
		if f.Render != "Fullbright" {
			t.Errorf("render %q: the user's render is left %q", render, f.Render)
		}
	}
}

func TestViewBakeRefusals(t *testing.T) {
	old := bakeTimeout
	t.Cleanup(func() { bakeTimeout = old })
	bakeTimeout = 200 * time.Millisecond
	for name, c := range map[string]struct {
		o     ViewOptions
		setup func(*radianttest.Fake)
		want  string
	}{
		"a 2D view":             {ViewOptions{View: "top", Bake: true}, nil, "a bake shows in the 3d view"},
		"another render":        {ViewOptions{Bake: true, Render: "fake"}, nil, `render "fake": a bake shows under real lighting`},
		"no end":                {ViewOptions{Bake: true}, func(f *radianttest.Fake) { f.BakeSilent = true }, `bake: Radiant said no "All Probes Done" in 200ms`},
		"the plugin refuses":    {ViewOptions{Bake: true}, func(f *radianttest.Fake) { f.BakeRefused = "Radiant faulted starting the bake" }, "Radiant bake: refused: Radiant faulted starting the bake"},
		"a dialog is open":      {ViewOptions{Bake: true}, func(f *radianttest.Fake) { f.Dialog = "Entity Info" }, "Radiant subscribe: busy: Entity Info"},
		"a render it can't set": {ViewOptions{Bake: true}, func(f *radianttest.Fake) { f.Render = "Fullbright"; f.Dialog = "Entity Info" }, "Entity Info"},
	} {
		f, cl, snap := viewing(t, 402)
		if c.setup != nil {
			f.Update(c.setup)
		}
		before := f.Render
		_, err := View(context.Background(), cl, snap, c.o)
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: %v, want %q", name, err, c.want)
		}
		if f.Update(func(f *radianttest.Fake) { f.Dialog = "" }); f.Render != before {
			t.Errorf("%s: render left %q, was %q", name, f.Render, before)
		}
	}
}

// The bridge closing while Radiant bakes ends the wait at once.
func TestViewBakeEndsWhenTheBridgeCloses(t *testing.T) {
	f, c, snap := viewing(t, 403)
	f.Update(func(f *radianttest.Fake) { f.BakeSilent = true })
	time.AfterFunc(100*time.Millisecond, c.Close)
	start := time.Now()
	_, err := View(context.Background(), c, snap, ViewOptions{Bake: true})
	if err == nil || !strings.Contains(err.Error(), "bake: the bridge closed before the bake ended") || time.Since(start) > 5*time.Second {
		t.Errorf("%v after %s", err, time.Since(start))
	}
}
