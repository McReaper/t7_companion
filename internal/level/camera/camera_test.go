package camera

import (
	"math"
	"testing"

	"github.com/McReaper/t7_companion/internal/format/mapfile"
	"github.com/McReaper/t7_companion/internal/level/geom"
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

func box(min, max mapfile.Vec3) geom.Box { return geom.Box{Min: min, Max: max, Ok: true} }

func TestTheFocalComesFromThePointFarthestOffCentre(t *testing.T) {
	l := Lens{W: 200, H: 100, F: 150}
	pose := Pose{Origin: mapfile.Vec3{0, 0, 0}, Yaw: 90} // looking +y
	up := mapfile.Vec3{0, 100, 30}                       // straight above the centre: only y tells f
	_, py, _ := l.Project(pose, up)
	if f := FocalFrom(200, 100, pose, []mapfile.Vec3{up}, [][2]float64{{100, py}}); math.Abs(f-150) > 1e-6 {
		t.Errorf("focal from a point above = %v", f)
	}
	// a point a little off centre, projected wrongly, must not be the one used
	near, far := mapfile.Vec3{1, 100, 0}, mapfile.Vec3{40, 100, 0}
	fx, _, _ := l.Project(pose, far)
	if f := FocalFrom(200, 100, pose, []mapfile.Vec3{near, far}, [][2]float64{{150, 50}, {fx, 50}}); math.Abs(f-150) > 1e-6 {
		t.Errorf("focal = %v: taken from the near point", f)
	}
	// a point far off but little off centre, projected wrongly, is not the one used: it is the
	// least off centre (x/z), though the farthest (x·z)
	deep, wide := mapfile.Vec3{10, 1000, 0}, mapfile.Vec3{40, 100, 0}
	wx, _, _ := l.Project(pose, wide)
	if f := FocalFrom(200, 100, pose, []mapfile.Vec3{deep, wide}, [][2]float64{{190, 50}, {wx, 50}}); math.Abs(f-150) > 1e-6 {
		t.Errorf("focal = %v: taken from the deep point", f)
	}
	deepUp, high := mapfile.Vec3{0, 1000, 10}, mapfile.Vec3{0, 100, 40}
	_, hy, _ := l.Project(pose, high)
	if f := FocalFrom(200, 100, pose, []mapfile.Vec3{deepUp, high}, [][2]float64{{100, 5}, {100, hy}}); math.Abs(f-150) > 1e-6 {
		t.Errorf("focal = %v: taken from the deep point above", f)
	}
	// fewer projections than points
	if f := FocalFrom(200, 100, pose, []mapfile.Vec3{far, near}, [][2]float64{{fx, 50}}); math.Abs(f-150) > 1e-6 {
		t.Errorf("focal with a projection missing = %v", f)
	}
}

func TestZoomFitsATallBox(t *testing.T) {
	tall := box(mapfile.Vec3{0, 0, 0}, mapfile.Vec3{100, 0, 1000}) // front view: 100 across, 1000 up
	// a 400×400 view: 400/1150 = 0.348 px/unit, 5 steps out from 100% (0.328)
	if n := ZoomSteps(Axes[1], 400, 400, tall); n != -5 {
		t.Errorf("ZoomSteps(front, tall) = %d, want -5", n)
	}
}

// What Radiant's 3D view (1648×869) projected for points around
// test_claude.map's floor, the camera at 0 -600 200, pitch -15, yaw 90.
var (
	measuredPose = Pose{Origin: mapfile.Vec3{0, -600, 200}, Pitch: -15, Yaw: 90}
	measuredPts  = []mapfile.Vec3{{0, 0, -16}, {-216, -232, -16}, {248, -232, -16}, {248, 256, -16}, {-216, 256, -16}, {0, 400, -16}}
	measuredAt   = [][2]float64{{824.00006, 536.29205}, {187.34277, 768.729}, {1554.9769, 768.729}, {1164.6436, 416.7685}, {527.31055, 416.7685}, {824.00006, 374.958}}
)

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
		if !Fits(at, l.W, l.H) {
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
		if fwd, _, _ := p.Basis(); geom.Dot(geom.Sub(b.Centre(), p.Origin), fwd) <= 0 {
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
		if got := Fits(tc.at, 100, 50); got != tc.want {
			t.Errorf("%s: fits = %v", name, got)
		}
	}
}

// above is straight down, +y up the image: at -90 Radiant drops the yaw.
var above = Pose{Pitch: -89, Yaw: 90}

func TestFramingFromAboveFramesAFlatBoxAwayFromTheOrigin(t *testing.T) {
	l := Lens{W: 1648, H: 869, F: 1212.4}
	for _, b := range []geom.Box{
		box(mapfile.Vec3{100, -40, 500}, mapfile.Vec3{1748, 60, 508}),     // as wide as the view's aspect
		box(mapfile.Vec3{-40, 200, 500}, mapfile.Vec3{60, 1069, 508}),     // as deep
		box(mapfile.Vec3{3000, 3000, 900}, mapfile.Vec3{3800, 3500, 908}), // far off
	} {
		p := FrameBox(l, above, b)
		x, y, ok := l.Project(p, b.Centre())
		if !ok || math.Abs(x-824) > 1 || math.Abs(y-434.5) > 1 {
			t.Errorf("%v: its middle at %v, %v", b, x, y)
		}
		var lo, hi [2]float64
		for i, c := range b.Corners() {
			cx, cy, _ := l.Project(p, c)
			if i == 0 {
				lo, hi = [2]float64{cx, cy}, [2]float64{cx, cy}
			}
			lo, hi = [2]float64{math.Min(lo[0], cx), math.Min(lo[1], cy)}, [2]float64{math.Max(hi[0], cx), math.Max(hi[1], cy)}
		}
		if fill := math.Max((hi[0]-lo[0])/1648, (hi[1]-lo[1])/869); fill < 0.85 || fill > 1 {
			t.Errorf("%v fills %.2f of the view", b, fill)
		}
	}
}
