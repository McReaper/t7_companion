package level

import (
	"strings"

	"github.com/McReaper/t7_companion/internal/format/mapfile"
	"github.com/McReaper/t7_companion/internal/level/geom"
)

// occluder is a brush a 3D view can't see through.
type occluder struct {
	owners []string // the ids a label of the brush or of its entity carries: its own brushes hide no label
	planes []geom.Plane
	box    geom.Box
}

// occluders are the brushes of the map a 3D shot sees as solid: every
// classic brush of the world and of brush entities, but a volume's or a
// trigger's and a clip's, which Radiant draws see-through, and the sky's when
// the shot hides it.
func occluders(snap *Snapshot, hide string) []occluder {
	sky := strings.Contains(strings.ToLower(hide), "sky")
	var out []occluder
	for _, e := range snap.Map.Entities {
		if class := strings.ToLower(e.Class()); strings.HasPrefix(class, "trigger_") || strings.Contains(class, "volume") {
			continue
		}
		for _, b := range e.Brushes() {
			if b.Kind() != mapfile.KindFaces || seeThrough(Material(b), sky) {
				continue
			}
			faces, err := b.Faces()
			box := geom.BrushBox(b)
			if err != nil || !box.Ok {
				continue
			}
			o := occluder{owners: []string{snap.IDs.Short(b.Guid()), snap.IDs.Short(e.Guid())}, box: box}
			for _, f := range faces {
				o.planes = append(o.planes, geom.PlaneOf(f.Points))
			}
			out = append(out, o)
		}
	}
	return out
}

// seeThrough tells whether Radiant draws a brush of that material see-through
// in the 3D view: tool materials but caulk, the sky when hidden.
func seeThrough(material string, skyHidden bool) bool {
	m := strings.ToLower(material)
	for _, tool := range []string{"clip", "trigger", "volume", "portal", "hint", "skip", "nodraw"} {
		if strings.Contains(m, tool) {
			return true
		}
	}
	return skyHidden && strings.Contains(m, "sky")
}

// occlusionMargin is how far before its point a brush must start to hide a
// label, in units: an entity standing on a floor or set in a wall's face is
// seen.
const occlusionMargin = 2

// inSight keeps the points no brush hides from the camera, with where each
// landed in the frame. A brush hides a point when the segment from the camera
// runs through it, entering it more than occlusionMargin before the point; a
// brush the camera is inside hides nothing, as Radiant draws no face seen
// from behind.
func inSight(occ []occluder, cam mapfile.Vec3, pts []labelPoint, at [][2]float64) ([]labelPoint, [][2]float64) {
	var keep []labelPoint
	var keepAt [][2]float64
	for i, p := range pts {
		seen := true
		for _, o := range occ {
			if !owns(o, p.id) && o.blocks(cam, p.p) {
				seen = false
				break
			}
		}
		if seen {
			keep, keepAt = append(keep, p), append(keepAt, at[i])
		}
	}
	return keep, keepAt
}

func owns(o occluder, id string) bool {
	return id == o.owners[0] || id == o.owners[1]
}

// blocks clips the segment a→b against the brush's planes (Cyrus–Beck).
func (o occluder) blocks(a, b mapfile.Vec3) bool {
	var seg geom.Box
	seg.Add(a)
	seg.Add(b)
	if !overlaps(seg, o.box) || geom.Inside(a, o.planes) {
		return false
	}
	d := geom.Sub(b, a)
	tIn, tOut := 0.0, 1.0
	for _, p := range o.planes {
		num, den := geom.Dot(p.N, a)-p.D, geom.Dot(p.N, d)
		switch {
		case den == 0 && num < 0:
			return false // parallel to the plane, outside it
		case den > 0:
			tIn = max(tIn, -num/den)
		case den < 0:
			tOut = min(tOut, -num/den)
		}
		if tIn > tOut {
			return false
		}
	}
	length := geom.Dot(d, d)
	if length == 0 {
		return false
	}
	return (1-tIn)*(1-tIn)*length > occlusionMargin*occlusionMargin
}

func overlaps(a, b geom.Box) bool {
	for i := range 3 {
		if a.Max[i] < b.Min[i] || b.Max[i] < a.Min[i] {
			return false
		}
	}
	return true
}
