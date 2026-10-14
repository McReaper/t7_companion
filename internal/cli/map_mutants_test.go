package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/McReaper/t7_companion/internal/format/mapfile"
	"github.com/McReaper/t7_companion/internal/level"
)

// The cases below pin what the CI's mutation run found unchecked.

func TestBrushListsStopAtTwoHundredRows(t *testing.T) {
	f := testRadiant(t)
	var text strings.Builder
	text.WriteString("{\nguid \"{00000000-0000-0000-0000-0000000000F0}\"\n\"classname\" \"worldspawn\"\n")
	for i := range maxBrushRows {
		b := level.BoxBrush(level.Box{Min: mapfile.Vec3{float64(i), 0, 0}, Max: mapfile.Vec3{float64(i) + 1, 1, 1}, Ok: true}, "caulk", "")
		text.WriteString(strings.Replace(b, "{\n", fmt.Sprintf("{\n guid \"{%08X-0000-0000-0000-000000000000}\"\n", 0x20000000+i), 1))
	}
	text.WriteString("}\n{\nguid \"{0FFFFFFF-0000-0000-0000-000000000000}\"\n\"classname\" \"light\"\n\"origin\" \"0 0 0\"\n}\n")
	f.Text = text.String()
	out, err := mapFind(context.Background(), mapFindArgs{Brushes: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Found) != 1 || len(out.Found[0].List) != maxBrushRows || out.More != "1 more: narrow the query, or raise limit" {
		t.Errorf("found %d entities, %d rows, more %q", len(out.Found), len(out.Found[0].List), out.More)
	}
}

func TestAnswersHaveNoNegativeZero(t *testing.T) {
	b, err := json.Marshal(vec(mapfile.Vec3{-0.0001, 1.23456, 0}))
	if err != nil || string(b) != "[0,1.235,0]" {
		t.Errorf("vec = %s, %v", b, err)
	}
}

func TestMapViewTakesTheLargestSizeAndGivesTheScaleToFourPlaces(t *testing.T) {
	testRadiant(t)
	if _, _, err := mapView(context.Background(), mapViewArgs{Size: maxViewSize}); err != nil {
		t.Errorf("size %d: %v", maxViewSize, err)
	}
	out, _, err := mapView(context.Background(), mapViewArgs{View: "top", IDs: []string{"6e73936d"}})
	if err != nil || out.Scale != 0.1342 {
		t.Errorf("scale %v, %v", out.Scale, err)
	}
}
