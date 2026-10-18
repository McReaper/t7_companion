package level

import (
	"cmp"
	"fmt"
	"math"

	"github.com/McReaper/t7_companion/internal/format/mapfile"
	"github.com/McReaper/t7_companion/internal/level/geom"
)

// MaterialInfo is what CheckTextures needs to know of a material.
type MaterialInfo struct {
	Known  bool // a GDT defines it
	Opaque bool // its faces hide what is behind them: a wall, a floor, caulk, sky
	Drawn  bool // its faces draw in game (not caulk, clip, trigger and other tools)
	Tile   Tile // its own tile, for how far a face's departs from it
}

// Finding is one texture problem of a face.
type Finding struct {
	Kind     string // hidden, seam, stretched, unknown
	ID       string // the brush's short id
	Face     string // the face's direction: +x … -z
	Material string
	Detail   string // hidden: "against <id> <face>" or "inside <id>"; seam: the other face, "<id> <face>"; stretched: "<tile>, the material's <tile>"
}

// cface is a brush face as the checks see it.
type cface struct {
	bi       int    // its brush's index among the map's world brushes
	fi       int    // its index among its brush's faces
	brush    string // GUID
	dir      string
	material string
	tex      [6]float64
	pl       geom.Plane     // its normal out of the brush
	pts      []mapfile.Vec3 // its corners
	poly     []geom.P2      // the same in the plane's 2D coordinates, a convex hull
	centre   mapfile.Vec3   // its corners' mean: on it, inside it
	box      geom.Box
}

// solid is a brush as the checks see it: its box and faces.
type solid struct {
	box   geom.Box
	faces []*cface
}

// checker holds a map's world brushes for CheckTextures.
type checker struct {
	short   func(guid string) string // a brush's id in a finding
	info    func(string) MaterialInfo
	faces   []*cface
	byPlane map[geom.PlaneKey][]*cface
	solids  []*solid          // by brush index
	hid     map[*cface]string // what hides a face, once found
}

// CheckTextures looks at the world brushes' faces (all of them, or those of
// the brushes and entities ids names), in the map's order, for what draws
// badly: a drawn face
// hidden by an opaque brush (caulk it), touching coplanar faces of a material
// whose tiles don't meet (a seam), a tile stretched from the material's
// shape, a material no GDT defines.
func CheckTextures(snap *Snapshot, ids []string, info func(material string) MaterialInfo) ([]Finding, error) {
	only, err := guidSet(snap, ids)
	if err != nil {
		return nil, err
	}
	c := newChecker(snap.IDs.Short, info)
	for _, e := range snap.Map.Entities {
		if e.Class() == "worldspawn" { // what the compiler draws and seals with: a brush entity's brushes move or don't draw
			for _, b := range e.Brushes() {
				c.add(b)
			}
		}
	}
	checked := func(g string) bool { return only == nil || only[hexOf(g)] }
	var out []Finding
	for _, f := range c.faces {
		if checked(f.brush) {
			out = append(out, c.check(f, checked)...)
		}
	}
	return out, nil
}

func newChecker(short func(string) string, info func(string) MaterialInfo) *checker {
	return &checker{short: short, info: info, byPlane: map[geom.PlaneKey][]*cface{}, hid: map[*cface]string{}}
}

// check is one face's findings.
func (c *checker) check(f *cface, checked func(string) bool) []Finding {
	fi := c.info(f.material)
	if !fi.Known {
		return []Finding{c.finding(f, "unknown", "")}
	}
	by := c.hidden(f)
	switch {
	case by != "" && fi.Drawn:
		return []Finding{c.finding(f, "hidden", by)}
	case by != "": // nothing of it shows: neither a stretch nor a seam
		return nil
	}
	var out []Finding
	if s := stretch(f, fi.Tile); s != "" && fi.Drawn {
		out = append(out, c.finding(f, "stretched", s))
	}
	if g := c.seamWith(f); g != nil && (!checked(g.brush) || f.bi < g.bi) { // a pair once
		out = append(out, c.finding(f, "seam", c.short(g.brush)+" "+g.dir))
	}
	for _, g := range c.zfightsWith(f) {
		if fi.Drawn && fi.Opaque && (!checked(g.brush) || f.bi < g.bi) { // a pair once
			out = append(out, c.finding(f, "zfight", c.short(g.brush)+" "+g.dir))
		}
	}
	return out
}

func (c *checker) finding(f *cface, kind, detail string) Finding {
	return Finding{Kind: kind, ID: c.short(f.brush), Face: f.dir, Material: f.material, Detail: detail}
}

// add takes a classic brush's faces with their corners; a face with no area
// is left out.
func (c *checker) add(b *mapfile.Brush) {
	s := &solid{}
	c.solids = append(c.solids, s)
	if b.Kind() != mapfile.KindFaces {
		return
	}
	fs, err := b.Faces()
	if err != nil {
		return
	}
	planes := make([]geom.Plane, len(fs))
	for i, f := range fs {
		planes[i] = geom.PlaneOf(f.Points)
	}
	for i, f := range fs {
		n := planes[i].N
		fc := &cface{bi: len(c.solids) - 1, fi: i, brush: b.Guid(), material: f.Material, tex: f.Tex, pl: geom.Plane{N: mapfile.Vec3{-n[0], -n[1], -n[2]}, D: -planes[i].D}}
		fc.dir, _, _ = texAxes(fc.pl.N)
		fc.pts = faceVertices(planes, i)
		if fc.poly = geom.ConvexHull(geom.Flatten(fc.pts, geom.Dominant(fc.pl.N))); len(fc.poly) < 3 {
			continue
		}
		for _, p := range fc.pts {
			fc.box.Add(p)
			fc.centre = mapfile.Vec3{fc.centre[0] + p[0], fc.centre[1] + p[1], fc.centre[2] + p[2]}
		}
		k := float64(len(fc.pts))
		fc.centre = mapfile.Vec3{fc.centre[0] / k, fc.centre[1] / k, fc.centre[2] / k}
		s.box.Union(fc.box)
		s.faces = append(s.faces, fc)
		c.faces = append(c.faces, fc)
		c.byPlane[geom.KeyOf(fc.pl)] = append(c.byPlane[geom.KeyOf(fc.pl)], fc)
	}
}

// hidden is what hides a face, "" when it can be seen; found once.
func (c *checker) hidden(f *cface) string {
	by, ok := c.hid[f]
	if !ok {
		by = c.hiddenBy(f)
		c.hid[f] = by
	}
	return by
}

// hiddenBy names what hides a face: an opaque brush's face against it, on
// its plane, facing it, that covers it whole, or several that do together;
// or an opaque brush it lies buried in. "" when it can be seen.
func (c *checker) hiddenBy(f *cface) string {
	back := geom.Plane{N: mapfile.Vec3{-f.pl.N[0], -f.pl.N[1], -f.pl.N[2]}, D: -f.pl.D}
	var against [][]geom.P2
	var first *cface
	for _, g := range c.byPlane[geom.KeyOf(back)] {
		switch {
		case g.bi == f.bi || !c.info(g.material).Opaque:
		case geom.Covers(g.poly, f.poly):
			return "against " + c.short(g.brush) + " " + g.dir
		case geom.Touch(g.poly, f.poly):
			against = append(against, g.poly)
			first = cmp.Or(first, g)
		}
	}
	if len(against) > 1 && geom.CoveredBy(against, f.poly) {
		return fmt.Sprintf("against %s %s and %d more", c.short(first.brush), first.dir, len(against)-1)
	}
	for i, s := range c.solids {
		if i != f.bi && s.box.Overlaps(f.box) && c.buriedIn(f, s) {
			return "inside " + c.short(s.faces[0].brush)
		}
	}
	return ""
}

// buriedIn tells whether a face lies inside a brush of opaque faces: its
// corners in or on the brush, its centre off every one of the brush's planes.
func (c *checker) buriedIn(f *cface, s *solid) bool {
	for _, g := range s.faces {
		if !c.info(g.material).Opaque || geom.Dot(g.pl.N, f.centre)-g.pl.D > -geom.Eps {
			return false
		}
		for _, p := range f.pts {
			if geom.Dot(g.pl.N, p)-g.pl.D > geom.Eps {
				return false
			}
		}
	}
	return len(s.faces) >= 4
}

// zfightsWith are the faces of other brushes on this one's plane, facing
// its way, opaque, drawn and seen, that overlap it and draw otherwise (another
// material, or tiles that don't meet): the two draw over each other and
// flicker in game (z-fighting), as a floor laid inside another floor does.
// Two faces drawing the same texels show nothing; a decal or a window is
// meant to lie on a wall: not opaque, it is left out.
func (c *checker) zfightsWith(f *cface) []*cface {
	var out []*cface
	for _, g := range c.byPlane[geom.KeyOf(f.pl)] {
		gi := c.info(g.material)
		differ := g.material != f.material || !sameTiles(f.tex, g.tex)
		if g.bi != f.bi && differ && gi.Opaque && gi.Drawn && geom.Overlapping(f.poly, g.poly) && c.hidden(g) == "" {
			out = append(out, g)
		}
	}
	return out
}

// seamWith is a face of another brush touching this one on its plane, of the
// same material, whose tiles don't continue this one's, and which shows.
func (c *checker) seamWith(f *cface) *cface {
	for _, g := range c.byPlane[geom.KeyOf(f.pl)] {
		if g.bi != f.bi && g.material == f.material && !sameTiles(f.tex, g.tex) && geom.Touch(f.poly, g.poly) && c.hidden(g) == "" {
			return g
		}
	}
	return nil
}

// sameTiles tells whether two faces on a plane lay a texture on the same
// tiles: the same sizes and rotation, offsets the same modulo a tile.
func sameTiles(a, b [6]float64) bool {
	if math.Abs(a[0]-b[0]) > 1e-3 || math.Abs(a[1]-b[1]) > 1e-3 || wrap(a[4]-b[4], 360) > 1e-3 && 360-wrap(a[4]-b[4], 360) > 1e-3 {
		return false
	}
	for i := 2; i < 4; i++ {
		s := math.Abs(a[i-2])
		if d := wrap(a[i]-b[i], s); d > 1e-3 && s-d > 1e-3 {
			return false
		}
	}
	return true
}

// stretch says how far a face's tile departs from its material's shape,
// "" within a half.
func stretch(f *cface, own Tile) string {
	w, h := math.Abs(f.tex[0]), math.Abs(f.tex[1])
	if w == 0 || h == 0 || own.W <= 0 || own.H <= 0 {
		return ""
	}
	r := (w / h) / (own.W / own.H)
	if r >= 1/1.5-1e-9 && r <= 1.5+1e-9 { // half again longer, and no more, is no stretch
		return ""
	}
	return num(w) + "x" + num(h) + ", the material's " + num(own.W) + "x" + num(own.H)
}

// guidSet is the GUIDs ids names (an entity's brushes with it), nil for all.
func guidSet(snap *Snapshot, ids []string) (map[string]bool, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	boxes, err := boxesOf(snap, ids)
	if err != nil {
		return nil, err
	}
	out := map[string]bool{}
	for g := range boxes {
		out[hexOf(g)] = true
	}
	return out, nil
}
