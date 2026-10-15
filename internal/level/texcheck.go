package level

import (
	"math"
	"slices"
	"sort"

	"github.com/McReaper/t7_companion/internal/format/mapfile"
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
	brush    string // GUID
	dir      string
	material string
	tex      [6]float64
	pl       plane          // its normal out of the brush
	pts      []mapfile.Vec3 // its corners
	poly     []pt2          // the same in the plane's 2D coordinates, a convex hull
	centre   mapfile.Vec3   // its corners' mean: on it, inside it
	box      Box
}

// solid is a brush as the checks see it: its box and faces.
type solid struct {
	box   Box
	faces []*cface
}

// pt2 is a point in a plane's 2D coordinates.
type pt2 struct{ x, y float64 }

// checker holds a map's world brushes for CheckTextures.
type checker struct {
	snap    *Snapshot
	info    func(string) MaterialInfo
	faces   []*cface
	byPlane map[planeKey][]*cface
	solids  []*solid          // by brush index
	hid     map[*cface]string // what hides a face, once found
}

// CheckTextures looks at the world brushes' faces (all of them, or those of
// the brushes and entities ids names) for what draws badly: a drawn face
// hidden by an opaque brush (caulk it), touching coplanar faces of a material
// whose tiles don't meet (a seam), a tile stretched from the material's
// shape, a material no GDT defines.
func CheckTextures(snap *Snapshot, ids []string, info func(material string) MaterialInfo) ([]Finding, error) {
	only, err := guidSet(snap, ids)
	if err != nil {
		return nil, err
	}
	c := &checker{snap: snap, info: info, byPlane: map[planeKey][]*cface{}, hid: map[*cface]string{}}
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
	sort.SliceStable(out, func(i, j int) bool { return out[i].Kind < out[j].Kind })
	return out, nil
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
		out = append(out, c.finding(f, "seam", c.snap.IDs.Short(g.brush)+" "+g.dir))
	}
	return out
}

func (c *checker) finding(f *cface, kind, detail string) Finding {
	return Finding{Kind: kind, ID: c.snap.IDs.Short(f.brush), Face: f.dir, Material: f.material, Detail: detail}
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
	planes := make([]plane, len(fs))
	for i, f := range fs {
		planes[i] = planeOf(f.Points)
	}
	for i, f := range fs {
		n := planes[i].n
		fc := &cface{bi: len(c.solids) - 1, brush: b.Guid(), material: f.Material, tex: f.Tex, pl: plane{mapfile.Vec3{-n[0], -n[1], -n[2]}, -planes[i].d}}
		fc.dir, _, _ = texAxes(fc.pl.n)
		fc.pts = faceVertices(planes, i)
		if fc.poly = convexHull(project(fc.pts, dominant(fc.pl.n))); len(fc.poly) < 3 {
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
		c.byPlane[keyOf(fc.pl)] = append(c.byPlane[keyOf(fc.pl)], fc)
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
// its plane, facing it, that covers it whole; or an opaque brush it lies
// buried in. "" when it can be seen.
func (c *checker) hiddenBy(f *cface) string {
	back := plane{mapfile.Vec3{-f.pl.n[0], -f.pl.n[1], -f.pl.n[2]}, -f.pl.d}
	for _, g := range c.byPlane[keyOf(back)] {
		if g.bi != f.bi && c.info(g.material).Opaque && covers(g.poly, f.poly) {
			return "against " + c.snap.IDs.Short(g.brush) + " " + g.dir
		}
	}
	for i, s := range c.solids {
		if i != f.bi && s.box.overlaps(f.box) && c.buriedIn(f, s) {
			return "inside " + c.snap.IDs.Short(s.faces[0].brush)
		}
	}
	return ""
}

// buriedIn tells whether a face lies inside a brush of opaque faces: its
// corners in or on the brush, its centre off every one of the brush's planes.
func (c *checker) buriedIn(f *cface, s *solid) bool {
	for _, g := range s.faces {
		if !c.info(g.material).Opaque || dot(g.pl.n, f.centre)-g.pl.d > -eps {
			return false
		}
		for _, p := range f.pts {
			if dot(g.pl.n, p)-g.pl.d > eps {
				return false
			}
		}
	}
	return len(s.faces) >= 4
}

// seamWith is a face of another brush touching this one on its plane, of the
// same material, whose tiles don't continue this one's, and which shows.
func (c *checker) seamWith(f *cface) *cface {
	for _, g := range c.byPlane[keyOf(f.pl)] {
		if g.bi != f.bi && g.material == f.material && !sameTiles(f.tex, g.tex) && touch(f.poly, g.poly) && c.hidden(g) == "" {
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
	if r > 1/1.5 && r < 1.5 {
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

// planeKey is a plane rounded, so faces on one plane share it.
type planeKey [4]int64

func keyOf(p plane) planeKey {
	q := func(x float64) int64 { return int64(math.Round(x * 1000)) }
	return planeKey{q(p.n[0]), q(p.n[1]), q(p.n[2]), int64(math.Round(p.d * 10))}
}

// dominant is the axis a normal is closest to, as texAxes picks it.
func dominant(n mapfile.Vec3) int {
	ax, ay, az := math.Abs(n[0]), math.Abs(n[1]), math.Abs(n[2])
	switch {
	case az >= ax && az >= ay:
		return 2
	case ax >= ay:
		return 0
	}
	return 1
}

// project drops an axis of the points.
func project(pts []mapfile.Vec3, axis int) []pt2 {
	out := make([]pt2, len(pts))
	u, v := (axis+1)%3, (axis+2)%3
	for i, p := range pts {
		out[i] = pt2{p[u], p[v]}
	}
	return out
}

// convexHull is the points' hull, counter-clockwise (Andrew's monotone chain).
func convexHull(pts []pt2) []pt2 {
	if len(pts) < 3 {
		return nil
	}
	pts = append([]pt2(nil), pts...)
	sort.Slice(pts, func(i, j int) bool { return pts[i].x < pts[j].x || pts[i].x == pts[j].x && pts[i].y < pts[j].y })
	var h []pt2
	for pass := 0; pass < 2; pass++ {
		start := len(h)
		for _, p := range pts {
			for len(h) >= start+2 && cross2(h[len(h)-2], h[len(h)-1], p) <= 1e-9 {
				h = h[:len(h)-1]
			}
			h = append(h, p)
		}
		h = h[:len(h)-1]
		slices.Reverse(pts)
	}
	return h
}

func cross2(o, a, b pt2) float64 { return (a.x-o.x)*(b.y-o.y) - (a.y-o.y)*(b.x-o.x) }

// inHull tells whether a point is inside or on a counter-clockwise hull.
func inHull(h []pt2, p pt2) bool {
	for i := range h {
		a, b := h[i], h[(i+1)%len(h)]
		if cross2(a, b, p) < -eps*math.Hypot(b.x-a.x, b.y-a.y) {
			return false
		}
	}
	return true
}

// covers tells whether hull a holds every corner of hull b.
func covers(a, b []pt2) bool {
	for _, p := range b {
		if !inHull(a, p) {
			return false
		}
	}
	return true
}

// touch tells whether two hulls overlap or share a stretch of edge, where a
// seam would show: a corner of one strictly inside the other, edges that
// cross, or edges along one line that overlap. Hulls that meet at a corner
// alone don't.
func touch(a, b []pt2) bool {
	for _, p := range a {
		if strictlyIn(b, p) {
			return true
		}
	}
	for _, p := range b {
		if strictlyIn(a, p) {
			return true
		}
	}
	for i := range a {
		p, q := a[i], a[(i+1)%len(a)]
		for j := range b {
			if r, s := b[j], b[(j+1)%len(b)]; properlyCross(p, q, r, s) || alongside(p, q, r, s) {
				return true
			}
		}
	}
	return false
}

// side is where a point lies off a segment's line, in units: positive on its
// left, within eps of 0 on it.
func side(p, q, r pt2) float64 {
	return cross2(p, q, r) / math.Max(math.Hypot(q.x-p.x, q.y-p.y), 1e-12)
}

// strictlyIn tells whether a point is inside a counter-clockwise hull, off
// its edges.
func strictlyIn(h []pt2, p pt2) bool {
	for i := range h {
		if side(h[i], h[(i+1)%len(h)], p) <= eps {
			return false
		}
	}
	return true
}

// properlyCross tells whether two segments cross each other inside both.
func properlyCross(p, q, r, s pt2) bool {
	d1, d2, d3, d4 := side(r, s, p), side(r, s, q), side(p, q, r), side(p, q, s)
	return (d1 > eps && d2 < -eps || d1 < -eps && d2 > eps) && (d3 > eps && d4 < -eps || d3 < -eps && d4 > eps)
}

// alongside tells whether two segments lie on one line and overlap by more
// than a point.
func alongside(p, q, r, s pt2) bool {
	if math.Abs(side(p, q, r)) > eps || math.Abs(side(p, q, s)) > eps {
		return false
	}
	l := math.Hypot(q.x-p.x, q.y-p.y)
	at := func(x pt2) float64 { return ((x.x-p.x)*(q.x-p.x) + (x.y-p.y)*(q.y-p.y)) / math.Max(l, 1e-12) }
	lo, hi := math.Max(0, math.Min(at(r), at(s))), math.Min(l, math.Max(at(r), at(s)))
	return hi-lo > eps
}

// overlaps tells whether two boxes share a point, within eps.
func (b Box) overlaps(o Box) bool {
	for i := range 3 {
		if b.Max[i] < o.Min[i]-eps || o.Max[i] < b.Min[i]-eps {
			return false
		}
	}
	return b.Ok && o.Ok
}
