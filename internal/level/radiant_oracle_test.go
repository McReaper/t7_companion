package level

import (
	"context"
	"math"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/McReaper/t7_companion/internal/format/mapfile"
	"github.com/McReaper/t7_companion/internal/radiant"
)

// BrushBox agrees with Radiant's own bounds on every brush of the map open in
// a running Radiant: the bridge's selection gives them. Opt-in on
// T7KB_RADIANT_PID, the pid of a Radiant whose selection the test may change
// (it deselects at the end): never the user's.
func TestBrushBoxesMatchRadiants(t *testing.T) {
	pid, _ := strconv.Atoi(os.Getenv("T7KB_RADIANT_PID"))
	if pid == 0 {
		t.Skip("T7KB_RADIANT_PID not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	c, err := radiant.Connect(ctx, radiant.Local, pid)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	r, err := c.Call(ctx, "entities", nil)
	if err != nil {
		t.Fatal(err)
	}
	m, err := mapfile.Parse(r.Body)
	if err != nil {
		t.Fatal(err)
	}
	ours := map[string]Box{}
	var guids []string
	for _, e := range m.Entities {
		for _, b := range e.Brushes() {
			if b.Kind() != mapfile.KindEmpty {
				ours[strings.ToUpper(b.Guid())] = BrushBox(b)
				guids = append(guids, b.Guid())
			}
		}
	}
	defer func() { _, _ = c.Call(ctx, "deselect", nil) }()
	var checked, differ int
	for start := 0; start < len(guids); start += 200 {
		batch := guids[start:min(start+200, len(guids))]
		if _, err := c.Call(ctx, "deselect", nil); err != nil {
			t.Fatal(err)
		}
		r, err := c.Call(ctx, "select "+strings.Join(batch, " "), nil)
		if err != nil {
			t.Fatal(err)
		}
		for _, line := range strings.Split(strings.TrimSpace(string(r.Body)), "\n") {
			f := strings.Fields(line)
			if len(f) != 8 {
				continue
			}
			var theirs [6]float64
			for i := range 6 {
				theirs[i], _ = strconv.ParseFloat(f[2+i], 64)
			}
			b := flatPadded(ours[strings.ToUpper(f[0])])
			checked++
			if theirs[0] > theirs[3] { // Radiant's empty bounds: a brush whose planes enclose nothing
				if b.Ok {
					t.Errorf("brush %s: Radiant finds it empty, ours is %+v", f[0], b)
				}
				continue
			}
			if !b.Ok || !near(b.Min, theirs[:3]) || !near(b.Max, theirs[3:]) {
				differ++
				if differ <= 10 {
					t.Errorf("brush %s: ours %+v, Radiant's %v", f[0], b, theirs)
				}
			}
		}
	}
	t.Logf("%d of %d brushes checked against Radiant, %d differ", checked, len(guids), differ)
}

// pad is half the thickness Radiant gives a box on an axis where it is
// thinner (a flat patch, a decal): it widens it there about its middle.
const pad = 0.125

func flatPadded(b Box) Box {
	for i := range 3 {
		if b.Max[i]-b.Min[i] < 2*pad {
			mid := (b.Min[i] + b.Max[i]) / 2
			b.Min[i], b.Max[i] = mid-pad, mid+pad
		}
	}
	return b
}

func near(v mapfile.Vec3, w []float64) bool {
	for i := range 3 {
		if math.Abs(v[i]-w[i]) > 0.05 {
			return false
		}
	}
	return true
}
