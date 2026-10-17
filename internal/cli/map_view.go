package cli

import (
	"context"
	"fmt"
	"strings"

	"github.com/McReaper/t7_companion/internal/level"
	"github.com/McReaper/t7_companion/internal/level/camera"
	"github.com/McReaper/t7_companion/internal/level/geom"
	"github.com/McReaper/t7_companion/internal/level/picture"
	"github.com/McReaper/t7_companion/internal/radiant"
)

// mapViewArgs are what map_view shows and how.
type mapViewArgs struct {
	View     string
	IDs      []string
	At       string // "x y z"
	Radius   float64
	Camera   string // "x y z pitch yaw roll"
	Size     int
	Render   string
	Hide     string
	Bake     bool
	NoLabels bool
	PID      int
}

// mapViewOut is a map_view answer, beside its image.
type mapViewOut struct {
	Map    string            `json:"map"`
	View   string            `json:"view"` // 3d, xy (top), xz (front) or yz (side)
	W      int               `json:"w"`
	H      int               `json:"h"`
	Camera []float64         `json:"camera,omitempty"` // a 3D view's: x y z pitch yaw roll
	Centre []float64         `json:"centre,omitempty"` // a 2D view's
	Scale  float64           `json:"scale,omitempty"`  // a 2D view's image pixels per unit
	Labels map[string][2]int `json:"labels,omitempty"` // id: where its label's point is in the image
	Notes  []string          `json:"notes,omitempty"`
	Baked  string            `json:"baked,omitempty"`  // the bake's end, as Radiant's console said it
	Warned []string          `json:"warned,omitempty"` // what the bake's console warned of
}

// maxViewSize is the longest side an image may ask for: what an agent's
// vision takes in without scaling it down again.
const maxViewSize = 1568

// mapView shoots a view of the map open in Radiant: its answer and the PNG.
func mapView(ctx context.Context, a mapViewArgs) (*mapViewOut, []byte, error) {
	o := level.ViewOptions{View: a.View, IDs: a.IDs, Radius: a.Radius, Size: a.Size, Render: a.Render, Labels: !a.NoLabels, Hide: a.Hide, Bake: a.Bake}
	if a.Size < 0 || a.Size > maxViewSize {
		return nil, nil, fmt.Errorf("size %d: at most %d pixels", a.Size, maxViewSize)
	}
	if a.At != "" {
		v, ok := geom.ParseVec3(strings.ReplaceAll(a.At, ",", " "))
		if !ok {
			return nil, nil, fmt.Errorf("at %q: give a point as x y z", a.At)
		}
		o.At = &v
	}
	if a.Camera != "" {
		f := strings.Fields(strings.ReplaceAll(a.Camera, ",", " "))
		pos, ok1 := geom.ParseVec3(strings.Join(f[:min(3, len(f))], " "))
		ang, ok2 := geom.ParseVec3(strings.Join(f[min(3, len(f)):], " "))
		if !ok1 || !ok2 {
			return nil, nil, fmt.Errorf("camera %q: give x y z pitch yaw roll", a.Camera)
		}
		o.Camera = &camera.Pose{Origin: pos, Pitch: ang[0], Yaw: ang[1], Roll: ang[2]}
	}
	c, err := radiant.Connect(ctx, radiantMachine, a.PID)
	if err != nil {
		return nil, nil, err
	}
	defer c.Close()
	snap, err := level.Read(ctx, c)
	if err != nil {
		return nil, nil, err
	}
	shot, err := level.View(ctx, c, snap, o)
	if err != nil {
		return nil, nil, err
	}
	png, err := picture.PNG(shot.Image)
	if err != nil {
		return nil, nil, err
	}
	out := &mapViewOut{Map: baseName(snap.State.Map), View: shot.View, W: shot.Image.Rect.Dx(), H: shot.Image.Rect.Dy(), Notes: shot.Notes, Baked: shot.Baked, Warned: shot.Warned}
	if shot.Pose != nil {
		out.Camera = append(vec(shot.Pose.Origin), vec([3]float64{shot.Pose.Pitch, shot.Pose.Yaw, shot.Pose.Roll})...)
	}
	if shot.Centre != nil {
		out.Centre, out.Scale = vec(*shot.Centre), float64(int(shot.Scale*10000+0.5))/10000
	}
	for _, l := range shot.Labels {
		if out.Labels == nil {
			out.Labels = map[string][2]int{}
		}
		out.Labels[l.ID] = [2]int{l.X, l.Y}
	}
	return out, png, nil
}
