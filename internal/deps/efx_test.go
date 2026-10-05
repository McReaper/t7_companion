package deps

import (
	"strings"
	"testing"

	"github.com/McReaper/t7_companion/internal/gdt"
)

func TestEfxRefs(t *testing.T) {
	src := "iwfx 3\n\n\tefPriority 0;\n{\n\tname \"def0\";\n\tfxOnImpact \"impacts/fx_hit\";\n\tfxOnDeath \"\";\n" +
		"\tbillboardSprite\n\t{\n\t\t\"gfx_smoke\"\n\t\t\"gfx_smoke_2\"\n\t};\n\telemSpawnSound\n\t{\n\t\t\"amb_moth\"\n\t};\n}\n" +
		"{\n\tname \"def1\";\n\tmodel\n\t{\n\t\t\"p7_debris\"\n\t};\n\trunner\n\t{\n\t\t\"smoke\\fx_child.efx\"\n\t};\n" +
		"\tlensFlare\n\t{\n\t\t\"9852f2c0-4665-43fb-97d1-067c7d91e9b4\"\n\t};\n}\n"
	var got []string
	for _, id := range efxRefs([]byte(src)) {
		got = append(got, id.String())
	}
	want := "fx impacts/fx_hit,material gfx_smoke,material gfx_smoke_2,xmodel p7_debris,fx smoke/fx_child,klf 9852f2c0-4665-43fb-97d1-067c7d91e9b4"
	if strings.Join(got, ",") != want {
		t.Errorf("got  %s\nwant %s", strings.Join(got, ","), want)
	}
}

// Only the camos and base materials a camo set's counts enable are read.
func TestEnabledCamos(t *testing.T) {
	fields := []gdt.Field{
		{Key: "numCamos", Value: "1"},
		{Key: "material1_1_numBaseMaterials", Value: "1"},
		{Key: "material1_1_base_material_1", Value: "mtl_base"},
		{Key: "material1_1_base_material_2", Value: "mtl_stale_base"},
		{Key: "material1_1_material", Value: "mtl_camo"},
		{Key: "material1_2_material", Value: "mtl_stale_camo"},
	}
	var got []string
	for _, f := range enabledCamos(fields) {
		if strings.HasPrefix(f.Value, "mtl_") { // the references; counts are kept too
			got = append(got, f.Value)
		}
	}
	if strings.Join(got, ",") != "mtl_base,mtl_camo" {
		t.Errorf("enabled: %v", got)
	}
}
