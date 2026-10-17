package level

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/McReaper/t7_companion/internal/format/mapfile"
	"github.com/McReaper/t7_companion/internal/level/geom"
)

// solids are what a player collides with, as the batch leaves the map: the
// world's brushes, func_groups' and the misc_prefabs' (read from their
// files, placed by origin, angles and modelscale, nested ones too), and
// patches as floors. Not doors (script_brushmodel: they open), volumes,
// triggers, nonColliding or weaponClip-only brushes, nor clips the player
// passes (clip_ai, the bullet and missile clips). With them, the zones'
// volumes (player_volume), the play space.
type solids struct {
	planes  [][]geom.Plane
	boxes   []geom.Box
	patches [][3]mapfile.Vec3
	tiles   map[[2]int][]int // 128-unit tiles of the ground -> the solids over them
	ptiles  map[[2]int][]int // and the patch triangles
	bounds  geom.Box
	zones   [][]geom.Plane
}

const solidTile = 128

// passable are tool materials a player walks through.
var passable = []string{"trigger", "volume", "portal", "hint", "clip_ai", "clip_missile", "clip_weap", "clip_weapon", "ai_", "mantle",
	"umbra", "caulk_sun_shadow", "lightgrid", "outdoor", "foliage_clip_nosight"}

func blocks(br *mapfile.Brush) bool {
	c := " " + br.Contents() + " "
	if strings.Contains(c, " nonColliding ") || strings.Contains(c, "weaponClip") {
		return false
	}
	faces, err := br.Faces()
	if err != nil || len(faces) < 4 {
		return false
	}
	for _, f := range faces {
		m := strings.ToLower(f.Material)
		tool := false
		for _, p := range passable {
			if strings.HasPrefix(m, p) {
				tool = true
				break
			}
		}
		if !tool {
			return true
		}
	}
	return false
}

// xform places a prefab's contents: a point's world position is o + its
// coordinates along the prefab's forward, left and up, scaled.
type xform struct {
	axes  [3]mapfile.Vec3
	o     mapfile.Vec3
	scale float64
}

var identity = xform{axes: [3]mapfile.Vec3{{1, 0, 0}, {0, 1, 0}, {0, 0, 1}}, scale: 1}

func (x xform) apply(p mapfile.Vec3) mapfile.Vec3 {
	var out mapfile.Vec3
	for k := range 3 {
		out[k] = x.o[k] + (x.axes[0][k]*p[0]+x.axes[1][k]*p[1]+x.axes[2][k]*p[2])*x.scale
	}
	return out
}

// then is x placed inside outer.
func (x xform) then(outer xform) xform {
	var axes [3]mapfile.Vec3
	for i, a := range x.axes {
		for k := range 3 {
			axes[i][k] = outer.axes[0][k]*a[0] + outer.axes[1][k]*a[1] + outer.axes[2][k]*a[2]
		}
	}
	return xform{axes: axes, o: outer.apply(x.o), scale: x.scale * outer.scale}
}

// prefabXform is a misc_prefab's placement: angles pitch yaw roll as Radiant
// turns an entity (forward, left, up), origin, modelscale.
func prefabXform(e *mapfile.Entity) xform {
	o, _ := geom.Origin(e)
	v, _ := e.Key("angles")
	a, _ := geom.ParseVec3(v)
	s := 1.0
	if v, ok := e.Key("modelscale"); ok {
		if f, err := strconv.ParseFloat(v, 64); err == nil && f != 0 {
			s = f
		}
	}
	p, y, r := a[0]*math.Pi/180, a[1]*math.Pi/180, a[2]*math.Pi/180
	cp, sp, cy, sy, cr, sr := math.Cos(p), math.Sin(p), math.Cos(y), math.Sin(y), math.Cos(r), math.Sin(r)
	return xform{axes: [3]mapfile.Vec3{
		{cp * cy, cp * sy, -sp},
		{sr*sp*cy - cr*sy, sr*sp*sy + cr*cy, sr * cp},
		{cr*sp*cy + sr*sy, cr*sp*sy - sr*cy, cr * cp},
	}, o: o, scale: s}
}

// solidsOf gathers the batch's solids; prefab loads a prefab's map by its
// model path (nil: prefabs are left out, noted).
func (b *batch) solidsOf(prefab func(string) (*mapfile.Map, error)) (*solids, []string) {
	s := &solids{tiles: map[[2]int][]int{}, ptiles: map[[2]int][]int{}}
	var notes []string
	for _, u := range b.units {
		switch {
		case u.state == deleted:
		case u.world:
			s.add(u.brush, identity)
		case u.ent.Class() == "func_group":
			for _, br := range u.ent.Brushes() {
				s.add(br, identity)
			}
		case u.ent.Class() == "misc_prefab":
			notes = append(notes, s.addPrefab(u.ent, identity, prefab, 0)...)
		default:
			s.addZone(u.ent, identity)
		}
	}
	return s, notes
}

// addZone keeps a zone's volume, an info_volume that is a player_volume.
func (s *solids) addZone(e *mapfile.Entity, x xform) {
	if v, _ := e.Key("script_noteworthy"); e.Class() != "info_volume" || v != "player_volume" {
		return
	}
	for _, br := range e.Brushes() {
		if faces, err := br.Faces(); err == nil {
			planes := make([]geom.Plane, len(faces))
			for i, f := range faces {
				planes[i] = geom.PlaneOf([3]mapfile.Vec3{x.apply(f.Points[0]), x.apply(f.Points[1]), x.apply(f.Points[2])})
			}
			s.zones = append(s.zones, planes)
		}
	}
}

// inZone tells whether a point is inside a zone's volume.
func (s *solids) inZone(p mapfile.Vec3) bool {
	for _, z := range s.zones {
		if lo, hi, ok := geom.Column(z, p[0], p[1]); ok && p[2] >= lo-geom.Eps && p[2] <= hi+geom.Eps {
			return true
		}
	}
	return false
}

func (s *solids) addPrefab(e *mapfile.Entity, outer xform, prefab func(string) (*mapfile.Map, error), depth int) []string {
	model, _ := e.Key("model")
	if prefab == nil || depth > 8 {
		return []string{"prefab " + model + " left out: its file isn't read here"}
	}
	m, err := prefab(model)
	if err != nil {
		return []string{fmt.Sprintf("prefab %s left out: %v", model, err)}
	}
	x := prefabXform(e).then(outer)
	var notes []string
	for _, pe := range m.Entities {
		switch pe.Class() {
		case "worldspawn", "func_group":
			for _, br := range pe.Brushes() {
				s.add(br, x)
			}
		case "misc_prefab":
			notes = append(notes, s.addPrefab(pe, x, prefab, depth+1)...)
		default:
			s.addZone(pe, x)
		}
	}
	return notes
}

func (s *solids) add(br *mapfile.Brush, x xform) {
	switch br.Kind() {
	case mapfile.KindFaces:
		if !blocks(br) {
			return
		}
		faces, _ := br.Faces()
		planes := make([]geom.Plane, len(faces))
		for i, f := range faces {
			var pts [3]mapfile.Vec3
			for k := range 3 {
				pts[k] = x.apply(f.Points[k])
			}
			planes[i] = geom.PlaneOf(pts)
		}
		var bx geom.Box
		for _, v := range geom.Vertices(planes) {
			bx.Add(v)
		}
		if !bx.Ok {
			return
		}
		s.index(s.tiles, len(s.planes), bx)
		s.planes, s.boxes = append(s.planes, planes), append(s.boxes, bx)
		s.bounds.Union(bx)
	case mapfile.KindCurve, mapfile.KindMesh:
		p, err := br.Patch()
		if err != nil {
			return
		}
		for r := range max(len(p.Rows)-1, 0) {
			for c := range max(min(len(p.Rows[r]), len(p.Rows[r+1]))-1, 0) {
				a, b, d, e := x.apply(p.Rows[r][c].Pos), x.apply(p.Rows[r][c+1].Pos), x.apply(p.Rows[r+1][c].Pos), x.apply(p.Rows[r+1][c+1].Pos)
				for _, t := range [][3]mapfile.Vec3{{a, b, d}, {b, e, d}} {
					var bx geom.Box
					for _, v := range t {
						bx.Add(v)
					}
					s.index(s.ptiles, len(s.patches), bx)
					s.patches = append(s.patches, t)
				}
			}
		}
	}
}

func (s *solids) index(tiles map[[2]int][]int, k int, bx geom.Box) {
	for i := int(math.Floor(bx.Min[0] / solidTile)); i <= int(math.Floor(bx.Max[0]/solidTile)); i++ {
		for j := int(math.Floor(bx.Min[1] / solidTile)); j <= int(math.Floor(bx.Max[1]/solidTile)); j++ {
			tiles[[2]int{i, j}] = append(tiles[[2]int{i, j}], k)
		}
	}
}

// span is a stretch of a vertical line inside something solid.
type span struct{ lo, hi float64 }

// spans are the solid stretches of the vertical line through (x, y), bottom
// up, overlapping ones merged; a patch is a floor without thickness.
func (s *solids) spans(x, y float64) []span {
	t := [2]int{int(math.Floor(x / solidTile)), int(math.Floor(y / solidTile))}
	var out []span
	for _, k := range s.tiles[t] {
		if bx := s.boxes[k]; x < bx.Min[0]-geom.Eps || x > bx.Max[0]+geom.Eps || y < bx.Min[1]-geom.Eps || y > bx.Max[1]+geom.Eps {
			continue
		}
		if lo, hi, ok := geom.Column(s.planes[k], x, y); ok {
			out = append(out, span{lo, hi})
		}
	}
	for _, k := range s.ptiles[t] {
		if h, ok := geom.OnTriangle(s.patches[k], x, y); ok {
			out = append(out, span{h - 1, h})
		}
	}
	return mergeSpans(out)
}

func mergeSpans(in []span) []span {
	if len(in) == 0 {
		return nil
	}
	for i := 1; i < len(in); i++ { // insertion sort: a column crosses few solids
		for j := i; j > 0 && in[j].lo < in[j-1].lo; j-- {
			in[j], in[j-1] = in[j-1], in[j]
		}
	}
	out := []span{in[0]}
	for _, sp := range in[1:] {
		if last := &out[len(out)-1]; sp.lo <= last.hi+geom.Eps {
			last.hi = math.Max(last.hi, sp.hi)
		} else {
			out = append(out, sp)
		}
	}
	return out
}

// inside tells whether a point is inside a solid.
func (s *solids) inside(p mapfile.Vec3) bool {
	for _, sp := range s.spans(p[0], p[1]) {
		if p[2] > sp.lo+geom.Eps && p[2] < sp.hi-geom.Eps {
			return true
		}
	}
	return false
}
