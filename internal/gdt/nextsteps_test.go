package gdt

import (
	"strings"
	"testing"
)

// After a write, a created asset is zoned unless something else carries it:
// an image (its material), a material (what uses it), an ssi (the map's bake:
// no link of a map lit by its own ssi packs one, verified on real links).
func TestNextStepsZoneOnlyWhatTheLinkerPacks(t *testing.T) {
	created := []*EditResult{{Type: "xmodel", Asset: "m"}, {Type: "ssi", Asset: "s"}, {Type: "image", Asset: "i"}, {Type: "material", Asset: "mt"}}
	next := strings.Join(nextSteps(created), "\n")
	for _, want := range []string{"Add `xmodel,m` to the map/mod .zone", `Don't zone ssi "s"`, `Don't zone material "mt"`} {
		if !strings.Contains(next, want) {
			t.Errorf("lacks %q:\n%s", want, next)
		}
	}
	for _, not := range []string{"`ssi,s`", "`image,i`", "`material,mt`"} {
		if strings.Contains(next, not) {
			t.Errorf("asks to zone %s:\n%s", not, next)
		}
	}
}
