package zone

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

// A report as the linker writes it: header, then index,type,name,resident,streamed,parentStack.
const sampleCSV = `index,type,name,resident,streamed,parentStack
1,string,,198877,0,||assetlist
2,image,fxt_fog,22,0,|ei/fog_mtl|material|fx/fog.efx|fx|fury.zpkg|csv|zone_source/zm_x.zone|csv
3,material,ei/fog_mtl,500,0,|fx/fog.efx|fx|fury.zpkg|csv|zone_source/zm_x.zone|csv
4,xmodel,skybox_default_day,92,0,|zone_source/zm_x.zone|csv
5,xmodelmesh,t6_skybox,224,11616,|skybox_default_day|xmodel|zone_source/zm_x.zone|csv
6,image,probe_0,10,4000,|zm_x|texturecombo
7,image,fxt_fog,30,0,|other_mtl|material|zone_source/zm_x.zone|csv
`

func writeReport(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "zm_x.csv"), []byte(sampleCSV), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestLoadAndChains(t *testing.T) {
	r, err := Load(writeReport(t), "zm_x")
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Assets) != 7 {
		t.Fatalf("got %d assets, want 7", len(r.Assets))
	}
	img := r.Find("FXT_FOG", "image") // names match case-insensitively
	if len(img) != 2 {
		t.Fatalf("an asset pulled in twice is listed twice, got %d", len(img))
	}
	want := []Ref{{"material", "ei/fog_mtl"}, {"fx", "fx/fog.efx"}, {"csv", "fury.zpkg"}, {"csv", "zone_source/zm_x.zone"}}
	if !reflect.DeepEqual(img[0].Chain, want) {
		t.Errorf("chain = %v, want %v (nearest parent first)", img[0].Chain, want)
	}

	for name, line := range map[string]string{
		"fxt_fog":            "fury.zpkg",                 // through an included package
		"skybox_default_day": "xmodel skybox_default_day", // listed in the zone itself
		"t6_skybox":          "xmodel skybox_default_day", // its zone line, not itself
		"probe_0":            "texturecombo zm_x",         // a chain that doesn't reach the zone: its root
	} {
		if got := r.Find(name, "")[0].Line().String(); got != line {
			t.Errorf("%s: line %q, want %q", name, got, line)
		}
	}
}

func TestUnder(t *testing.T) {
	r, err := Load(writeReport(t), "zm_x")
	if err != nil {
		t.Fatal(err)
	}
	names := func(ps []Packed) string {
		var s []string
		for _, p := range ps {
			s = append(s, p.Name)
		}
		return strings.Join(s, ",")
	}
	if got := names(r.Under(Ref{Name: "fury.zpkg"})); got != "fxt_fog,ei/fog_mtl" {
		t.Errorf("under the package: %s", got)
	}
	if got := names(r.Under(Ref{Type: "xmodel", Name: "skybox_default_day"})); got != "skybox_default_day,t6_skybox" {
		t.Errorf("under the xmodel line (itself included): %s", got)
	}
	if got := r.Under(Ref{Type: "image", Name: "skybox_default_day"}); len(got) != 0 {
		t.Errorf("a typed ref matches its type only, got %s", names(got))
	}
}

func TestChanged(t *testing.T) {
	dir := t.TempDir()
	linked := time.Date(2026, 6, 2, 23, 25, 0, 0, time.UTC)
	same, edited := filepath.Join(dir, "same.gsc"), filepath.Join(dir, "edited.gsc")
	for _, f := range []string{same, edited} {
		if err := os.WriteFile(f, nil, 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.Chtimes(f, linked, linked); err != nil {
			t.Fatal(err)
		}
	}
	later := linked.Add(time.Hour)
	if err := os.Chtimes(edited, later, later); err != nil {
		t.Fatal(err)
	}
	gone := filepath.Join(dir, "gone.gsc")
	stamp := "1780442700" // linked, in Unix seconds
	deps := "version,1,2\nscriptparsetree,scripts/a.gsc\n" +
		"\tfile," + same + "," + stamp + ",ab,cd\n" +
		"\tfile," + edited + "," + stamp + ",\n" +
		"\tfile," + same + "," + stamp + ",\n" + // listed twice: checked once
		"\tfile," + gone + "," + stamp + ",\n" +
		"\tgdt,image,a,E8-73\n"
	if err := os.WriteFile(filepath.Join(dir, "zm_x.deps"), []byte(deps), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := Changed(dir, "zm_x")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{edited, gone}
	if strings.Compare(want[0], want[1]) > 0 {
		want[0], want[1] = want[1], want[0]
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("changed = %v, want %v (edited after the link, or deleted)", got, want)
	}
}

func TestReportDir(t *testing.T) {
	root := t.TempDir()
	want := filepath.Join(root, "usermaps", "zm_x", "zone_source", "all", "assetinfo")
	if err := os.MkdirAll(want, 0o755); err != nil {
		t.Fatal(err)
	}
	if got, err := ReportDir(root, "zm_x"); err != nil || got != want {
		t.Errorf("ReportDir = %q, %v", got, err)
	}
	if _, err := ReportDir(root, "zm_unlinked"); err == nil || !strings.Contains(err.Error(), "link it first") {
		t.Errorf("an unlinked map must say to link it first: %v", err)
	}
}
