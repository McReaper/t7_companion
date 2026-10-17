package level

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/McReaper/t7_companion/internal/format/mapfile"
	"github.com/McReaper/t7_companion/internal/level/geom"
	"github.com/McReaper/t7_companion/internal/radiant"
)

// nextAxis turns the 2D view to its next axis: XY, XZ, YZ, then XY again.
const nextAxis = "command View/Layout/Next (XY, YZ, XY)"

// view2D shoots a 2D view along the axis, turned there when it shows another
// (turning centres it on the camera), framed on the target (its zoom set from
// 100% in steps, then centred on the target through the camera, as zooming
// moves the centre) or as it is. The user's view is read first, on the axis
// it shows, and put back in that order: axis, zoom, centre.
func (s *session) view2D(ctx context.Context, axis int, target *geom.Box, pts []mapfile.Vec3) (*frame, error) {
	a := Axes[axis]
	first := pts
	if target != nil {
		first = nil // this shot only reads the view
	}
	user, err := shoot(ctx, s.c, a.Name, nil, first)
	shown, turned := shownAxis(err)
	switch {
	case turned:
		user, err = shoot(ctx, s.c, Axes[shown].Name, nil, nil)
	case err == nil && target == nil:
		return user, nil
	default:
		shown = axis
	}
	if err != nil {
		return nil, err
	}
	cam, err := s.camera(ctx)
	if err != nil {
		return nil, err
	}
	back := append(append(turns(axis, shown), zoomLines(StepsTo(user.Scale))...), centreLines(user.Centre, cam)...)
	s.restores = append(s.restores, restore{"2D view's axis, zoom and centre", back})
	lines := turns(shown, axis)
	if target != nil {
		lines = append(append(lines, zoomLines(ZoomSteps(a, user.W, user.H, *target))...), centreLines(target.Centre(), cam)...)
	}
	for _, line := range lines {
		if err := s.call(ctx, line); err != nil {
			return nil, err
		}
	}
	return shoot(ctx, s.c, a.Name, nil, pts)
}

// turns are the View/Layout/Next that turn the 2D view from one axis to another.
func turns(from, to int) []string {
	var out []string
	for range NextPresses(from, to) {
		out = append(out, nextAxis)
	}
	return out
}

// shownAxis reads which axis the 2D view shows from the bridge's not-found
// "the 2D view shows <axis>: …".
func shownAxis(err error) (int, bool) {
	var e *radiant.Error
	if !errors.As(err, &e) || e.Code != "not-found" {
		return 0, false
	}
	rest, ok := strings.CutPrefix(e.Message, "the 2D view shows ")
	if !ok {
		return 0, false
	}
	name, _, _ := strings.Cut(rest, ":")
	return AxisNamed(strings.TrimSpace(name))
}

// camera reads the 3D camera.
func (s *session) camera(ctx context.Context) (Pose, error) {
	r, err := s.c.Call(ctx, "camera", nil)
	if err != nil {
		return Pose{}, err
	}
	f := strings.Fields(r.Summary)
	if len(f) != 6 {
		return Pose{}, fmt.Errorf("the camera is %q, not x y z pitch yaw roll", r.Summary)
	}
	o, ok1 := geom.ParseVec3(strings.Join(f[:3], " "))
	a, ok2 := geom.ParseVec3(strings.Join(f[3:], " "))
	if !ok1 || !ok2 {
		return Pose{}, fmt.Errorf("the camera is %q, not x y z pitch yaw roll", r.Summary)
	}
	return Pose{Origin: o, Pitch: a[0], Yaw: a[1], Roll: a[2]}, nil
}

// zoomLines set the 2D view's zoom to 100% then n steps in (out when n < 0).
func zoomLines(n int) []string {
	out := []string{"command View/Zoom/XY 100%"}
	step := "command View/Zoom/XY Zoom In"
	if n < 0 {
		step, n = "command View/Zoom/XY Zoom Out", -n
	}
	for range n {
		out = append(out, step)
	}
	return out
}

// centreLines centre the 2D view on a point: the camera goes there, the view
// centres on it, the camera comes back.
func centreLines(c mapfile.Vec3, cam Pose) []string {
	at := cam
	at.Origin = c
	return []string{"camera " + poseArgs(at), "action actionCenter_2D_On_Camera trigger", "camera " + poseArgs(cam)}
}

func poseArgs(p Pose) string {
	return strings.Join([]string{num(p.Origin[0]), num(p.Origin[1]), num(p.Origin[2]), num(p.Pitch), num(p.Yaw), num(p.Roll)}, " ")
}

// DefaultSize is an image's longer side unless asked otherwise.
const DefaultSize = 768

// compose scales the frame to the size asked and writes the ids whose points
// landed in it, nearest the centre first.
func compose(f *frame, view string, pts []labelPoint, size int) (*Shot, error) {
	img, err := FromBGRA(f.Pixels, f.W, f.H)
	if err != nil {
		return nil, err
	}
	if size <= 0 {
		size = DefaultSize
	}
	w, h := Fit(f.W, f.H, size)
	img = Shrink(img, w, h)
	k := float64(w) / float64(f.W)
	shot := &Shot{View: view, Image: img}
	if view == "3d" {
		p := f.Pose
		shot.Pose = &p
	} else {
		c := f.Centre
		shot.Centre, shot.Scale = &c, f.Scale*k
	}
	for i, p := range pts {
		at := f.At[i]
		x, y := at[0]*k, at[1]*k
		if math.IsNaN(x) || x < 0 || y < 0 || x >= float64(w) || y >= float64(h) {
			continue
		}
		shot.Labels = append(shot.Labels, Label{ID: p.id, X: int(x), Y: int(y)})
	}
	cx, cy := w/2, h/2
	sort.SliceStable(shot.Labels, func(i, j int) bool {
		a, b := shot.Labels[i], shot.Labels[j]
		return (a.X-cx)*(a.X-cx)+(a.Y-cy)*(a.Y-cy) < (b.X-cx)*(b.X-cx)+(b.Y-cy)*(b.Y-cy)
	})
	if len(shot.Labels) > maxLabels {
		shot.Labels = shot.Labels[:maxLabels]
	}
	DrawLabels(img, shot.Labels)
	return shot, nil
}
