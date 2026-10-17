package level

import (
	"cmp"
	"fmt"
	"maps"
	"math"
	"slices"
	"strings"

	"github.com/McReaper/t7_companion/internal/format/mapfile"
	"github.com/McReaper/t7_companion/internal/level/geom"
)

// ZoneOp makes a zone of the zone manager's (_zm_zonemgr.gsc): an
// info_volume over each of its areas, the zone being all its volumes; riser
// spawners spread over the floors under it, the structs its first volume
// targets; and a node_pathnode on its floor, which the compiler grows the
// navmesh from where the floor isn't joined to the player's. Its links are
// the map script's add_adjacent_zone calls, which map_edit can't write: the
// answer lists them.
type ZoneOp struct {
	Name     string     `json:"name"`
	Areas    []ZoneArea `json:"areas"`
	Spawners *int       `json:"spawners,omitempty"` // none, 4
	Links    []ZoneLink `json:"links,omitempty"`
	Pathnode *bool      `json:"pathnode,omitempty"` // none, true
}

// ZoneArea is a space a zone covers: a box, an outline on the ground from z
// up height, or the outlines of floor brushes from their top up height.
type ZoneArea struct {
	Min     *[3]float64  `json:"min,omitempty"`
	Max     *[3]float64  `json:"max,omitempty"`
	Outline [][2]float64 `json:"outline,omitempty"`
	Z       float64      `json:"z,omitempty"`      // an outline's floor
	Height  float64      `json:"height,omitempty"` // an outline's or floors'; none, 128
	Floors  []string     `json:"floors,omitempty"`
}

// ZoneLink opens a zone on another: zombies and spawning follow players
// through once its flag is set.
type ZoneLink struct {
	Zone   string `json:"zone"`
	Flag   string `json:"flag,omitempty"` // none, always_on: open from the start (the template sets it)
	OneWay bool   `json:"one_way,omitempty"`
}

// Link is one add_adjacent_zone call the map's script needs.
type Link struct {
	A, B, Flag string
	OneWay     bool
}

// area is a zone's area as built: its outline on the ground and the convex
// pieces it is cut in, from z0 to z1.
type area struct {
	outline []geom.P2
	parts   [][]geom.P2
	z0, z1  float64
}

const (
	zoneHeight   = 128       // an area's default height above its floor
	stepHeight   = 18        // how far a floor may sit off an area's bottom: the navmesh's step
	spawnerSpace = 32        // the grid spawners are picked from
	roomArea     = 128 * 128 // an area this large gets a spawner of its own
	maxSpawners  = 64
)

// zone runs a zone op.
func (b *batch) zone(i int, o *ZoneOp) error {
	areas, err := b.zoneAreas(o)
	if err != nil {
		return fmt.Errorf("zone %s: %w", o.Name, err)
	}
	target := b.zoneTarget(o.Name)
	keys := map[string]string{"targetname": o.Name, "target": target, "script_noteworthy": "player_volume"}
	for _, a := range areas {
		var brushes []string
		for _, p := range a.parts {
			brushes = append(brushes, prismBrush(p, a.z0, a.z1, "volume", texNumbers(64, 64)))
		}
		if err := b.made(i, EntityText("info_volume", keys, brushes...), false); err != nil {
			return err
		}
		b.units[len(b.units)-1].part = o.Name + " volume"
	}
	if err := b.zoneEntities(i, o, areas, target); err != nil {
		return fmt.Errorf("zone %s: %w", o.Name, err)
	}
	for _, l := range o.Links {
		if !word(l.Zone) || l.Zone == o.Name || (l.Flag != "" && !word(l.Flag)) {
			return fmt.Errorf("zone %s: link %+v: another zone's name, and a flag of one word", o.Name, l)
		}
		b.links = append(b.links, Link{A: o.Name, B: l.Zone, Flag: cmp.Or(l.Flag, "always_on"), OneWay: l.OneWay})
	}
	b.zones = append(b.zones, madeZone{o.Name, areas})
	return nil
}

// zoneAreas checks a zone op and builds its areas.
func (b *batch) zoneAreas(o *ZoneOp) ([]area, error) {
	switch {
	case !word(o.Name) || strings.Contains(o.Name, "|"):
		return nil, fmt.Errorf("name %q: one word", o.Name)
	case len(o.Areas) == 0:
		return nil, fmt.Errorf("no areas: give each a box (min, max), an outline or floors")
	case o.Spawners != nil && (*o.Spawners < 0 || *o.Spawners > maxSpawners):
		return nil, fmt.Errorf("spawners %d: 0 to %d", *o.Spawners, maxSpawners)
	}
	var out []area
	for k, a := range o.Areas {
		got, err := b.areaOf(a)
		if err != nil {
			return nil, fmt.Errorf("area %d: %w", k, err)
		}
		out = append(out, got...)
	}
	return out, nil
}

// areaOf builds one area: a floor brush gives one each.
func (b *batch) areaOf(a ZoneArea) ([]area, error) {
	forms := 0
	for _, set := range []bool{a.Min != nil || a.Max != nil, len(a.Outline) > 0, len(a.Floors) > 0} {
		if set {
			forms++
		}
	}
	h := cmp.Or(a.Height, zoneHeight)
	switch {
	case forms != 1:
		return nil, fmt.Errorf("one of min and max (a box), outline, or floors")
	case h < 0:
		return nil, fmt.Errorf("height %v: above 0", a.Height)
	case len(a.Floors) > 0:
		return b.floorAreas(a.Floors, h)
	case len(a.Outline) > 0:
		poly, err := geom.OutlineOf(a.Outline)
		if err != nil {
			return nil, err
		}
		parts, err := geom.ConvexParts(poly)
		return []area{{poly, parts, a.Z, a.Z + h}}, err
	case a.Min == nil || a.Max == nil:
		return nil, fmt.Errorf("a box takes both min and max")
	}
	lo, hi := *a.Min, *a.Max
	for k := range 3 {
		if hi[k] <= lo[k] {
			return nil, fmt.Errorf("max %v must exceed min %v on every axis", hi, lo)
		}
	}
	rect := []geom.P2{{X: lo[0], Y: lo[1]}, {X: hi[0], Y: lo[1]}, {X: hi[0], Y: hi[1]}, {X: lo[0], Y: hi[1]}}
	return []area{{rect, [][]geom.P2{rect}, lo[2], hi[2]}}, nil
}

// floorAreas are the areas over floor brushes: each brush's outline seen
// from above, from the lowest point of its top to height over its highest.
func (b *batch) floorAreas(ids []string, h float64) ([]area, error) {
	var out []area
	for _, id := range ids {
		ts, err := b.targets(id)
		if err != nil {
			return nil, err
		}
		for _, t := range ts {
			br := t.br
			if t.u.world {
				br = t.u.brush
			}
			if br == nil || br.Kind() != mapfile.KindFaces {
				return nil, fmt.Errorf("%s: floors are brushes", id)
			}
			a, ok := floorArea(br, h)
			if !ok {
				return nil, fmt.Errorf("%s: a brush with no top to stand on", id)
			}
			out = append(out, a)
		}
	}
	return out, nil
}

// floorArea is the area over one brush: its corners seen from above, its
// top's height there.
func floorArea(br *mapfile.Brush, h float64) (area, bool) {
	faces, err := br.Faces()
	if err != nil {
		return area{}, false
	}
	planes := make([]geom.Plane, len(faces))
	for i, f := range faces {
		planes[i] = geom.PlaneOf(f.Points)
	}
	var pts []geom.P2
	for _, v := range geom.Vertices(planes) {
		pts = append(pts, geom.P2{X: v[0], Y: v[1]})
	}
	hull := geom.ConvexHull(pts)
	if len(hull) < 3 || geom.SignedArea(hull) <= geom.Eps {
		return area{}, false
	}
	var c geom.P2
	for _, q := range hull {
		c = geom.P2{X: c.X + q.X/float64(len(hull)), Y: c.Y + q.Y/float64(len(hull))}
	}
	lo, hi := math.Inf(1), math.Inf(-1)
	for _, p := range hull { // just inside each corner, where a vertical line meets the brush
		if _, top, ok := geom.Column(planes, p.X+(c.X-p.X)*0.01, p.Y+(c.Y-p.Y)*0.01); ok {
			lo, hi = math.Min(lo, top), math.Max(hi, top)
		}
	}
	return area{hull, [][]geom.P2{hull}, lo, hi + h}, lo <= hi
}

// zoneTarget is the name of a zone's spawner structs: the target of its
// first volume in the map, which is the one the zone manager reads, else
// <zone>_spawners.
func (b *batch) zoneTarget(name string) string {
	for _, e := range b.snap.Map.Entities {
		if e.Class() != "info_volume" {
			continue
		}
		if n, _ := e.Key("targetname"); n == name {
			if t, ok := e.Key("target"); ok && t != "" {
				return t
			}
		}
	}
	return name + "_spawners"
}

// zoneEntities places a zone's riser spawners and its pathnode on its
// floors: on a grid over its areas, where a floor lies near an area's
// bottom with a body's room above it; the pathnode nearest the middle, the
// spawners spread over the other spots, one in each area first.
func (b *batch) zoneEntities(i int, o *ZoneOp, areas []area, target string) error {
	n := 4
	if o.Spawners != nil {
		n = *o.Spawners
	}
	node := o.Pathnode == nil || *o.Pathnode
	if n == 0 && !node {
		return nil
	}
	spots := zoneSpots(b.groundOf(), areas)
	if len(spots) == 0 {
		return fmt.Errorf("no floor under its areas within %d units of their bottom with %d units clear above: build the floor first, or spawners 0 and pathnode false", stepHeight, clearance)
	}
	var mid mapfile.Vec3
	for _, s := range spots {
		mid = mapfile.Vec3{mid[0] + s.at[0]/float64(len(spots)), mid[1] + s.at[1]/float64(len(spots)), 0}
	}
	if node {
		k := 0
		for j, s := range spots {
			if flat(s.at, mid) < flat(spots[k].at, mid) {
				k = j
			}
		}
		near := spots[k].at
		spots = slices.Delete(spots, k, k+1) // a spawner rises elsewhere
		if err := b.entity(i, &EntityOp{Class: "node_pathnode", Origin: mapfile.Vec3{near[0], near[1], near[2] + 2}}); err != nil {
			return err
		}
		b.units[len(b.units)-1].part = o.Name + " pathnode"
	}
	var rooms []int // the areas large enough for a spawner of their own: not a doorway's
	for k, a := range areas {
		if geom.SignedArea(a.outline) >= roomArea {
			rooms = append(rooms, k)
		}
	}
	for _, s := range spread(spots, mid, n, rooms) {
		yaw := math.Round(math.Atan2(mid[1]-s[1], mid[0]-s[0]) * 180 / math.Pi)
		e := &EntityOp{Class: "script_struct", Origin: s, Angles: &[3]float64{0, yaw, 0},
			Keys: map[string]string{"targetname": target, "script_noteworthy": "riser_location", "script_string": "find_flesh"}}
		if err := b.entity(i, e); err != nil {
			return err
		}
		b.units[len(b.units)-1].part = o.Name + " spawner"
	}
	return nil
}

// spot is a point a spawner can rise from, and the area it is in.
type spot struct {
	at   mapfile.Vec3
	area int
}

// zoneSpots are the points on a zone's floors a spawner can rise from: a
// grid over each area's outline.
func zoneSpots(g *ground, areas []area) []spot {
	var out []spot
	for k, a := range areas {
		lo, hi := geom.P2{X: math.Inf(1), Y: math.Inf(1)}, geom.P2{X: math.Inf(-1), Y: math.Inf(-1)}
		for _, q := range a.outline {
			lo, hi = geom.P2{X: math.Min(lo.X, q.X), Y: math.Min(lo.Y, q.Y)}, geom.P2{X: math.Max(hi.X, q.X), Y: math.Max(hi.Y, q.Y)}
		}
		x0, y0 := math.Ceil(lo.X/spawnerSpace)*spawnerSpace, math.Ceil(lo.Y/spawnerSpace)*spawnerSpace
		for i := range int(math.Max(0, math.Floor((hi.X-x0)/spawnerSpace)+1)) { // counted, so no loop can run on
			for j := range int(math.Max(0, math.Floor((hi.Y-y0)/spawnerSpace)+1)) {
				x, y := x0+float64(i)*spawnerSpace, y0+float64(j)*spawnerSpace
				if !inOutline(a.outline, geom.P2{X: x, Y: y}) {
					continue
				}
				if z, ok := g.floor(x, y, a.z0-stepHeight, a.z0+stepHeight); ok && z < a.z1 {
					out = append(out, spot{mapfile.Vec3{x, y, z}, k})
				}
			}
		}
	}
	return out
}

// inOutline tells a point inside a polygon, not on its edge: an even
// number of edges crossed by a ray to +x outside.
func inOutline(poly []geom.P2, p geom.P2) bool {
	in := false
	for i, a := range poly {
		b := poly[(i+1)%len(poly)]
		if math.Abs(geom.Cross2(a, b, p)) <= geom.Eps*math.Hypot(b.X-a.X, b.Y-a.Y) && p.X >= math.Min(a.X, b.X)-geom.Eps && p.X <= math.Max(a.X, b.X)+geom.Eps &&
			p.Y >= math.Min(a.Y, b.Y)-geom.Eps && p.Y <= math.Max(a.Y, b.Y)+geom.Eps {
			return false
		}
		if (a.Y > p.Y) != (b.Y > p.Y) && p.X < a.X+(p.Y-a.Y)*(b.X-a.X)/(b.Y-a.Y) {
			in = !in
		}
	}
	return in
}

// spread picks n spots far from each other, one in each of the rooms (the
// areas named) first while n lasts, so every room of a zone has zombies
// rising in it; each pick is the spot farthest from those picked (the
// first, from the middle).
func spread(spots []spot, mid mapfile.Vec3, n int, rooms []int) []mapfile.Vec3 {
	var picked []mapfile.Vec3
	if n >= len(spots) {
		for _, s := range spots {
			picked = append(picked, s.at)
		}
		return picked
	}
	taken := make([]bool, len(spots))
	pick := func(in func(spot) bool) {
		best, far := -1, -1.0
		for k, s := range spots {
			if taken[k] || !in(s) {
				continue
			}
			d := flat(s.at, mid)
			if len(picked) > 0 {
				d = math.Inf(1)
				for _, p := range picked {
					d = math.Min(d, flat(s.at, p))
				}
			}
			if d > far {
				best, far = k, d
			}
		}
		if best >= 0 {
			taken[best], picked = true, append(picked, spots[best].at)
		}
	}
	for _, a := range rooms[:min(n, len(rooms))] {
		pick(func(s spot) bool { return s.area == a })
	}
	for range n - len(picked) {
		pick(func(spot) bool { return true })
	}
	return picked
}

// flat is the distance between two points seen from above.
func flat(a, b mapfile.Vec3) float64 { return math.Hypot(a[0]-b[0], a[1]-b[1]) }

// madeZone is a zone a batch made: its name and areas, for the checks.
type madeZone struct {
	name  string
	areas []area
}

// checkZones checks a batch's zones once all its ops ran: each link names a
// zone the batch made or the map has; and it notes zones that overlap,
// where a player counts in both, and linked zones that don't meet, where a
// player between them dies. Zones are their convex pieces, each an outline
// from z0 to z1: their bounds would have zones that wind round each other
// overlap.
func (b *batch) checkZones() error {
	made, all := b.zoneParts()
	for _, l := range b.links {
		if _, ok := all[l.B]; !ok {
			return fmt.Errorf("zone %s: link to %s: no zone of that name in the map or the batch", l.A, l.B)
		}
		if g := apart(all[l.A], all[l.B]); g > geom.Eps {
			b.notes = append(b.notes, fmt.Sprintf("zones %s and %s are linked but %s units apart: a player crossing between them is in neither, and dies there; grow one to meet the other", l.A, l.B, num(math.Round(g))))
		}
	}
	for _, n := range slices.Sorted(maps.Keys(made)) {
		for _, m := range slices.Sorted(maps.Keys(all)) {
			if _, also := made[m]; m != n && (!also || m > n) && zonesOverlap(made[n], all[m]) {
				b.notes = append(b.notes, fmt.Sprintf("zone %s overlaps zone %s: a player inside both counts in both; shrink or delete one's volumes", n, m))
			}
		}
	}
	return nil
}

// zoneParts are the pieces of the zones the batch made, and of every zone
// as the batch leaves the map: the map's volumes it hasn't deleted, and its
// own.
func (b *batch) zoneParts() (made, all map[string][]zpart) {
	all, made = map[string][]zpart{}, map[string][]zpart{}
	for _, u := range b.units {
		if u.ent == nil || u.guid == "" || u.state == deleted || u.ent.Class() != "info_volume" {
			continue
		}
		n, _ := u.ent.Key("targetname")
		for _, br := range u.ent.Brushes() {
			if p, ok := brushPart(br); ok && n != "" {
				all[n] = append(all[n], p)
			}
		}
	}
	for _, z := range b.zones {
		for _, a := range z.areas {
			for _, p := range a.parts {
				made[z.name] = append(made[z.name], zpart{p, a.z0, a.z1})
				all[z.name] = append(all[z.name], zpart{p, a.z0, a.z1})
			}
		}
	}
	return made, all
}

// zpart is a convex piece of a zone: its outline from above, from z0 to z1.
type zpart struct {
	hull   []geom.P2
	z0, z1 float64
}

// brushPart is a volume's brush as a zone's piece.
func brushPart(br *mapfile.Brush) (zpart, bool) {
	faces, err := br.Faces()
	if err != nil {
		return zpart{}, false
	}
	planes := make([]geom.Plane, len(faces))
	for i, f := range faces {
		planes[i] = geom.PlaneOf(f.Points)
	}
	p := zpart{z0: math.Inf(1), z1: math.Inf(-1)}
	var pts []geom.P2
	for _, v := range geom.Vertices(planes) {
		pts = append(pts, geom.P2{X: v[0], Y: v[1]})
		p.z0, p.z1 = math.Min(p.z0, v[2]), math.Max(p.z1, v[2])
	}
	p.hull = geom.ConvexHull(pts)
	return p, len(p.hull) >= 3
}

// zonesOverlap tells two zones sharing room: two of their pieces overlapping by
// an area from above and by a height.
func zonesOverlap(a, b []zpart) bool {
	for _, p := range a {
		for _, q := range b {
			if math.Min(p.z1, q.z1)-math.Max(p.z0, q.z0) > geom.Eps && geom.Overlapping(p.hull, q.hull) {
				return true
			}
		}
	}
	return false
}

// apart is how far two zones are from each other: between their nearest
// pieces, across and up; 0 when two of them touch or overlap.
func apart(a, b []zpart) float64 {
	d := math.Inf(1)
	for _, p := range a {
		for _, q := range b {
			up := math.Max(0, math.Max(p.z0-q.z1, q.z0-p.z1))
			d = math.Min(d, math.Hypot(hullDistance(p.hull, q.hull), up))
		}
	}
	return d
}

// hullDistance is how far apart two convex hulls are: 0 when they overlap,
// else the nearest of a corner of one to an edge of the other.
func hullDistance(a, b []geom.P2) float64 {
	if geom.Overlapping(a, b) {
		return 0
	}
	d := math.Inf(1)
	for _, h := range [2][2][]geom.P2{{a, b}, {b, a}} {
		for _, p := range h[0] {
			for i := range h[1] {
				d = math.Min(d, segmentDistance(p, h[1][i], h[1][(i+1)%len(h[1])]))
			}
		}
	}
	return d
}

// segmentDistance is how far a point is from a segment.
func segmentDistance(p, a, b geom.P2) float64 {
	dx, dy := b.X-a.X, b.Y-a.Y
	t := 0.0
	if l := dx*dx + dy*dy; l > 0 {
		t = math.Max(0, math.Min(1, ((p.X-a.X)*dx+(p.Y-a.Y)*dy)/l))
	}
	return math.Hypot(p.X-a.X-t*dx, p.Y-a.Y-t*dy)
}
