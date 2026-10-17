package level

import (
	"context"
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/McReaper/t7_companion/internal/format/mapfile"
	"github.com/McReaper/t7_companion/internal/level/camera"
	"github.com/McReaper/t7_companion/internal/level/geom"
	"github.com/McReaper/t7_companion/internal/radiant"
)

// frame is one capture's answer: the view's size and projection, its pixels,
// and where each point asked for landed (NaN behind the camera).
type frame struct {
	W, H   int
	Pose   camera.Pose  // a 3D view's camera
	Centre mapfile.Vec3 // a 2D view's centre
	Scale  float64      // a 2D view's pixels per unit
	Pixels []byte
	At     [][2]float64
}

// shoot captures a view ("3d", "xy", "xz" or "yz"), from pose when given
// (3D), projecting the points.
func shoot(ctx context.Context, c *radiant.Client, view string, pose *camera.Pose, pts []mapfile.Vec3) (*frame, error) {
	line := "capture " + view
	if pose != nil {
		line += fmt.Sprintf(" %s %s %s %s %s %s", num(pose.Origin[0]), num(pose.Origin[1]), num(pose.Origin[2]), num(pose.Pitch), num(pose.Yaw), num(pose.Roll))
	}
	var body strings.Builder
	for _, p := range pts {
		body.WriteString(num(p[0]) + " " + num(p[1]) + " " + num(p[2]) + "\n")
	}
	r, err := c.Call(ctx, line, []byte(body.String()))
	if err != nil {
		return nil, err
	}
	f, err := parseSummary(r.Summary)
	if err != nil {
		return nil, err
	}
	n := f.W * f.H * 4
	if len(r.Body) < n {
		return nil, fmt.Errorf("the capture's %d bytes are less than a %d×%d frame", len(r.Body), f.W, f.H)
	}
	f.Pixels = r.Body[:n]
	if rest := strings.TrimRight(string(r.Body[n:]), "\n"); rest != "" {
		for _, line := range strings.Split(rest, "\n") {
			if len(f.At) == len(pts) {
				break
			}
			f.At = append(f.At, parsePoint(line))
		}
	}
	if len(f.At) != len(pts) {
		return nil, fmt.Errorf("the capture projected %d of %d points", len(f.At), len(pts))
	}
	return f, nil
}

// parseSummary reads "w=… h=… origin=x,y,z angles=p,y,r …" (3D) or "w=…
// h=… centre=x,y,z scale=…" (2D).
func parseSummary(s string) (*frame, error) {
	f := &frame{}
	for _, kv := range strings.Fields(s) {
		k, v, _ := strings.Cut(kv, "=")
		switch k {
		case "w":
			f.W, _ = strconv.Atoi(v)
		case "h":
			f.H, _ = strconv.Atoi(v)
		case "origin":
			f.Pose.Origin, _ = geom.ParseVec3(strings.ReplaceAll(v, ",", " "))
		case "angles":
			a, _ := geom.ParseVec3(strings.ReplaceAll(v, ",", " "))
			f.Pose.Pitch, f.Pose.Yaw, f.Pose.Roll = a[0], a[1], a[2]
		case "centre":
			f.Centre, _ = geom.ParseVec3(strings.ReplaceAll(v, ",", " "))
		case "scale":
			f.Scale, _ = strconv.ParseFloat(v, 64)
		}
	}
	if f.W <= 0 || f.H <= 0 {
		return nil, fmt.Errorf("the capture's summary has no size: %q", s)
	}
	return f, nil
}

func parsePoint(line string) [2]float64 {
	f := strings.Fields(line)
	if len(f) != 2 {
		return [2]float64{math.NaN(), math.NaN()}
	}
	x, err1 := strconv.ParseFloat(f[0], 64)
	y, err2 := strconv.ParseFloat(f[1], 64)
	if err1 != nil || err2 != nil {
		return [2]float64{math.NaN(), math.NaN()}
	}
	return [2]float64{x, y}
}

// num writes a coordinate for a request: to a thousandth, no exponent.
func num(f float64) string {
	return strconv.FormatFloat(math.Round(f*1000)/1000+0, 'f', -1, 64)
}
