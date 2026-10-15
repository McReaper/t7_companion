package level

import (
	"context"
	"fmt"
	"image"
	"math"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/McReaper/t7_companion/internal/format/mapfile"
	"github.com/McReaper/t7_companion/internal/radiant"
)

// ViewOptions say what View shows.
type ViewOptions struct {
	View   string        // 3d (default), top, front or side; xy, xz and yz too
	IDs    []string      // frame these entities or brushes
	Frame  *Box          // or frame this box
	At     *mapfile.Vec3 // or frame the box of Radius around this point
	Radius float64
	Camera *Pose  // or shoot the 3D view from here
	Size   int    // the image's longer side, at most
	Render string // the 3D view's render method for the shot, put back after
	Labels bool   // write ids on the image
	Hide   string // "volumes", "sky" or both, comma-separated: hidden for the shot
	Bake   bool   // bake Radiant's lighting first and shoot under real lighting
	Look   *Pose  // the way a framed 3D target is looked at (default Looking)
	Marks  []Mark // with Labels, these labels instead of ids
}

// Mark is a label of the caller's: its text (hex digits, as ids are) and the
// point it marks.
type Mark struct {
	Text string
	At   mapfile.Vec3
}

// Shot is what View saw.
type Shot struct {
	View   string // 3d, xy, xz or yz
	Image  *image.RGBA
	Pose   *Pose         // a 3D shot's camera
	Centre *mapfile.Vec3 // a 2D shot's centre
	Scale  float64       // a 2D shot's image pixels per unit
	Labels []Label       // the ids in the image, where their points landed
	Notes  []string      // what View could not put back as it was
	Baked  string        // the bake's end, as Radiant's console said it: "All Probes Done (1.61 s)"
}

// views are the names View takes, by bridge name.
var views = map[string]string{"": "3d", "3d": "3d", "top": "xy", "xy": "xy", "front": "xz", "xz": "xz", "side": "yz", "yz": "yz"}

// renders are the render methods by the short names map_view takes; the
// menu's own names are taken too.
var renders = map[string]string{"wireframe": "Wireframe", "fullbright": "Fullbright", "fake": "Normal-based Fake Lighting",
	"normal": "Normal-based Fake Lighting", "view": "View-Based Fake Lighting", "real": "Real Lighting", "density": "Tri Density"}

// maxLabels bounds the ids written on one image, nearest the centre first.
const maxLabels = 40

// View captures a view of the map open in the Radiant: framed on a target,
// shot from a pose, or as it is; then scales it and writes ids on it. It puts
// back what it changed of the user's views (render method, 2D axis, zoom and
// centre, camera) and notes what it could not.
func View(ctx context.Context, c *radiant.Client, snap *Snapshot, o ViewOptions) (*Shot, error) {
	view, err := o.check()
	if err != nil {
		return nil, err
	}
	target, err := targetBox(snap, o)
	if err != nil {
		return nil, err
	}
	pts := o.marks(snap, target)
	s := &session{c: c, pid: c.Hello.PID}
	var f *frame
	if o.Hide != "" {
		err = s.hide(ctx, snap, o.Hide)
	}
	switch {
	case err != nil:
	case view == "3d":
		f, err = s.view3D(ctx, target, o, points(pts))
	default:
		axis, _ := AxisNamed(view)
		f, err = s.view2D(ctx, axis, target, points(pts))
	}
	s.undo(ctx)
	if err == nil && blank(f) {
		err = fmt.Errorf("the capture is all one colour: Radiant drew nothing in the view (asleep screen, or a window Windows no longer draws); try again once Radiant is on screen")
	}
	if err != nil {
		if len(s.notes) > 0 {
			return nil, fmt.Errorf("%w; and %s", err, strings.Join(s.notes, "; "))
		}
		return nil, err
	}
	shot, err := compose(f, view, pts, o.Size)
	if err != nil {
		return nil, err
	}
	shot.Notes, shot.Baked = s.notes, s.baked
	return shot, nil
}

// check is the view the options name, its bridge name, or why they can't
// be shot.
func (o ViewOptions) check() (string, error) {
	view, ok := views[strings.ToLower(strings.TrimSpace(o.View))]
	switch {
	case !ok:
		return "", fmt.Errorf("view %q: 3d, top, front or side", o.View)
	case o.Camera != nil && view != "3d":
		return "", fmt.Errorf("a camera pose is for the 3d view")
	case o.Render != "" && view != "3d":
		return "", fmt.Errorf("render is the 3d view's: a 2D view draws outlines")
	case o.Bake && view != "3d":
		return "", fmt.Errorf("a bake shows in the 3d view: a 2D view draws outlines")
	case o.Bake && o.Render != "" && renders[strings.ToLower(o.Render)] != "Real Lighting" && !strings.EqualFold(o.Render, "Real Lighting"):
		return "", fmt.Errorf("render %q: a bake shows under real lighting, the render a bake shoots with", o.Render)
	}
	for _, h := range strings.Split(o.Hide, ",") {
		if h = strings.ToLower(strings.TrimSpace(h)); h != "" && h != "volumes" && h != "sky" {
			return "", fmt.Errorf("hide %q: volumes, sky, or both comma-separated", o.Hide)
		}
	}
	return view, nil
}

// marks are the points the shot labels: the caller's, else the ids'.
func (o ViewOptions) marks(snap *Snapshot, target *Box) []labelPoint {
	if !o.Labels {
		return nil
	}
	if len(o.Marks) == 0 {
		return labelPoints(snap, o, target)
	}
	pts := make([]labelPoint, len(o.Marks))
	for i, m := range o.Marks {
		pts[i] = labelPoint{m.Text, m.At}
	}
	return pts
}

// targetBox is what the shot frames, nil for the view as it is.
func targetBox(snap *Snapshot, o ViewOptions) (*Box, error) {
	var b Box
	switch {
	case o.Frame != nil:
		return o.Frame, nil
	case len(o.IDs) > 0:
		boxes, err := boxesOf(snap, o.IDs)
		if err != nil {
			return nil, err
		}
		for _, x := range boxes {
			b.Union(x)
		}
		if !b.Ok {
			return nil, fmt.Errorf("%s: nothing there to frame", strings.Join(o.IDs, ", "))
		}
		b = b.Grown(minFrame)
	case o.At != nil:
		r := o.Radius
		if r <= 0 {
			r = 256
		}
		r = math.Max(r, 16)
		b.Add(mapfile.Vec3{o.At[0] - r, o.At[1] - r, o.At[2] - r})
		b.Add(mapfile.Vec3{o.At[0] + r, o.At[1] + r, o.At[2] + r})
	default:
		return nil, nil
	}
	return &b, nil
}

// minFrame is the least a target framed by ids spans on each axis, in units:
// a point entity shows with the room around it.
const minFrame = 256

// boxesOf are the boxes of the entities and brushes the ids name, by GUID.
func boxesOf(snap *Snapshot, ids []string) (map[string]Box, error) {
	want := map[string]bool{}
	for _, id := range ids {
		g, err := snap.IDs.Resolve(id)
		if err != nil {
			return nil, err
		}
		want[hexOf(g)] = true
	}
	out := map[string]Box{}
	for _, e := range snap.Map.Entities {
		if want[hexOf(e.Guid())] {
			out[e.Guid()] = EntityBox(e)
		}
		for _, b := range e.Brushes() {
			if want[hexOf(b.Guid())] {
				out[b.Guid()] = BrushBox(b)
			}
		}
	}
	return out, nil
}

// labelPoint is an id and the point its label marks.
type labelPoint struct {
	id string
	p  mapfile.Vec3
}

func points(pts []labelPoint) []mapfile.Vec3 {
	out := make([]mapfile.Vec3, len(pts))
	for i, p := range pts {
		out[i] = p.p
	}
	return out
}

// labelPoints are what gets an id on the image: the named entities and
// brushes, else every entity but worldspawn, at its origin or its box's
// centre; the nearest the target's centre first, when there are too many to
// project.
func labelPoints(snap *Snapshot, o ViewOptions, target *Box) []labelPoint {
	var out []labelPoint
	if len(o.IDs) > 0 {
		boxes, _ := boxesOf(snap, o.IDs)
		for g, b := range boxes {
			if b.Ok {
				out = append(out, labelPoint{snap.IDs.Short(g), b.Centre()})
			}
		}
	} else {
		for _, e := range snap.Map.Entities {
			if e.Class() == "worldspawn" {
				continue
			}
			p, ok := Origin(e)
			if b := EntityBox(e); !ok && b.Ok {
				p, ok = b.Centre(), true
			}
			if ok {
				out = append(out, labelPoint{snap.IDs.Short(e.Guid()), p})
			}
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].id < out[j].id })
	if target != nil && len(out) > 1000 {
		c := target.Centre()
		sort.SliceStable(out, func(i, j int) bool { return dist(out[i].p, c) < dist(out[j].p, c) })
		out = out[:1000]
	}
	return out
}

func dist(a, b mapfile.Vec3) float64 { d := sub(a, b); return math.Sqrt(dot(d, d)) }

// session is one View's calls to Radiant: what it changed of the user's
// views, to undo, and what it could not put back.
type session struct {
	c        *radiant.Client
	pid      int
	restores []restore
	notes    []string
	baked    string // the console line that ended the bake
}

// blank tells a capture of one colour: no view Radiant draws is, as its
// statistics line alone has text in it.
func blank(f *frame) bool {
	p := f.Pixels
	for i := 4; i+3 < len(p); i += 4 {
		if p[i] != p[0] || p[i+1] != p[1] || p[i+2] != p[2] {
			return false
		}
	}
	return true
}

// hide hides, for the shot, what the hide option names: volumes and triggers,
// whose tool textures draw over a view from inside them as well as from
// outside (a volume_decal stays, as it draws its decal); the sky's brushes,
// whose faces draw over a view from outside the skybox. View/Show Last
// Hidden shows them again, and only them: what the user had hidden stays
// hidden.
func (s *session) hide(ctx context.Context, snap *Snapshot, what string) error {
	guids := hiddenBy(snap, strings.ToLower(what))
	if len(guids) == 0 {
		return nil
	}
	err := s.call(ctx, "deselect")
	if err == nil {
		err = s.call(ctx, "select "+strings.Join(guids, " "))
	}
	if err == nil {
		err = s.call(ctx, "command View/Hide Entities/Hide Selected")
	}
	restoreSelection(ctx, s.c, snap.Sel.GUIDs)
	if err != nil {
		return fmt.Errorf("hiding the %s: %w", what, err)
	}
	s.restores = append(s.restores, restore{what: "hidden " + what, lines: []string{"command View/Show Last Hidden"}})
	return nil
}

// hiddenBy is the GUIDs a hide option names: entities for volumes, brushes
// with a sky face for sky.
func hiddenBy(snap *Snapshot, what string) []string {
	volumes, sky := strings.Contains(what, "volumes"), strings.Contains(what, "sky")
	var guids []string
	for _, e := range snap.Map.Entities {
		if c := e.Class(); volumes && c != "volume_decal" && (strings.Contains(c, "volume") || strings.HasPrefix(c, "trigger_")) {
			guids = append(guids, e.Guid())
			continue
		}
		for _, it := range e.Items {
			if sky && it.Brush != nil && skyBrush(it.Brush) {
				guids = append(guids, it.Brush.Guid())
			}
		}
	}
	return guids
}

// skyBrush tells a brush with a face of the sky's material, as a skybox is.
func skyBrush(b *mapfile.Brush) bool {
	faces, _ := b.Faces()
	return slices.ContainsFunc(faces, func(f mapfile.Face) bool { return strings.EqualFold(f.Material, "sky") })
}

// restore is the calls that put one thing back as the user had it.
type restore struct {
	what  string
	lines []string
}

func (s *session) call(ctx context.Context, line string) error {
	_, err := s.c.Call(ctx, line, nil)
	return err
}

// undo puts back what the session changed, last change first, even once the
// caller's context has ended; a failure is noted.
func (s *session) undo(ctx context.Context) {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), restoreTimeout)
	defer cancel()
	for i := len(s.restores) - 1; i >= 0; i-- {
		for _, line := range s.restores[i].lines {
			if err := s.call(ctx, line); err != nil {
				s.notes = append(s.notes, fmt.Sprintf("could not put back the %s: %v", s.restores[i].what, err))
				break
			}
		}
	}
}

// lensSlack is how far a measured focal length may be off the assumed one
// before a framed shot is taken again: a view's fill shifts as much.
const lensSlack = 0.05

// restoreTimeout bounds putting the user's views back.
const restoreTimeout = 10 * time.Second

// lenses are the 3D views' lenses as last measured, by Radiant pid; until
// one is, the lens of a 1648×869 view Radiant drew.
var (
	lensMu sync.Mutex
	lenses = map[int]Lens{}
	guess  = Lens{W: 1648, H: 869, F: 1212.4}
)

func (s *session) lens() Lens { return lensOf(s.pid) }

// lensOf is a Radiant's 3D lens as last measured, else the guess.
func lensOf(pid int) Lens {
	lensMu.Lock()
	defer lensMu.Unlock()
	if l, ok := lenses[pid]; ok {
		return l
	}
	return guess
}

func (s *session) remember(l Lens) {
	lensMu.Lock()
	defer lensMu.Unlock()
	lenses[s.pid] = l
}

// view3D shoots the 3D view: from the pose given, framed on the target (a
// second shot when the lens it assumed was off), or as it is.
func (s *session) view3D(ctx context.Context, target *Box, o ViewOptions, pts []mapfile.Vec3) (*frame, error) {
	render := o.Render
	if o.Bake {
		render = "real"
	}
	if err := s.setRender(ctx, render); err != nil {
		return nil, err
	}
	if o.Bake {
		if err := s.bake(ctx); err != nil {
			return nil, err
		}
	}
	pose := o.Camera
	var corners []mapfile.Vec3
	look := Looking
	if o.Look != nil {
		look = *o.Look
	}
	if pose == nil && target != nil {
		corners = target.Corners()
		p := FrameBox(s.lens(), look, *target)
		pose = &p
	}
	f, err := shoot(ctx, s.c, "3d", pose, append(corners, pts...))
	if err == nil && corners != nil {
		// The lens the frame assumed is checked against where Radiant drew the
		// corners: a frame from another lens is shot again with the one measured.
		if l := (Lens{f.W, f.H, FocalFrom(f.W, f.H, f.Pose, corners, f.At[:len(corners)])}); l.F > 0 {
			assumed := s.lens()
			s.remember(l)
			if !fits(f.At[:len(corners)], f.W, f.H) || l.W != assumed.W || l.H != assumed.H || math.Abs(l.F/assumed.F-1) > lensSlack {
				p := FrameBox(l, look, *target)
				f, err = shoot(ctx, s.c, "3d", &p, append(corners, pts...))
			}
		}
	}
	if err != nil {
		return nil, err
	}
	f.At = f.At[len(corners):]
	return f, nil
}

// bakeTimeout bounds the wait on Radiant's bake; a variable so tests wait
// less.
var bakeTimeout = 10 * time.Minute

// bake runs Radiant's own lighting bake, the lighting toolbar's bolt at its
// L/M/H setting, and waits for the console's "All Probes Done" that ends it.
// It lights Radiant's view only: the game's .led is the build's light stage.
// The console is read from before the bake starts, as the client drops what
// it can't hold.
func (s *session) bake(ctx context.Context) error {
	if err := s.call(ctx, "subscribe console"); err != nil {
		return err
	}
	if err := s.call(ctx, "bake"); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, bakeTimeout)
	defer cancel()
	for {
		select {
		case e, ok := <-s.c.Events():
			switch {
			case !ok:
				return fmt.Errorf("bake: the bridge closed before the bake ended")
			case e.Name == "console" && strings.HasPrefix(e.Args, "All Probes Done"):
				s.baked = e.Args
				return nil
			}
		case <-ctx.Done():
			return fmt.Errorf("bake: Radiant said no \"All Probes Done\" in %s", bakeTimeout)
		}
	}
}

// setRender switches the 3D view's render method for the shot; the session
// puts the user's back.
func (s *session) setRender(ctx context.Context, render string) error {
	if render == "" {
		return nil
	}
	want, ok := renders[strings.ToLower(render)]
	if !ok {
		for _, m := range renders {
			if strings.EqualFold(m, render) {
				want, ok = m, true
			}
		}
	}
	if !ok {
		return fmt.Errorf("render %q: wireframe, fullbright, fake, view, real or density", render)
	}
	st, err := s.c.State(ctx)
	if err != nil {
		return err
	}
	if st.Render == want {
		return nil
	}
	if err := s.call(ctx, "command View/Render Method/"+want); err != nil {
		return err
	}
	if st.Render != "" {
		s.restores = append(s.restores, restore{"render method", []string{"command View/Render Method/" + st.Render}})
	}
	return nil
}

// fits reports whether the points all landed in the view and span more than
// half of it one way.
func fits(at [][2]float64, w, h int) bool {
	minX, minY, maxX, maxY := math.Inf(1), math.Inf(1), math.Inf(-1), math.Inf(-1)
	for _, p := range at {
		if math.IsNaN(p[0]) {
			return false
		}
		minX, minY, maxX, maxY = math.Min(minX, p[0]), math.Min(minY, p[1]), math.Max(maxX, p[0]), math.Max(maxY, p[1])
	}
	fw, fh := float64(w), float64(h)
	return minX >= 0 && minY >= 0 && maxX <= fw && maxY <= fh && (maxX-minX > fw/2 || maxY-minY > fh/2)
}
