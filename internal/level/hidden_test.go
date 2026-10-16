package level

import (
	"context"
	"strings"
	"testing"
)

// faces hidden caulks what other brushes hide, by face, and only that: a
// wall's bottom on a floor, both ends of a block between two walls; a face
// behind glass shows, so stays.
func TestTexturingHiddenFacesCaulksWhatOthersHide(t *testing.T) {
	c, snap := zoning(t, 901)
	info := func(m string) MaterialInfo {
		return MaterialInfo{Known: true, Drawn: m != "caulk", Opaque: m != "glass"}
	}
	d, err := EditWith(context.Background(), c, snap, ops(t, `[
		{"box": {"min": [0, 0, -16], "max": [256, 256, 0], "material": "brick"}},
		{"box": {"min": [0, 0, 0], "max": [8, 256, 112], "material": "brick"}},
		{"box": {"min": [8, 100, 0], "max": [100, 108, 112], "material": "wood"}},
		{"box": {"min": [100, 0, 0], "max": [108, 256, 112], "material": "glass"}},
		{"texture": {"ids": ["@1", "@2"], "faces": "hidden"}}
	]`), "", nil, info)
	if err != nil {
		t.Fatal(err)
	}
	caulk := map[int][]string{}
	for _, e := range d.After.Map.Entities {
		for _, br := range e.Brushes() {
			for _, m := range d.Made {
				if hexOf(br.Guid()) != hexOf(m.ID) || m.Op == 0 || m.Op == 3 {
					continue
				}
				for dir, f := range faceTex(t, br) {
					if f.Material == "caulk" {
						caulk[m.Op] = append(caulk[m.Op], dir)
					}
				}
			}
		}
	}
	got := map[int]string{}
	for k, v := range caulk {
		got[k] = strings.Join(sortedStrings(v), ",")
	}
	if got[1] != "-z" || got[2] != "-x,-z" || len(got) != 2 { // the wall (op 1), the block (op 2)
		t.Errorf("caulked %v: the wall's bottom; the block's bottom and its end on the wall, not the one on the glass", got)
	}
	if _, err := Edit(context.Background(), c, snap, ops(t, `[{"box": {"min": [0, 0, 0], "max": [8, 8, 8], "material": "brick"}}, {"texture": {"ids": ["@0"], "faces": "hidden", "tile": [64, 64]}}]`), "", nil); err == nil || !strings.Contains(err.Error(), "takes a material only") {
		t.Errorf("hidden faces with a tile: %v", err)
	}
}

func sortedStrings(s []string) []string {
	out := append([]string(nil), s...)
	for i := range out {
		for j := i + 1; j < len(out); j++ {
			if out[j] < out[i] {
				out[i], out[j] = out[j], out[i]
			}
		}
	}
	return out
}
