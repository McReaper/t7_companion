package level

import (
	"math"

	"github.com/McReaper/t7_companion/internal/format/mapfile"
	"github.com/McReaper/t7_companion/internal/level/geom"
)

// Pose is a 3D camera: where it stands and its angles in degrees, Radiant's
// way (positive pitch looks up, yaw 90 faces +y).
type Pose struct {
	Origin           mapfile.Vec3
	Pitch, Yaw, Roll float64
}

// basis is the camera's forward, right and up vectors (roll aside: the map
// tools never roll).
func (p Pose) basis() (fwd, right, up mapfile.Vec3) {
	pr, yr := p.Pitch*math.Pi/180, p.Yaw*math.Pi/180
	fwd = mapfile.Vec3{math.Cos(pr) * math.Cos(yr), math.Cos(pr) * math.Sin(yr), math.Sin(pr)}
	right = mapfile.Vec3{math.Sin(yr), -math.Cos(yr), 0}
	return fwd, right, geom.Cross(right, fwd)
}

// camSpace is a point in the camera's frame: across, up, and ahead.
func (p Pose) camSpace(v mapfile.Vec3) (x, y, z float64) {
	fwd, right, up := p.basis()
	d := geom.Sub(v, p.Origin)
	return geom.Dot(d, right), geom.Dot(d, up), geom.Dot(d, fwd)
}

// Lens is a 3D view's pinhole projection: its size and its focal length in
// pixels. Radiant's 3D view projects px = w/2 + f·x/z, py = h/2 − f·y/z.
type Lens struct {
	W, H int
	F    float64
}

// Project is where a point lands in the view, false behind the camera.
func (l Lens) Project(p Pose, v mapfile.Vec3) (px, py float64, ok bool) {
	x, y, z := p.camSpace(v)
	if z <= 1e-6 {
		return 0, 0, false
	}
	return float64(l.W)/2 + l.F*x/z, float64(l.H)/2 - l.F*y/z, true
}

// FocalFrom recovers the focal length from where Radiant projected points:
// the point farthest off the view's centre tells it best. 0 when no point
// does.
func FocalFrom(w, h int, p Pose, pts []mapfile.Vec3, at [][2]float64) float64 {
	best, f := 0.0, 0.0
	for i, v := range pts {
		if i >= len(at) || math.IsNaN(at[i][0]) {
			continue
		}
		x, y, z := p.camSpace(v)
		if z <= 1e-6 {
			continue
		}
		if ax := math.Abs(x / z); ax > best {
			best, f = ax, (at[i][0]-float64(w)/2)*z/x
		}
		if ay := math.Abs(y / z); ay > best {
			best, f = ay, (float64(h)/2-at[i][1])*z/y
		}
	}
	return f
}

// Looking is the angle map_view frames a target from: from the south-west,
// above, looking down at it.
var Looking = Pose{Pitch: -35, Yaw: 45}

// margin is how much room a framed target leaves around it.
const margin = 1.15

// FrameBox is the pose, looking the way look does at the box's centre, from
// which its corners just fill the view, margin around: a corner x across, y
// up and z ahead of the centre fits at a distance d once |x|·f/(w/2) ≤ d+z,
// and so for y; and it stays ahead of the camera.
func FrameBox(l Lens, look Pose, b geom.Box) Pose {
	fwd, right, up := look.basis()
	c := b.Centre()
	d := 1.0
	for _, k := range b.Corners() {
		p := geom.Sub(k, c)
		x, y, z := geom.Dot(p, right), geom.Dot(p, up), geom.Dot(p, fwd)
		d = math.Max(d, math.Max(math.Max(math.Abs(x)*l.F/(float64(l.W)/2), math.Abs(y)*l.F/(float64(l.H)/2)), 1)-z)
	}
	d *= margin
	look.Origin = mapfile.Vec3{c[0] - fwd[0]*d, c[1] - fwd[1]*d, c[2] - fwd[2]*d}
	return look
}

// Axis is one of the 2D view's axes.
type Axis struct {
	Name   string // as the bridge names it: xy, xz, yz
	Layout string // as state names it: XY (Top), XZ, YZ
	U, V   int    // the world axes across and up the view
}

// The 2D view's axes, in the order View/Layout/Next turns it.
var Axes = []Axis{{"xy", "XY (Top)", 0, 1}, {"xz", "XZ", 0, 2}, {"yz", "YZ", 1, 2}}

// AxisNamed is the axis of a bridge or state name, false when none is.
func AxisNamed(s string) (int, bool) {
	for i, a := range Axes {
		if s == a.Name || s == a.Layout || (a.Name == "xy" && s == "XY") {
			return i, true
		}
	}
	return 0, false
}

// NextPresses is how many View/Layout/Next turn the 2D view from one axis to
// another.
func NextPresses(from, to int) int { return ((to-from)%len(Axes) + len(Axes)) % len(Axes) }

// ZoomStep is what one XY Zoom In multiplies the 2D view's scale by; XY 100%
// sets it to 1.
const ZoomStep = 1.25

// ZoomSteps is how many XY Zoom In (negative: Zoom Out) from 100% make the
// box fit a w×h view along the axis.
func ZoomSteps(a Axis, w, h int, b geom.Box) int {
	bw := math.Max(b.Max[a.U]-b.Min[a.U], 1) * margin
	bh := math.Max(b.Max[a.V]-b.Min[a.V], 1) * margin
	s := math.Min(float64(w)/bw, float64(h)/bh)
	return int(math.Floor(math.Log(s) / math.Log(ZoomStep)))
}

// StepsTo is how many steps from 100% give a scale, the nearest whole number.
func StepsTo(scale float64) int {
	if scale <= 0 {
		return 0
	}
	return int(math.Round(math.Log(scale) / math.Log(ZoomStep)))
}
