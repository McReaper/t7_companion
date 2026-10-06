package cli

import (
	"strings"
	"testing"
)

// Before any link: what the zone pulls in, the reference no source defines,
// the GDT versions stock replaces, one line, and why an asset is pulled in.
func TestZonePredict(t *testing.T) {
	root := sourcesRoot(t)
	all, err := zonePredict(root, "zm_x", "", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if all.ByType["weapon"] != 1 || all.ByType["material"] != 1 || all.ByType["xmodel"] != 1 {
		t.Errorf("by type: %+v", all.ByType)
	}
	if len(all.Dangling) != 1 || all.Dangling[0].Asset != "xmodel gun_world" || all.Dangling[0].NamedBy != "weapon my_gun" {
		t.Errorf("no source: %+v", all.Dangling)
	}
	if len(all.Masked) != 1 || all.Masked[0].GDT != "source_data/mine.gdt" || all.Masked[0].Assets != 2 {
		t.Errorf("GDT versions stock replaces: %+v", all.Masked)
	}

	line, err := zonePredict(root, "zm_x", "my_gun", "", "") // the type comes from the zone line
	if err != nil || line.Line != "weapon my_gun" || strings.Join(line.Direct, ",") != "xmodel gun_world" {
		t.Errorf("one line: %+v, %v", line, err)
	}

	why, err := zonePredict(root, "zm_x", "", "gun_world", "")
	if err != nil || strings.Join(why.PulledInBy, " < ") != "xmodel gun_world < weapon my_gun < zone line weapon my_gun" {
		t.Errorf("why: %+v, %v", why, err)
	}
	if out, _ := zonePredict(root, "zm_x", "", "nothing", ""); out == nil || out.NotPulled == "" {
		t.Errorf("an asset nothing pulls in says so: %+v", out)
	}
	if _, err := zonePredict(root, "zm_x", "nothing", "", ""); err == nil {
		t.Error("a line no zone line or GDT names needs its type")
	}
}
