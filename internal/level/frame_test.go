package level

import (
	"math"
	"testing"

	"github.com/McReaper/t7_companion/internal/format/mapfile"
	"github.com/McReaper/t7_companion/internal/level/geom"
)

// What Radiant's 3D view (1648×869) projected for points around
// test_claude.map's floor, the camera at 0 -600 200, pitch -15, yaw 90.
var (
	measuredPose = Pose{Origin: mapfile.Vec3{0, -600, 200}, Pitch: -15, Yaw: 90}
	measuredPts  = []mapfile.Vec3{{0, 0, -16}, {-216, -232, -16}, {248, -232, -16}, {248, 256, -16}, {-216, 256, -16}, {0, 400, -16}}
	measuredAt   = [][2]float64{{824.00006, 536.29205}, {187.34277, 768.729}, {1554.9769, 768.729}, {1164.6436, 416.7685}, {527.31055, 416.7685}, {824.00006, 374.958}}
)

func TestTheLensProjectsAsRadiantDid(t *testing.T) {
	l := Lens{W: 1648, H: 869, F: 1212.4}
	for i, p := range measuredPts {
		x, y, ok := l.Project(measuredPose, p)
		if !ok || math.Abs(x-measuredAt[i][0]) > 0.5 || math.Abs(y-measuredAt[i][1]) > 0.5 {
			t.Errorf("%v: (%.2f, %.2f), Radiant drew it at %v", p, x, y, measuredAt[i])
		}
	}
	if _, _, ok := l.Project(measuredPose, mapfile.Vec3{0, -700, 200}); ok {
		t.Error("a point behind the camera is projected")
	}
	if f := FocalFrom(1648, 869, measuredPose, measuredPts, measuredAt); math.Abs(f-1212.4) > 0.5 {
		t.Errorf("focal from Radiant's projections = %.2f, want 1212.4", f)
	}
	nan := [][2]float64{{math.NaN(), math.NaN()}}
	if f := FocalFrom(1648, 869, measuredPose, measuredPts[:1], nan); f != 0 {
		t.Errorf("focal from a point behind = %v", f)
	}
	if f := FocalFrom(1648, 869, measuredPose, []mapfile.Vec3{{0, -700, 200}}, [][2]float64{{1, 1}}); f != 0 {
		t.Errorf("focal from a point behind the camera = %v", f)
	}
}

func TestFrameBoxFitsTheBoxInTheView(t *testing.T) {
	l := Lens{W: 1648, H: 869, F: 1212.4}
	for _, b := range []geom.Box{
		box(mapfile.Vec3{-216, -232, -64}, mapfile.Vec3{248, 256, -16}),
		box(mapfile.Vec3{0, 0, 0}, mapfile.Vec3{2048, 64, 512}),
		box(mapfile.Vec3{10, 10, 10}, mapfile.Vec3{10, 10, 10}).Grown(256), // a point, as View grows it
		box(mapfile.Vec3{0, 0, 0}, mapfile.Vec3{2048, 2048, 64}),           // deep along the line of sight: its near corner far nearer than its centre
	} {
		p := FrameBox(l, Looking, b)
		if p.Pitch != Looking.Pitch || p.Yaw != Looking.Yaw {
			t.Errorf("framed %v from %+v", b, p)
		}
		var at [][2]float64
		for _, c := range b.Corners() {
			x, y, _ := l.Project(p, c)
			at = append(at, [2]float64{x, y})
		}
		if !fits(at, l.W, l.H) {
			t.Errorf("%v framed from %+v lands at %v", b, p, at)
		}
		// the corners fill it, margin aside: one of them lands within a margin of an edge
		edge := 1.0
		for _, a := range at {
			edge = math.Min(edge, math.Min(math.Min(a[0], float64(l.W)-a[0])/float64(l.W), math.Min(a[1], float64(l.H)-a[1])/float64(l.H)))
		}
		if edge > 0.1 { // a sphere framed it 0.2 to 0.3 in
			t.Errorf("%v framed from %+v: the nearest corner is %.3f of the view from its edge", b, p, edge)
		}
		if fwd, _, _ := p.basis(); geom.Dot(geom.Sub(b.Centre(), p.Origin), fwd) <= 0 {
			t.Errorf("the camera looks away from %v", b)
		}
	}
}

func TestFits(t *testing.T) {
	for name, tc := range map[string]struct {
		at   [][2]float64
		want bool
	}{
		"fills it":       {[][2]float64{{10, 10}, {90, 40}}, true},
		"wide only":      {[][2]float64{{10, 25}, {90, 26}}, true},
		"too small":      {[][2]float64{{40, 20}, {50, 30}}, false},
		"off the left":   {[][2]float64{{-1, 10}, {90, 40}}, false},
		"off the bottom": {[][2]float64{{10, 10}, {90, 51}}, false},
		"behind":         {[][2]float64{{10, 10}, {math.NaN(), math.NaN()}}, false},
	} {
		if got := fits(tc.at, 100, 50); got != tc.want {
			t.Errorf("%s: fits = %v", name, got)
		}
	}
}

func TestTheTwoDViewsAxesAndZoom(t *testing.T) {
	for s, want := range map[string]int{"xy": 0, "XY (Top)": 0, "XY": 0, "xz": 1, "XZ": 1, "yz": 2, "YZ": 2} {
		if got, ok := AxisNamed(s); !ok || got != want {
			t.Errorf("AxisNamed(%q) = %d, %v", s, got, ok)
		}
	}
	if _, ok := AxisNamed("3d"); ok {
		t.Error("3d is a 2D axis")
	}
	for _, tc := range [][3]int{{0, 0, 0}, {0, 1, 1}, {0, 2, 2}, {2, 0, 1}, {1, 0, 2}, {2, 1, 2}} {
		if got := NextPresses(tc[0], tc[1]); got != tc[2] {
			t.Errorf("NextPresses(%d, %d) = %d, want %d", tc[0], tc[1], got, tc[2])
		}
	}
	// a 693×1165 view: 464 units across at 1.15 margin want 1.30 px/unit, one step in
	floor := box(mapfile.Vec3{-216, -232, -64}, mapfile.Vec3{248, 256, -16})
	if n := ZoomSteps(Axes[0], 693, 1165, floor); n != 1 {
		t.Errorf("ZoomSteps(top, floor) = %d, want 1", n)
	}
	if n := ZoomSteps(Axes[1], 693, 1165, box(mapfile.Vec3{0, 0, 0}, mapfile.Vec3{8192, 0, 64})); n != -12 {
		t.Errorf("ZoomSteps(front, 8192 wide) = %d, want -12", n)
	}
	if n := ZoomSteps(Axes[2], 693, 1165, box(mapfile.Vec3{0, 0, 0}, mapfile.Vec3{0, 0, 0}).Grown(256)); n != 3 {
		t.Errorf("ZoomSteps(side, a point grown to 256) = %d, want 3", n)
	}
	if g := box(mapfile.Vec3{0, 0, 0}, mapfile.Vec3{512, 10, 0}).Grown(256); g != box(mapfile.Vec3{0, -123, -128}, mapfile.Vec3{512, 133, 128}) {
		t.Errorf("Grown = %+v", g)
	}
	for scale, want := range map[float64]int{1: 0, 1.25: 1, 1.5625: 2, 0.8: -1, 0.64: -2, 1.3: 1, 0: 0, -1: 0} {
		if got := StepsTo(scale); got != want {
			t.Errorf("StepsTo(%v) = %d, want %d", scale, got, want)
		}
	}
}
