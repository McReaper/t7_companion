package gdt

import (
	"slices"
	"testing"
)

// A Globals() block whose renderFlags opens a block of its own still gives
// the keys after it, and a techset takes the keys it doesn't set from the
// techset it includes; its own win.
func TestTechsetGlobalsNestedAndIncluded(t *testing.T) {
	root := techsetTreeFor(t, map[string]string{
		"weapons/reticle.techsetdef": `#include "state.techsetdef"
Globals()
{
	category = "Weapons"
	renderFlags = RenderFlags() : "emissive transparent"
	{
		postBlur = true
	}
	availablePrefixes = "mc/ mcs/"
}
`,
		"state.techsetdef": "State( \"x\" ) { }\n",
		"effect/base_blend.techsetdef": `Globals()
{
	category = "Effect"
	renderFlags = "emissive"
	availablePrefixes = "ei/ el/"
}
`,
		"effect/trail.techsetdef":      "#include \"base_blend\"\n",
		"effect/trail_bare.techsetdef": "#include \"base_blend\"\nGlobals()\n{\n\tavailablePrefixes = \"\"\n}\n",
		"effect/trail_mine.techsetdef": "#include \"base_blend\"\nGlobals()\n{\n\tcategory = \"Mine\"\n}\n",
		"2d/hud.techsetdef":            "Globals()\n{\n\tcategory = \"2D\"\n}\n",
		"geometry/decal.techsetdef":    "Globals()\n{\n\tcategory = \"Decal\"\n\tavailablePrefixes = \"mc/ wc/ / vd/\"\n}\n",
	})
	ts, err := OpenTechsets(root)
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		typ, category string
		prefixes      []string
		bare          bool
	}{
		{"reticle", "Weapons", []string{"mc/", "mcs/"}, false},
		{"trail", "Effect", []string{"ei/", "el/"}, false},
		{"trail_bare", "Effect", nil, true},                   // its own "" wins over the include's
		{"trail_mine", "Mine", []string{"ei/", "el/"}, false}, // the include's keys after one it sets still count
		{"hud", "2D", nil, true},
		{"decal", "Decal", []string{"mc/", "wc/", "/", "vd/"}, true},
	}
	for _, c := range cases {
		got, err := ts.Resolve(c.typ)
		if err != nil {
			t.Fatal(err)
		}
		if got.Category != c.category || !slices.Equal(got.Prefixes, c.prefixes) || got.BuildsBare() != c.bare {
			t.Errorf("%s: category %q prefixes %q bare %v, want %q %q %v", c.typ, got.Category, got.Prefixes, got.BuildsBare(), c.category, c.prefixes, c.bare)
		}
	}
	for _, typ := range []string{"trail", "trail_mine"} {
		if trail, _ := ts.Resolve(typ); trail.RenderFlags != "emissive" {
			t.Errorf("%s: render flags from the include: %q", typ, trail.RenderFlags)
		}
	}
}
