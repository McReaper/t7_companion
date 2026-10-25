package level

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/McReaper/t7_companion/internal/format/mapfile"
	"github.com/McReaper/t7_companion/internal/level/camera"
	"github.com/McReaper/t7_companion/internal/level/picture"
	"github.com/McReaper/t7_companion/internal/radiant"
	"github.com/McReaper/t7_companion/internal/radiant/radianttest"
)

// viewing is a fake Radiant with the test map open, connected, and its
// snapshot; each test takes its own pid, as lenses are kept per Radiant.
func viewing(t *testing.T, pid int) (*radianttest.Fake, *radiant.Client, *Snapshot) {
	t.Helper()
	b, err := os.ReadFile("testdata/test_claude.map")
	if err != nil {
		t.Fatal(err)
	}
	f := radianttest.New(pid, `C:\m\test_claude.map`)
	f.Text = string(b)[strings.Index(string(b), "// entity 0"):]
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)
	c, err := radiant.NewClient(ctx, f.Dial())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(c.Close)
	snap, err := Read(ctx, c)
	if err != nil {
		t.Fatal(err)
	}
	return f, c, snap
}

// sent are the requests of one kind the fake served.
func sent(f *radianttest.Fake, prefix string) []string {
	var out []string
	for _, r := range f.Requests() {
		if strings.HasPrefix(r, prefix) {
			out = append(out, r)
		}
	}
	return out
}

func TestA3DViewFramesItsTargetMeasuringTheLensOnce(t *testing.T) {
	f, c, snap := viewing(t, 101)
	f.Update(func(f *radianttest.Fake) { f.Focal = 40 }) // a wider lens than the one guessed
	shot, err := View(context.Background(), c, snap, ViewOptions{IDs: []string{"5ec8de49"}, Labels: true})
	if err != nil {
		t.Fatal(err)
	}
	if n := len(sent(f, "capture 3d ")); n != 2 {
		t.Errorf("%d captures: the guessed lens is not the fake's, so it takes two", n)
	}
	if shot.View != "3d" || shot.Pose == nil || shot.Pose.Pitch != -35 || shot.Pose.Yaw != 45 || shot.Image.Rect.Dx() != 160 {
		t.Errorf("shot = %+v", shot)
	}
	if len(shot.Labels) != 1 || shot.Labels[0].ID != "5ec8de49" || abs(shot.Labels[0].X-80) > 1 || abs(shot.Labels[0].Y-45) > 1 { // the centre, within the pixel it falls in
		t.Errorf("labels = %+v: the floor's centre, at the view's", shot.Labels)
	}
	if _, err := View(context.Background(), c, snap, ViewOptions{IDs: []string{"af3c9c64"}}); err != nil {
		t.Fatal(err)
	}
	if n := len(sent(f, "capture 3d ")); n != 3 {
		t.Errorf("%d captures in all: the second view knew the lens", n)
	}
}

func TestA3DViewFromAPoseOrAsItIs(t *testing.T) {
	f, c, snap := viewing(t, 102)
	pose := camera.Pose{Origin: mapfile.Vec3{0, -600, 200}, Pitch: -15, Yaw: 90}
	shot, err := View(context.Background(), c, snap, ViewOptions{Camera: &pose, Labels: true, Size: 80})
	if err != nil {
		t.Fatal(err)
	}
	if got := sent(f, "capture"); len(got) != 1 || got[0] != "capture 3d 0 -600 200 -15 90 0" {
		t.Errorf("captures = %q", got)
	}
	if shot.Image.Rect.Dx() != 80 || shot.Image.Rect.Dy() != 45 || *shot.Pose != pose {
		t.Errorf("shot %v, pose %+v", shot.Image.Rect, shot.Pose)
	}
	var ids []string
	for _, l := range shot.Labels {
		ids = append(ids, l.ID)
	}
	if strings.Join(ids, " ") != "6e73936d 5ec8de46 76780cea f2d51a52" {
		t.Errorf("labels %q: every entity but worldspawn in the frame, nearest the centre first", ids)
	}
	if _, err := View(context.Background(), c, snap, ViewOptions{View: "3D"}); err != nil {
		t.Fatal(err)
	}
	if got := sent(f, "capture"); len(got) != 2 || got[1] != "capture 3d" {
		t.Errorf("as it is: %q", got)
	}
}

func TestARenderMethodIsUsedForTheShotThenPutBack(t *testing.T) {
	f, c, snap := viewing(t, 103)
	for _, render := range []string{"fake", "Fullbright", "real"} {
		if _, err := View(context.Background(), c, snap, ViewOptions{Render: render}); err != nil {
			t.Fatal(err)
		}
	}
	want := []string{"command View/Render Method/Normal-based Fake Lighting", "command View/Render Method/Real Lighting",
		"command View/Render Method/Fullbright", "command View/Render Method/Real Lighting"}
	if got := sent(f, "command"); strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("commands %q\nwant %q (the user's real lighting needs no switch)", got, want)
	}
	if _, err := View(context.Background(), c, snap, ViewOptions{Render: "sepia"}); err == nil || !strings.Contains(err.Error(), "wireframe, fullbright") {
		t.Errorf("an unknown render method: %v", err)
	}
}

func TestATopViewTurnsZoomsAndCentresThenPutsItAllBack(t *testing.T) {
	f, c, snap := viewing(t, 104)
	f.Update(func(f *radianttest.Fake) {
		f.View2D, f.Scale2D, f.Centre2D, f.Camera = "YZ", 0.8, [3]float64{5, 6, 7}, [6]float64{1, 2, 3, 0, 90, 0}
	})
	shot, err := View(context.Background(), c, snap, ViewOptions{View: "top", IDs: []string{"6e73936d"}, Labels: true})
	if err != nil {
		t.Fatal(err)
	}
	// the volume_sun: 568×584 units, in a 90×160 view: 0.134 px per unit, 9 steps out
	if shot.View != "xy" || *shot.Centre != (mapfile.Vec3{12, 28, 16}) || fmt.Sprintf("%.4f", shot.Scale) != "0.1342" {
		t.Errorf("shot centre %v scale %v", shot.Centre, shot.Scale)
	}
	if len(shot.Labels) != 1 || shot.Labels[0] != (picture.Label{ID: "6e73936d", X: 45, Y: 80}) {
		t.Errorf("labels = %+v", shot.Labels)
	}
	var st radianttest.Fake
	f.Update(func(f *radianttest.Fake) {
		st.View2D, st.Scale2D, st.Centre2D, st.Camera = f.View2D, f.Scale2D, f.Centre2D, f.Camera
	})
	if st.View2D != "YZ" || fmt.Sprintf("%.4f", st.Scale2D) != "0.8000" || st.Centre2D != [3]float64{5, 6, 7} || st.Camera != [6]float64{1, 2, 3, 0, 90, 0} {
		t.Errorf("put back: axis %s, scale %v, centre %v, camera %v", st.View2D, st.Scale2D, st.Centre2D, st.Camera)
	}
	if n := len(sent(f, "command View/Layout/Next")); n != 3 {
		t.Errorf("%d turns: one to XY, two back to YZ", n)
	}
}

func TestA2DViewAsItIs(t *testing.T) {
	f, c, snap := viewing(t, 105)
	f.Update(func(f *radianttest.Fake) { f.Centre2D, f.Camera = [3]float64{5, 6, 7}, [6]float64{1, 2, 3, 0, 90, 0} })
	shot, err := View(context.Background(), c, snap, ViewOptions{View: "top", Labels: true})
	if err != nil {
		t.Fatal(err)
	}
	if shot.View != "xy" || len(f.Requests()) != 5 || *shot.Centre != (mapfile.Vec3{5, 6, 7}) {
		t.Errorf("the axis shown, as it is: %q", f.Requests())
	}
	// another axis: turning centres the view on the camera, so the user's
	// centre comes back through it, after the axis
	shot, err = View(context.Background(), c, snap, ViewOptions{View: "side"})
	if err != nil {
		t.Fatal(err)
	}
	var st radianttest.Fake
	f.Update(func(f *radianttest.Fake) { st.View2D, st.Centre2D, st.Camera = f.View2D, f.Centre2D, f.Camera })
	if shot.View != "yz" || len(sent(f, "command View/Layout/Next")) != 3 || st.View2D != "XY (Top)" ||
		st.Centre2D != [3]float64{5, 6, 7} || st.Camera != [6]float64{1, 2, 3, 0, 90, 0} {
		t.Errorf("view %s; put back: axis %s, centre %v, camera %v", shot.View, st.View2D, st.Centre2D, st.Camera)
	}
}

func TestViewRefusals(t *testing.T) {
	_, c, snap := viewing(t, 106)
	pose := camera.Pose{}
	for o, want := range map[*ViewOptions]string{
		{View: "diagonal"}:                 `view "diagonal": 3d, top, front or side`,
		{View: "top", Camera: &pose}:       "a camera pose is for the 3d view",
		{View: "front", Render: "fake"}:    "render is the 3d view's",
		{IDs: []string{"00000000"}}:        "no entity or brush 00000000",
		{IDs: []string{"5ec8de4"}}:         "names 2 entities or brushes",
		{View: "top", At: &mapfile.Vec3{}}: "",
	} {
		_, err := View(context.Background(), c, snap, *o)
		if want == "" {
			if err != nil {
				t.Errorf("%+v: %v", *o, err)
			}
		} else if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%+v: err = %v, want %q", *o, err, want)
		}
	}
}

func TestAViewSaysWhatItCouldNotPutBack(t *testing.T) {
	f, c, snap := viewing(t, 107)
	f.Handle("command", func(f *radianttest.Fake, args string, _ []byte) radianttest.Reply {
		if args == "View/Render Method/Real Lighting" {
			return radianttest.Err("disabled", args+" is disabled")
		}
		f.Render = strings.TrimPrefix(args, "View/Render Method/")
		return radianttest.OK("", nil)
	})
	shot, err := View(context.Background(), c, snap, ViewOptions{Render: "wireframe"})
	if err != nil || len(shot.Notes) != 1 || !strings.Contains(shot.Notes[0], "could not put back the render method") {
		t.Errorf("notes %q, %v", shot.Notes, err)
	}
	f.Update(func(f *radianttest.Fake) { f.Minimised, f.Render = true, "Real Lighting" })
	_, err = View(context.Background(), c, snap, ViewOptions{Render: "wireframe"})
	if !radiant.IsCode(err, "minimised") || !strings.Contains(err.Error(), "; and could not put back the render method") {
		t.Errorf("a failed shot whose put-back failed too: %v", err)
	}
}

func TestShootReadsTheCapturesAnswer(t *testing.T) {
	f, c, _ := viewing(t, 108)
	for name, tc := range map[string]struct {
		summary string
		body    string
		want    string
	}{
		"no size":        {"origin=0,0,0", "", "has no size"},
		"short pixels":   {"w=2 h=2", "1234", "less than a 2×2 frame"},
		"too few points": {"w=1 h=1", "abcd", "projected 0 of 1 points"},
	} {
		f.Handle("capture", func(*radianttest.Fake, string, []byte) radianttest.Reply {
			return radianttest.OK(tc.summary, []byte(tc.body))
		})
		if _, err := shoot(context.Background(), c, "3d", nil, []mapfile.Vec3{{}}); err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: err = %v", name, err)
		}
	}
	f.Handle("capture", func(*radianttest.Fake, string, []byte) radianttest.Reply {
		return radianttest.OK("w=1 h=1 centre=1,2,3 scale=2", []byte("abcd-\n3 x\n4 5\n"))
	})
	fr, err := shoot(context.Background(), c, "xy", nil, []mapfile.Vec3{{}, {}, {}})
	if err != nil || fr.Centre != (mapfile.Vec3{1, 2, 3}) || fr.Scale != 2 || fr.At[2] != [2]float64{4, 5} || fr.At[0] == fr.At[0] || fr.At[1] == fr.At[1] {
		t.Errorf("frame %+v, %v: behind and unreadable points are NaN", fr, err)
	}
}

func TestTheIDsOnAnImageStopAtForty(t *testing.T) {
	f, c, _ := viewing(t, 109)
	var text strings.Builder
	text.WriteString("{\nguid \"{00000000-0000-0000-0000-000000000001}\"\n\"classname\" \"worldspawn\"\n}\n")
	for i := range 60 {
		fmt.Fprintf(&text, "{\nguid \"{%08X-0000-0000-0000-000000000000}\"\n\"classname\" \"info_null\"\n\"origin\" \"%d 0 0\"\n}\n", i+16, i)
	}
	f.Update(func(f *radianttest.Fake) { f.Text = text.String() })
	snap, err := Read(context.Background(), c)
	if err != nil {
		t.Fatal(err)
	}
	pose := camera.Pose{Origin: mapfile.Vec3{30, -200, 0}, Yaw: 90}
	shot, err := View(context.Background(), c, snap, ViewOptions{Camera: &pose, Labels: true})
	if err != nil || len(shot.Labels) != maxLabels {
		t.Errorf("%d labels, %v", len(shot.Labels), err)
	}
}

// Hiding the volumes for a shot: the map's volume_sun hidden, shot, shown
// again; the user's selection as it was.
func TestAViewCanHideTheVolumesForTheShot(t *testing.T) {
	f, c, _ := viewing(t, 106)
	f.Update(func(f *radianttest.Fake) { f.Selection = []string{"{5EC8DE49-C58E-11F1-B542-00155DD185EA}"} })
	snap, err := Read(context.Background(), c)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := View(context.Background(), c, snap, ViewOptions{View: "top", Hide: "Volumes"}); err != nil {
		t.Fatal(err)
	}
	var seq []string
	for _, r := range f.Requests() {
		if strings.HasPrefix(r, "command View/") && !strings.HasPrefix(r, "command View/Zoom") && !strings.HasPrefix(r, "command View/Layout") || strings.HasPrefix(r, "capture") || strings.HasPrefix(r, "select ") {
			seq = append(seq, r)
		}
	}
	want := []string{"select {6E73936D-", "command View/Hide Entities/Hide Selected", "select {5EC8DE49-", "capture xy", "command View/Show Last Hidden"}
	for i, w := range want {
		if len(seq) != len(want) || !strings.HasPrefix(seq[i], w) {
			t.Fatalf("requests %q, want them to start %q", seq, want)
		}
	}
	if seq[0] != "select {6E73936D-C58E-11F1-B542-00155DD185EA}" {
		t.Errorf("hid %q: the volume_sun alone (the probes are no volumes)", seq[0])
	}
	f.Update(func(f *radianttest.Fake) {
		if f.Hidden != nil || len(f.Selection) != 1 {
			t.Errorf("after: hidden %v, selection %v", f.Hidden, f.Selection)
		}
	})
	if _, err := View(context.Background(), c, snap, ViewOptions{Hide: "lights"}); err == nil || !strings.Contains(err.Error(), `hide "lights": volumes`) {
		t.Errorf("hide lights: %v", err)
	}
	f.Handle("command", func(*radianttest.Fake, string, []byte) radianttest.Reply {
		return radianttest.Err("missing", "no menu entry")
	})
	if _, err := View(context.Background(), c, snap, ViewOptions{View: "top", Hide: "volumes"}); err == nil || !strings.Contains(err.Error(), "hiding the volumes: ") {
		t.Errorf("a Radiant without the menu: %v", err)
	}
}

// A view Radiant drew nothing in comes back white: no image to reason from,
// whichever pixel tells it.
func TestABlankCaptureIsRefused(t *testing.T) {
	f, c, snap := viewing(t, 107)
	f.Update(func(f *radianttest.Fake) { f.Blank = true })
	if _, err := View(context.Background(), c, snap, ViewOptions{View: "top"}); err == nil || !strings.Contains(err.Error(), "the capture is all one colour") {
		t.Errorf("a white capture: %v", err)
	}
	for i := range 3 {
		fr := &frame{Pixels: bytes.Repeat([]byte{7, 7, 7, 255}, 4)}
		fr.Pixels[12+i]++ // the last pixel's blue, green or red
		if blank(fr) {
			t.Errorf("a frame whose last pixel differs in channel %d is blank", i)
		}
	}
	if !blank(&frame{Pixels: []byte{1, 2, 3, 255, 1, 2, 3, 0}}) || !blank(&frame{}) {
		t.Error("one colour, alpha aside, is blank, as is no pixel")
	}
}

func abs(n int) int { return max(n, -n) }

// Hiding the sky names the brushes with a sky face and nothing else; hiding
// the volumes names the volume entities, not the sky.
func TestHidingTheSkyNamesItsBrushes(t *testing.T) {
	brush := func(guid, material string, lo, hi mapfile.Vec3) string {
		return strings.Replace(BoxBrush(box(lo, hi), material, ""), "{\n", "{\n guid \"{"+guid+"-0000-0000-0000-000000000000}\"\n", 1)
	}
	text := "{\n\"classname\" \"worldspawn\"\n" +
		brush("5a5a5a5a", "sky", mapfile.Vec3{-512, -512, 512}, mapfile.Vec3{512, 512, 544}) +
		brush("c0c0c0c0", "concrete", mapfile.Vec3{0, 0, 0}, mapfile.Vec3{64, 64, 16}) + "}\n" +
		"{\nguid \"{0f0f0f0f-0000-0000-0000-000000000000}\"\n\"classname\" \"info_volume\"\n" +
		brush("0e0e0e0e", "volume", mapfile.Vec3{0, 0, 0}, mapfile.Vec3{64, 64, 64}) + "}\n"
	m, err := mapfile.Parse([]byte(text))
	if err != nil {
		t.Fatal(err)
	}
	snap := &Snapshot{Map: m}
	if got := hiddenBy(snap, "sky"); len(got) != 1 || !strings.Contains(got[0], "5a5a5a5a") {
		t.Errorf("sky hides %q, want the sky brush alone", got)
	}
	if got := hiddenBy(snap, "volumes"); len(got) != 1 || !strings.Contains(got[0], "0f0f0f0f") {
		t.Errorf("volumes hides %q, want the info_volume alone", got)
	}
}

// A 3D shot from a pose waits out a mouse button held for an instant, the
// bridge refusing it busy meanwhile, and says what to do when one stays held.
func TestA3DShotFromAPoseWaitsForTheMouseButtons(t *testing.T) {
	f, c, snap := viewing(t, 120)
	oldWait, oldPoll := mouseWait, mousePoll
	mouseWait, mousePoll = 300*time.Millisecond, 10*time.Millisecond
	t.Cleanup(func() { mouseWait, mousePoll = oldWait, oldPoll })
	pose := camera.Pose{Origin: mapfile.Vec3{0, -600, 200}, Pitch: -15, Yaw: 90}
	f.Update(func(f *radianttest.Fake) { f.MouseHeld = 2 })
	if _, err := View(context.Background(), c, snap, ViewOptions{Camera: &pose}); err != nil {
		t.Fatalf("a button held for two tries: %v", err)
	}
	if n := len(sent(f, "capture 3d 0 -600 200")); n != 3 {
		t.Errorf("%d captures: two refused, the third taken", n)
	}
	f.Update(func(f *radianttest.Fake) { f.MouseHeld = 1 << 20 })
	start := time.Now()
	_, err := View(context.Background(), c, snap, ViewOptions{Camera: &pose})
	if err == nil || !radiant.IsCode(err, "busy") || !strings.Contains(err.Error(), "a mouse button is held for 300ms: a 3D shot from a camera waits for every mouse button to be up") {
		t.Errorf("a button that stays held: %v", err)
	}
	if d := time.Since(start); d < 300*time.Millisecond || d > 3*time.Second {
		t.Errorf("waited %v, not about mouseWait", d)
	}
}
