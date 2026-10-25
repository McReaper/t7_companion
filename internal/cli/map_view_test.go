package cli

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/McReaper/t7_companion/internal/radiant/radianttest"
)

func TestMapViewAnswersTheProjectionBesideThePNG(t *testing.T) {
	testRadiant(t)
	out, b, err := mapView(context.Background(), mapViewArgs{View: "top", IDs: []string{"6e73936d"}})
	if err != nil {
		t.Fatal(err)
	}
	img, err := png.Decode(bytes.NewReader(b))
	if err != nil || img.Bounds().Dx() != out.W || img.Bounds().Dy() != out.H {
		t.Fatalf("png %v, %v; answer %d×%d", img.Bounds(), err, out.W, out.H)
	}
	if out.Map != "test_claude.map" || out.View != "xy" || out.Camera != nil || len(out.Centre) != 3 || out.Scale <= 0 || out.Labels["6e73936d"] == [2]int{} {
		t.Errorf("answer = %+v", out)
	}
	pose, _, err := mapView(context.Background(), mapViewArgs{Camera: "0,-600,200,-15,90,0", NoLabels: true, Size: 64})
	if err != nil || pose.W != 64 || pose.Labels != nil || len(pose.Camera) != 6 || pose.Camera[3] != -15 || pose.Centre != nil {
		t.Errorf("from a pose: %+v, %v", pose, err)
	}
	at, _, err := mapView(context.Background(), mapViewArgs{View: "front", At: "0 0 0", Radius: 100})
	if err != nil || at.View != "xz" {
		t.Errorf("at a point: %+v, %v", at, err)
	}
}

// A view turns the screen on before it shoots: Windows composes no window
// under a screen that slept, and the capture comes back blank. A bad
// argument wakes nothing.
func TestMapViewWakesTheScreenFirst(t *testing.T) {
	f := testRadiant(t)
	m := radiantMachine.(*radianttest.Machine)
	if _, _, err := mapView(context.Background(), mapViewArgs{View: "top"}); err != nil || m.Wakes() != 1 {
		t.Errorf("a view: %v, %d wakes", err, m.Wakes())
	}
	if _, _, err := mapView(context.Background(), mapViewArgs{Size: -1}); err == nil || m.Wakes() != 1 {
		t.Errorf("a refused view: %v, %d wakes", err, m.Wakes())
	}
	f.Update(func(f *radianttest.Fake) { f.Blank = true })
	if _, _, err := mapView(context.Background(), mapViewArgs{View: "top"}); err == nil || !strings.Contains(err.Error(), "all one colour") || m.Wakes() != 2 {
		t.Errorf("a blank capture after the wake: %v, %d wakes", err, m.Wakes())
	}
}

func TestMapViewRefusesBadArguments(t *testing.T) {
	testRadiant(t)
	for a, want := range map[*mapViewArgs]string{
		{Size: 2000}:          "size 2000: at most 1568",
		{Size: -1}:            "size -1",
		{At: "1 2"}:           `at "1 2": give a point`,
		{Camera: "1 2 3"}:     `camera "1 2 3": give x y z pitch yaw roll`,
		{Camera: "1 2 3 4 5"}: "give x y z pitch yaw roll",
		{View: "iso"}:         `view "iso"`,
	} {
		if _, _, err := mapView(context.Background(), *a); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%+v: err = %v, want %q", *a, err, want)
		}
	}
	onMachine(t)
	if _, _, err := mapView(context.Background(), mapViewArgs{}); err == nil {
		t.Error("a view with no Radiant")
	}
}

func TestMapViewToolGivesTextThenImage(t *testing.T) {
	testRadiant(t)
	res, err := mapViewTool().Handler(context.Background(), mcpRequest("map_view", map[string]any{"ids": "af3c9c64", "size": 100}))
	if err != nil || res.IsError || len(res.Content) != 2 {
		t.Fatalf("result %+v, %v", res, err)
	}
	var out mapViewOut
	if err := json.Unmarshal([]byte(res.Content[0].(mcp.TextContent).Text), &out); err != nil || out.View != "3d" || out.W != 100 {
		t.Errorf("text: %+v, %v", out, err)
	}
	im := res.Content[1].(mcp.ImageContent)
	b, err := base64.StdEncoding.DecodeString(im.Data)
	if err != nil || im.MIMEType != "image/png" {
		t.Fatalf("image: %s, %v", im.MIMEType, err)
	}
	if _, err := png.Decode(bytes.NewReader(b)); err != nil {
		t.Errorf("not a png: %v", err)
	}
	// No size: the default's, not a pixel; the volumes hidden for the shot.
	res, err = mapViewTool().Handler(context.Background(), mcpRequest("map_view", map[string]any{"view": "top", "hide": "volumes"}))
	if err != nil || res.IsError || json.Unmarshal([]byte(res.Content[0].(mcp.TextContent).Text), &out) != nil || out.H != 160 {
		t.Errorf("the default size, volumes hidden: %+v, %v", out, err)
	}
	bad, _ := mapViewTool().Handler(context.Background(), mcpRequest("map_view", map[string]any{"view": "iso"}))
	if !bad.IsError || !strings.Contains(resultText(bad), `view "iso"`) {
		t.Errorf("a bad view: %s", resultText(bad))
	}
}

func TestMapViewCommandWritesThePNG(t *testing.T) {
	testRadiant(t)
	path := filepath.Join(t.TempDir(), "v.png")
	cmd := newMapCmd()
	var stdout, stderr strings.Builder
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs([]string{"view", "5ec8de49", "--size", "120", "--render", "fake", "--out", path})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if img, err := png.Decode(bytes.NewReader(b)); err != nil || img.Bounds().Dx() != 120 {
		t.Errorf("png: %v", err)
	}
	if !strings.Contains(stderr.String(), "wrote "+path) || !strings.Contains(stdout.String(), `"view": "3d"`) {
		t.Errorf("stdout %s\nstderr %s", stdout.String(), stderr.String())
	}
	cmd = newMapCmd()
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs([]string{"view", "--out", filepath.Join(t.TempDir(), "no", "such", "dir", "v.png")})
	if err := cmd.Execute(); err == nil {
		t.Error("writing into a missing directory succeeded")
	}
	cmd = newMapCmd()
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs([]string{"view", "--hide", "lights", "--out", path})
	if err := cmd.Execute(); err == nil || !strings.Contains(err.Error(), `hide "lights"`) {
		t.Errorf("--hide lights: %v", err)
	}
}
