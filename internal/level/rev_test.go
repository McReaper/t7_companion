package level

import (
	"context"
	"strings"
	"testing"
)

// A rev is the text's, not its comments' or its spacing's.
func TestRevIgnoresCommentsAndSpacing(t *testing.T) {
	a := revOf([]byte("// brush 3\n{\n ( 0 0 0 ) ( 1 0 0 ) ( 0 1 0 ) caulk 64 64 0 0 0 0\n}\n"))
	if b := revOf([]byte("// brush 7\r\n{\r\n ( 0 0 0 ) ( 1 0 0 ) ( 0 1 0 ) caulk 64 64 0 0 0 0   \r\n}\r\n")); a != b {
		t.Errorf("renumbered and CRLF: %s, %s", a, b)
	}
	if b := revOf([]byte("// brush 3\n{\n ( 0 0 0 ) ( 1 0 0 ) ( 0 1 0 ) brick 64 64 0 0 0 0\n}\n")); a == b {
		t.Error("another material, the same rev")
	}
	if len(a) != 8 {
		t.Errorf("rev %q: 8 hex digits", a)
	}
}

// map_edit refuses, before sending anything, a batch naming what changed or
// went since it was read, or (required) what it names unread; what the batch
// makes itself (@n) needs no rev.
func TestEditRefusesWhatChangedSinceItWasRead(t *testing.T) {
	c, snap := zoning(t, 990)
	brush := snap.Map.Entities[0].Brushes()[0].Guid() // a world brush
	probe := snap.Map.Entities[1].Guid()              // a reflection_probe
	var sunBrush string                               // the volume_sun's brush, named alone
	for _, e := range snap.Map.Entities {
		if e.Class() == "volume_sun" {
			sunBrush = e.Brushes()[0].Guid()
		}
	}
	id := func(g string) string { return snap.IDs.Short(g) }
	rev := map[string]string{id(brush): snap.RevOf(brush), id(probe): snap.RevOf(probe), id(sunBrush): snap.RevOf(sunBrush)}
	if rev[id(brush)] == "" || rev[id(probe)] == "" || rev[id(sunBrush)] == "" || snap.RevOf("{00000000-0000-0000-0000-000000000000}") != "" {
		t.Fatalf("revs %v", rev)
	}
	move := `[{"move": {"ids": ["` + id(brush) + `", "` + id(sunBrush) + `"], "by": [0, 0, 8]}}, {"set": {"id": "` + id(probe) + `", "keys": {"ao_range": "4"}}}]`
	if _, err := EditWith(context.Background(), c, snap, ops(t, move), "", nil, nil, WithRevs(rev, true)); err != nil {
		t.Fatalf("revs as read: %v", err)
	}
	for want, w := range map[string]struct {
		revs map[string]string
		ops  string
	}{
		id(brush) + ": changed since it was read": {map[string]string{id(brush): "deadbeef", id(sunBrush): rev[id(sunBrush)], id(probe): rev[id(probe)]}, move},
		"0badc0de: gone": {map[string]string{"0badc0de": "x", id(brush): rev[id(brush)], id(sunBrush): rev[id(sunBrush)], id(probe): rev[id(probe)]}, move},
		"op 1's " + id(probe) + ": named without the rev": {map[string]string{id(brush): rev[id(brush)], id(sunBrush): rev[id(sunBrush)]}, move},
	} {
		_, c2 := zoning(t, 991)
		_, err := EditWith(context.Background(), c, c2, ops(t, w.ops), "", nil, nil, WithRevs(w.revs, true))
		if err == nil || !strings.Contains(err.Error(), want) || !strings.Contains(err.Error(), "Nothing was sent") {
			t.Errorf("%q: %v", want, err)
		}
	}
	made := `[{"box": {"min": [0, 0, 0], "max": [8, 8, 8], "material": "caulk"}}, {"move": {"ids": ["@0"], "by": [8, 0, 0]}}]`
	c3, s3 := zoning(t, 992)
	if _, err := EditWith(context.Background(), c3, s3, ops(t, made), "", nil, nil, WithRevs(nil, true)); err != nil {
		t.Errorf("@n needs no rev: %v", err)
	}
	c4, s4 := zoning(t, 993)
	if _, err := EditWith(context.Background(), c4, s4, ops(t, move), "", nil, nil, WithRevs(nil, false)); err != nil {
		t.Errorf("revs not required: %v", err)
	}
	withWorld := map[string]string{id(snap.Map.Entities[0].Guid()): "x"} // the worldspawn's rev, as map_find gives it beside its brushes
	for k, v := range rev {
		withWorld[k] = v
	}
	c5, s5 := zoning(t, 994)
	if _, err := EditWith(context.Background(), c5, s5, ops(t, move), "", nil, nil, WithRevs(withWorld, true)); err != nil {
		t.Errorf("the worldspawn's rev is ignored: %v", err)
	}
}

// Every op that names the map's entities or brushes is checked: deletes, keys,
// moves, materials, textures, copies and a zone's floors.
func TestEveryOpThatNamesIDsIsChecked(t *testing.T) {
	o := ops(t, `[{"delete": ["a1"]}, {"set": {"id": "b2", "keys": {"k": "v"}}}, {"move": {"ids": ["c3"], "by": [0, 0, 1]}},
		{"material": {"ids": ["d4"], "material": "caulk"}}, {"texture": {"ids": ["e5"]}}, {"copy": {"ids": ["f6"], "by": [1, 0, 0]}},
		{"zone": {"name": "z", "areas": [{"floors": ["g7"]}]}}, {"box": {"min": [0, 0, 0], "max": [1, 1, 1], "material": "caulk"}}]`)
	var got []string
	for _, op := range o {
		got = append(got, op.names()...)
	}
	if strings.Join(got, " ") != "a1 b2 c3 d4 e5 f6 g7" {
		t.Errorf("names %v", got)
	}
}
