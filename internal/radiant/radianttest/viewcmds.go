package radianttest

import (
	"bytes"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
)

// The fake's views: a pinhole 3D view and one 2D view, as PROTOCOL.md's
// capture describes them. Its zoom moves the 2D view's centre, as Radiant's
// does (towards the mouse).
type views struct {
	Camera   [6]float64 // x y z pitch yaw roll
	W3, H3   int
	Focal    float64 // the 3D view's focal length in pixels
	W2, H2   int
	Centre2D [3]float64
	Scale2D  float64
}

// layouts are the 2D view's axes in View/Layout/Next's order, as state names them.
var layouts = []string{"XY (Top)", "XZ", "YZ"}

var axisOf = map[string]int{"xy": 0, "xz": 1, "yz": 2}

// axes are each 2D axis's world axes across and up.
var axes = [3][2]int{{0, 1}, {0, 2}, {1, 2}}

func camera(f *Fake, args string, _ []byte) Reply {
	if v := strings.Fields(args); len(v) > 0 {
		c, ok := floats(v, 6)
		if !ok {
			return Err("bad-request", "camera [<x> <y> <z> <pitch> <yaw> <roll>]")
		}
		copy(f.Camera[:], c)
	}
	return OK(join(f.Camera[:], " "), nil)
}

func capture(f *Fake, args string, body []byte) Reply {
	w := strings.Fields(args)
	if len(w) == 0 {
		return Err("bad-request", "capture 3d|xy|xz|yz [<x> <y> <z> <pitch> <yaw> <roll>]")
	}
	if f.Minimised {
		return Err("minimised", "Radiant is minimised: it draws nothing")
	}
	var pts [][3]float64
	for _, line := range strings.Split(strings.TrimSpace(string(body)), "\n") {
		if p, ok := floats(strings.Fields(line), 3); ok {
			pts = append(pts, [3]float64{p[0], p[1], p[2]})
		}
	}
	if w[0] == "3d" {
		return f.capture3D(w[1:], pts)
	}
	axis, ok := axisOf[w[0]]
	switch {
	case !ok:
		return Err("bad-request", "capture 3d|xy|xz|yz")
	case layouts[axis] != f.View2D:
		return Err("not-found", "the 2D view shows "+f.View2D+": command View/Layout/Next (XY, YZ, XY) turns it XY, XZ, YZ")
	}
	u, v := axes[axis][0], axes[axis][1]
	out := f.frame(f.W2, f.H2)
	for _, p := range pts {
		out = fmt.Appendf(out, "%s %s\n", num(float64(f.W2)/2+(p[u]-f.Centre2D[u])*f.Scale2D), num(float64(f.H2)/2-(p[v]-f.Centre2D[v])*f.Scale2D))
	}
	return OK(fmt.Sprintf("w=%d h=%d centre=%s scale=%s", f.W2, f.H2, join(f.Centre2D[:], ","), num(f.Scale2D)), out)
}

func (f *Fake) capture3D(pose []string, pts [][3]float64) Reply {
	cam := f.Camera
	if len(pose) > 0 {
		c, ok := floats(pose, 6)
		if !ok {
			return Err("bad-request", "capture 3d [<x> <y> <z> <pitch> <yaw> <roll>]")
		}
		copy(cam[:], c)
	}
	pr, yr := cam[3]*math.Pi/180, cam[4]*math.Pi/180
	fwd := [3]float64{math.Cos(pr) * math.Cos(yr), math.Cos(pr) * math.Sin(yr), math.Sin(pr)}
	right := [3]float64{math.Sin(yr), -math.Cos(yr), 0}
	up := [3]float64{right[1]*fwd[2] - right[2]*fwd[1], right[2]*fwd[0] - right[0]*fwd[2], right[0]*fwd[1] - right[1]*fwd[0]}
	out := f.frame(f.W3, f.H3)
	for _, p := range pts {
		d := [3]float64{p[0] - cam[0], p[1] - cam[1], p[2] - cam[2]}
		x, y, z := dot3(d, right), dot3(d, up), dot3(d, fwd)
		if z <= 1e-6 {
			out = append(out, "-\n"...)
			continue
		}
		out = fmt.Appendf(out, "%s %s\n", num(float64(f.W3)/2+f.Focal*x/z), num(float64(f.H3)/2-f.Focal*y/z))
	}
	return OK(fmt.Sprintf("w=%d h=%d origin=%s angles=%s game_view=0", f.W3, f.H3, join(cam[:3], ","), join(cam[3:], ",")), out)
}

// command runs the menu entries the map tools use on views.
func command(f *Fake, args string, _ []byte) Reply {
	path := strings.TrimSpace(args)
	switch {
	case path == "View/Zoom/XY Zoom In":
		f.Scale2D *= 1.25
		f.Centre2D[0] += 10 // towards the mouse
	case path == "View/Zoom/XY Zoom Out":
		f.Scale2D /= 1.25
		f.Centre2D[0] -= 10
	case path == "View/Zoom/XY 100%":
		f.Scale2D = 1
	case path == "View/Layout/Next (XY, YZ, XY)": // the next axis, centred on the camera, as Radiant does
		for i, l := range layouts {
			if l == f.View2D {
				f.View2D = layouts[(i+1)%len(layouts)]
				break
			}
		}
		copy(f.Centre2D[:], f.Camera[:3])
	case strings.HasPrefix(path, "View/Render Method/"):
		f.Render = strings.TrimPrefix(path, "View/Render Method/")
	case path == "View/Hide Entities/Hide Selected": // what is hidden is no longer selected
		f.Hidden, f.Selection = append([]string(nil), f.Selection...), nil
	case path == "View/Show Last Hidden":
		f.Hidden = nil
	case path == "Edit/Undo":
		if len(f.history) == 0 {
			return Err("disabled", "Edit/Undo is disabled")
		}
		u := f.history[len(f.history)-1]
		f.history = f.history[:len(f.history)-1]
		f.Text, f.Modified, f.UndoLabel = u.text, u.modified, u.label
	default:
		return Err("missing", "no menu entry "+path)
	}
	return OK("", nil)
}

// action triggers the actions the map tools use.
func action(f *Fake, args string, _ []byte) Reply {
	if strings.TrimSpace(args) != "actionCenter_2D_On_Camera trigger" {
		return Err("not-found", "no action named "+args)
	}
	copy(f.Centre2D[:], f.Camera[:3])
	return OK("checkable=0 checked=0 enabled=1", nil)
}

// undoStep is the map as an edit found it.
type undoStep struct {
	text     string
	modified bool
	label    string
}

// frame is a capture's pixels: white when the fake's views draw nothing.
func (f *Fake) frame(w, h int) []byte {
	if f.Blank {
		return bytes.Repeat([]byte{255}, w*h*4)
	}
	return pixels(w, h)
}

// pixels is a w×h BGRA frame: a gradient, so a scaled copy can be told from the original.
func pixels(w, h int) []byte {
	out := make([]byte, 0, w*h*4)
	for y := range h {
		for x := range w {
			out = append(out, byte(x), byte(y), 128, 255)
		}
	}
	return out
}

func floats(s []string, n int) ([]float64, bool) {
	if len(s) != n {
		return nil, false
	}
	out := make([]float64, n)
	for i, x := range s {
		v, err := strconv.ParseFloat(x, 64)
		if err != nil {
			return nil, false
		}
		out[i] = v
	}
	return out, true
}

func dot3(a, b [3]float64) float64 { return a[0]*b[0] + a[1]*b[1] + a[2]*b[2] }

func num(f float64) string { return strconv.FormatFloat(f, 'f', -1, 64) }

func join(v []float64, sep string) string {
	s := make([]string, len(v))
	for i, x := range v {
		s[i] = num(x)
	}
	return strings.Join(s, sep)
}

// bake starts Radiant's bake, refused unless the 3D view draws real lighting;
// as the plugin does, it answers at once and the console says when it ends,
// after lines of its own.
func bake(f *Fake, _ string, _ []byte) Reply {
	if f.Render != "Real Lighting" {
		return Err("refused", "the bake runs only under real lighting: command View/Render Method/Real Lighting")
	}
	if f.BakeRefused != "" {
		return Err("refused", f.BakeRefused)
	}
	f.Baked++
	if !f.BakeSilent {
		time.AfterFunc(20*time.Millisecond, func() {
			f.Event("console", "Sun bake Done")
			for _, l := range f.BakeConsole {
				f.Event("console", l)
			}
			f.Event("console", "Probe 1 of 1")
			f.Event("console", "All Probes Done (0.02 s)")
		})
	}
	return OK("", nil)
}
