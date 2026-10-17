package level

import (
	"math"
	"testing"

	"github.com/McReaper/t7_companion/internal/format/mapfile"
	"github.com/McReaper/t7_companion/internal/level/geom"
)

// turnedProbe is a probe's box in its own axes: its yaw, its reach each way
// from its origin, and its corners on the ground.
type turnedProbe struct {
	yaw     float64
	min     mapfile.Vec3 // size_min
	max     mapfile.Vec3 // size_max
	corners [4][2]float64
}

func inItsAxes(t *testing.T, p madeProbe) turnedProbe {
	t.Helper()
	a, _ := geom.ParseVec3(p.keys["size_min"])
	b, _ := geom.ParseVec3(p.keys["size_max"])
	tp := turnedProbe{min: a, max: b}
	if ang, ok := geom.ParseVec3(p.keys["angles"]); ok {
		tp.yaw = ang[1]
	}
	f := turnOf(tp.yaw)
	for k, c := range [4][2]float64{{-a[0], -a[1]}, {b[0], -a[1]}, {b[0], b[1]}, {-a[0], b[1]}} {
		x, y := f.world(c[0], c[1])
		tp.corners[k] = [2]float64{p.origin[0] + x, p.origin[1] + y}
	}
	return tp
}

// A corridor off the axes is one box turned with it, from wall face to wall
// face across, its corners in the corridor.
func TestProbesTurnAlongACorridorOffTheAxes(t *testing.T) {
	_, ps, err := probesRun(t, 981, `[
		{"walls": {"material": "brick", "z": [0, 112], "lines": [{"points": [[6000, 0], [6400, 400], [6320, 480], [5920, 80]], "closed": true}]}},
		{"box": {"min": [5900, -20, -16], "max": [6420, 500, 0], "material": "concrete"}},
		{"box": {"min": [5900, -20, 112], "max": [6420, 500, 128], "material": "concrete"}},
		{"probes": {"at": [6160, 240, 8]}}]`)
	if err != nil {
		t.Fatal(err)
	}
	if len(ps) != 1 {
		t.Fatalf("%d probes: %+v", len(ps), ps)
	}
	tp := inItsAxes(t, ps[0])
	if ps[0].keys["angles"] != "0 -45 0" {
		t.Errorf("angles %q: turned with the corridor", ps[0].keys["angles"])
	}
	across := 80*math.Sqrt2 - 8 // centre lines 80√2 apart, walls 8 thick; turned -45, the box's x runs across
	if w := tp.min[0] + tp.max[0]; w < across-0.5 || w > across+2 {
		t.Errorf("%g across, want wall face to wall face, %g (a unit into each wall at most)", w, across)
	}
	if l := tp.min[1] + tp.max[1]; l < 400 {
		t.Errorf("%g along, want the corridor's length", l)
	}
	for _, c := range tp.corners { // within the walls' centre lines: y - x from -6000 to -5840, x + y from 6000 to 6800
		if d, s := c[1]-c[0]+6000, c[0]+c[1]; d < -0.5 || d > 160.5 || s < 5999.5 || s > 6800.5 {
			t.Errorf("corner %v out of the corridor (%+v)", c, tp)
		}
	}
}

// A room with a corner cut off the axes: a box 48 deep turned along the cut,
// its face side on the wall; the rest boxed on the axes, none past the cut's
// middle.
func TestProbesTurnABandAlongACutCorner(t *testing.T) {
	_, ps, err := probesRun(t, 982, `[
		{"walls": {"material": "brick", "z": [0, 112], "lines": [{"points": [[6000, 0], [6512, 0], [6512, 128], [6384, 256], [6000, 256]], "closed": true}]}},
		{"box": {"min": [5996, -4, -16], "max": [6516, 260, 0], "material": "concrete"}},
		{"box": {"min": [5996, -4, 112], "max": [6516, 260, 128], "material": "concrete"}},
		{"probes": {"at": [6128, 128, 8]}}]`)
	if err != nil {
		t.Fatal(err)
	}
	var bands int
	for _, p := range ps {
		tp := inItsAxes(t, p)
		if tp.yaw == 0 {
			if p.hi[0]+p.hi[1] > 6640+0.5 {
				t.Errorf("box %v-%v past the cut's middle (x + y = 6640)", p.lo, p.hi)
			}
			continue
		}
		bands++
		if tp.yaw != -45 {
			t.Errorf("yaw %g, want -45", tp.yaw)
		}
		depth, along := tp.min[1]+tp.max[1], tp.min[0]+tp.max[0]
		if tp.min[0]+tp.max[0] < tp.min[1]+tp.max[1] {
			depth, along = along, depth
		}
		if math.Abs(depth-48) > 0.5 || along < 120 {
			t.Errorf("band %g deep, %g along (%+v): want 48, along the cut", depth, along, tp)
		}
		for _, c := range tp.corners {
			if c[0]+c[1] > 6640-4*math.Sqrt2+0.5 {
				t.Errorf("corner %v past the cut's face (%+v)", c, tp)
			}
		}
	}
	if bands != 1 || len(ps) < 2 {
		t.Errorf("%d turned of %d probes, want the cut's band and boxes on the axes", bands, len(ps))
	}
}

// A turn maps the ground both ways; the world's is the identity.
func TestTurnMapsBothWays(t *testing.T) {
	f := turnOf(30)
	u, v := f.local(10, 20)
	if x, y := f.world(u, v); math.Abs(x-10) > 1e-9 || math.Abs(y-20) > 1e-9 {
		t.Errorf("back %g %g", x, y)
	}
	if u, v := f.local(1, 0); math.Abs(u-math.Cos(math.Pi/6)) > 1e-9 || math.Abs(v+0.5) > 1e-9 {
		t.Errorf("local of x: %g %g", u, v)
	}
	if u, v := (turn{}).local(3, 4); u != 3 || v != 4 {
		t.Errorf("the world's turn: %g %g", u, v)
	}
	bx := probeBox{f: turnOf(90), lo: mapfile.Vec3{0, 0, 0}, hi: mapfile.Vec3{10, 20, 5}}
	if lo, hi := bx.extentIn(turn{}); math.Abs(lo[0]+20) > 1e-9 || math.Abs(hi[0]) > 1e-9 || math.Abs(lo[1]) > 1e-9 || math.Abs(hi[1]-10) > 1e-9 {
		t.Errorf("a quarter-turned box in the world: %v %v", lo, hi)
	}
}

// A side meeting a thin wall at an angle stops in it: at most at the middle
// of the wall where the wall is nearest, so the face beyond (another room's)
// stays out of the probe and its blend, though the farthest hit is deeper.
func TestProbeSideStopsInsideASlantedWall(t *testing.T) {
	a, b := geom.P2{X: 92 - 8.0/6, Y: -8}, geom.P2{X: 92 + 104.0/6, Y: 104} // the west face: x = 92 + y/6
	n := geom.P2{X: 112, Y: -(b.X - a.X)}
	l := math.Hypot(n.X, n.Y)
	off := geom.P2{X: n.X / l * 8, Y: n.Y / l * 8}
	poly := []geom.P2{a, {X: a.X + off.X, Y: a.Y + off.Y}, {X: b.X + off.X, Y: b.Y + off.Y}, b}
	m, err := mapfile.Parse([]byte(EntityText("worldspawn", nil, prismBrush(poly, 0, 112, "brick", texNumbers(64, 64)))))
	if err != nil {
		t.Fatal(err)
	}
	s := &solids{tiles: map[[2]int][]int{}, ptiles: map[[2]int][]int{}}
	s.add(m.Entities[0].Brushes()[0], identity)
	cells := map[int]cellInfo{}
	g := &probeGrid{x0: -64, y0: -64, n: 64, m: 64, s: s, height: 384}
	for c := range g.n * g.m {
		cells[c] = cellInfo{0, 112, math.Inf(-1)}
	}
	boxes := []probeBox{{rlo: [2]float64{0, 0}, rhi: [2]float64{96, 96}, lo: mapfile.Vec3{0, 0, 0}, hi: mapfile.Vec3{96, 96, 112}, cells: cells}}
	g.snapSide(0, boxes, 0, 1)
	far := 94 + 8*l/112 // the wall's east face where it is nearest the box, across the sampled line y 12
	if hi := boxes[0].hi[0]; hi <= 96 || hi > far-3 {
		t.Errorf("east side at %g: into the wall, short of %g (its far face less a blend)", hi, far-3)
	}
}
