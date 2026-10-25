package level

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/McReaper/t7_companion/internal/format/mapfile"
	"github.com/McReaper/t7_companion/internal/level/camera"
)

// boxText is a brush through a box's corners, each face in the material.
func boxText(t *testing.T, guid string, min, max mapfile.Vec3, material string) string {
	t.Helper()
	var pts [][3]float64
	for _, c := range box(min, max).Corners() {
		pts = append(pts, [3]float64(c))
	}
	faces, err := hullFaces(pts)
	if err != nil {
		t.Fatal(err)
	}
	var s strings.Builder
	fmt.Fprintf(&s, "{\n guid \"{%s}\"\n", guid)
	for _, f := range faces {
		s.WriteString(strings.Replace(face(f[0], f[1], f[2]), "caulk", material, 1) + "\n")
	}
	return s.String() + "}\n"
}

// walled is a map with a 16-thick wall at x 0..16; in front of it a clip
// brush, a trigger and a caulked trigger_use; behind it a door (a
// script_brushmodel) and a light.
func walled(t *testing.T) *Snapshot {
	t.Helper()
	text := "{\nguid \"{10000000-0000-0000-0000-000000000000}\"\n\"classname\" \"worldspawn\"\n" +
		boxText(t, "20000000-0000-0000-0000-000000000000", mapfile.Vec3{0, -100, 0}, mapfile.Vec3{16, 100, 100}, "caulk") +
		boxText(t, "30000000-0000-0000-0000-000000000000", mapfile.Vec3{-60, -100, 0}, mapfile.Vec3{-44, 100, 100}, "clip_player") +
		"}\n{\nguid \"{40000000-0000-0000-0000-000000000000}\"\n\"classname\" \"trigger_multiple\"\n" +
		boxText(t, "50000000-0000-0000-0000-000000000000", mapfile.Vec3{-40, -100, 0}, mapfile.Vec3{-24, 100, 100}, "trigger") +
		"}\n{\nguid \"{70000000-0000-0000-0000-000000000000}\"\n\"classname\" \"trigger_use\"\n" +
		boxText(t, "80000000-0000-0000-0000-000000000000", mapfile.Vec3{-20, -100, 0}, mapfile.Vec3{-10, 100, 100}, "caulk") +
		"}\n{\nguid \"{90000000-0000-0000-0000-000000000000}\"\n\"classname\" \"script_brushmodel\"\n" +
		boxText(t, "a0000000-0000-0000-0000-000000000000", mapfile.Vec3{40, -20, 0}, mapfile.Vec3{56, 20, 100}, "caulk") +
		"}\n{\nguid \"{60000000-0000-0000-0000-000000000000}\"\n\"classname\" \"light\"\n\"origin\" \"100 0 50\"\n}\n"
	m, err := mapfile.Parse([]byte(text))
	if err != nil {
		t.Fatal(err)
	}
	return &Snapshot{Map: m, IDs: NewIDs(GUIDs(m))}
}

// Only the wall stops the eye: a clip brush and a trigger are drawn
// see-through, and the sky too once a shot hides it.
func TestOccludersAreTheBrushesRadiantDrawsSolid(t *testing.T) {
	occ := occluders(walled(t), "")
	if len(occ) != 2 || !slices.Equal(occ[0].owners, []string{"20000000", "10000000"}) || occ[0].box.Max[0] != 16 ||
		!slices.Equal(occ[1].owners, []string{"a0000000", "90000000"}) {
		t.Fatalf("occluders = %+v: the wall and the door, no trigger whatever its material", occ)
	}
	for m, want := range map[string]bool{"caulk": false, "sky": false, "clip_player": true, "weaponClip": true, "trigger": true, "portal_nodraw": true, "t7_plaster_smooth_dirty_01": false} {
		if seeThrough(m, false) != want {
			t.Errorf("seeThrough(%q) = %v", m, !want)
		}
	}
	if !seeThrough("sky_zm", true) {
		t.Error("a hidden sky is not seen")
	}
}

// The wall hides what is behind it and what is deep in it, not what is in
// front, beside it, or on its face, nor anything from a camera inside it.
func TestAWallHidesWhatIsBehindIt(t *testing.T) {
	wall := occluders(walled(t), "")[0]
	cam := mapfile.Vec3{-100, 0, 50}
	for _, c := range []struct {
		from, to mapfile.Vec3
		hides    bool
		what     string
	}{
		{cam, mapfile.Vec3{100, 0, 50}, true, "behind it"},
		{cam, mapfile.Vec3{-50, 0, 50}, false, "in front of it"},
		{cam, mapfile.Vec3{8, 0, 50}, true, "8 deep in it"},
		{cam, mapfile.Vec3{1, 0, 50}, false, "on its face, within the margin"},
		{cam, mapfile.Vec3{100, 300, 50}, false, "beside it"},
		{mapfile.Vec3{-100, 150, 50}, mapfile.Vec3{100, 150, 50}, false, "past its end, parallel to its side"},
		{mapfile.Vec3{-100, 0, 150}, mapfile.Vec3{100, 0, 150}, false, "over it"},
		{mapfile.Vec3{8, 0, 50}, mapfile.Vec3{100, 0, 50}, false, "from a camera inside it"},
		{mapfile.Vec3{100, 0, 50}, cam, true, "behind it, seen the other way"},
		{cam, cam, false, "a point at the camera"},
	} {
		if got := wall.blocks(c.from, c.to); got != c.hides {
			t.Errorf("%s: blocks = %v", c.what, got)
		}
	}
	snap := walled(t)
	pts := []labelPoint{{"60000000", mapfile.Vec3{100, 0, 50}}, {"20000000", mapfile.Vec3{8, 0, 50}}, {"aaaaaaaa", mapfile.Vec3{-80, 0, 50}}}
	keep, at := inSight(occluders(snap, ""), cam, pts, [][2]float64{{1, 1}, {2, 2}, {3, 3}})
	if len(keep) != 2 || keep[0].id != "20000000" || keep[1].id != "aaaaaaaa" || !slices.Equal(at, [][2]float64{{2, 2}, {3, 3}}) {
		t.Errorf("in sight: %+v %v: the light is behind the wall; the wall's own id is in it, and kept", keep, at)
	}
	door := []labelPoint{{"90000000", mapfile.Vec3{48, 0, 50}}} // a brush entity's id, at its centre inside its brush
	if keep, _ := inSight(occluders(snap, ""), mapfile.Vec3{48, -300, 50}, door, [][2]float64{{4, 4}}); len(keep) != 1 {
		t.Error("the door's own brush hides its id")
	}
}

// A 3D shot writes no id of an entity a brush hides from its camera: the
// probe above the test map's floor is out of sight from under it, in sight
// from above, and a caller's own marks are written either way.
func TestA3DShotWritesNoIDBehindABrush(t *testing.T) {
	_, c, snap := viewing(t, 121)
	labelled := func(pose camera.Pose, marks []Mark) []string {
		shot, err := View(context.Background(), c, snap, ViewOptions{Camera: &pose, Labels: true, Marks: marks})
		if err != nil {
			t.Fatal(err)
		}
		var ids []string
		for _, l := range shot.Labels {
			ids = append(ids, l.ID)
		}
		return ids
	}
	under := camera.Pose{Origin: mapfile.Vec3{44, 108, -300}, Pitch: 89}
	above := camera.Pose{Origin: mapfile.Vec3{44, 108, 300}, Pitch: -89}
	if ids := labelled(under, nil); slices.Contains(ids, "5ec8de46") {
		t.Errorf("from under the floor: %q", ids)
	}
	if ids := labelled(above, nil); !slices.Contains(ids, "5ec8de46") {
		t.Errorf("from above: %q", ids)
	}
	if ids := labelled(under, []Mark{{Text: "abc", At: mapfile.Vec3{44, 108, 4}}}); !slices.Equal(ids, []string{"abc"}) {
		t.Errorf("a mark under the floor: %q", ids)
	}
}
