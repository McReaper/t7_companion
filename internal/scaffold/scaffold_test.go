package scaffold

import (
	"bytes"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// fakeRoot holds two map templates and the mod template, laid out like rex/templates.
func fakeRoot(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	write := func(rel string, body []byte) {
		p := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, body, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	zm := "rex/templates/ZM Mod Level/"
	write(zm+"map_source/zm/template.map", []byte("iwmap 4\r\n// entity 0\r\n{\r\nguid \"{156D057A-194E-11E5-9C7D-40A8F0C9B644}\"\r\n\"classname\" \"worldspawn\"\r\n}\r\n{\r\n guid \"{F9C7D8BE-1008-44E7-BFC0-62D6ABA70286}\"\r\n}\r\n"))
	write(zm+"usermaps/template/zone_source/template.zone", []byte(">class,zm_mod_level\r\nscriptparsetree,scripts/zm/template.gsc\r\n"))
	write(zm+"usermaps/template/scripts/zm/template.gsc", []byte("#using scripts\\zm\\template_fx;\nfunction main() { level.template = 1; }"))
	write(zm+"usermaps/template/zone/loadingimage.png", []byte("\x89PNG\r\n\x1a\ntemplate\x00guid"))
	write("rex/templates/MP Mod Level/usermaps/template/zone_source/template.zone", []byte(">class,mp_mod_level\n"))
	for _, z := range ModZones {
		write("rex/templates/Mod/mods/template/zone_source/"+z+"_mod.zone", []byte("// "+z+"\n"))
	}
	return root
}

func read(t *testing.T, root, rel string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestCreateMap(t *testing.T) {
	root := fakeRoot(t)
	p, err := NewPlan(root, "ZM_Leviathan", "", nil) // lower-cased, template from the prefix
	if err != nil {
		t.Fatal(err)
	}
	if p.Name != "zm_leviathan" || p.Kind != "map" || p.Template != "ZM Mod Level" || len(p.Conflicts) != 0 {
		t.Fatalf("plan: %+v", p)
	}
	var paths []string
	for _, f := range p.Files {
		paths = append(paths, f.Path)
	}
	want := "map_source/zm/zm_leviathan.map usermaps/zm_leviathan/scripts/zm/zm_leviathan.gsc usermaps/zm_leviathan/zone/loadingimage.png usermaps/zm_leviathan/zone_source/zm_leviathan.zone"
	if got := strings.Join(paths, " "); got != want {
		t.Fatalf("files:\n%s\nwant\n%s", got, want)
	}
	if err := p.Write(); err != nil {
		t.Fatal(err)
	}

	zone := read(t, root, "usermaps/zm_leviathan/zone_source/zm_leviathan.zone")
	if string(zone) != ">class,zm_mod_level\r\nscriptparsetree,scripts/zm/zm_leviathan.gsc\r\n" {
		t.Errorf("zone (template replaced, CRLF kept): %q", zone)
	}
	if gsc := read(t, root, "usermaps/zm_leviathan/scripts/zm/zm_leviathan.gsc"); !bytes.Contains(gsc, []byte("level.zm_leviathan = 1; }")) {
		t.Errorf("the last line, without a line break, is filled too: %q", gsc)
	}
	if png := read(t, root, "usermaps/zm_leviathan/zone/loadingimage.png"); string(png) != "\x89PNG\r\n\x1a\ntemplate\x00guid" {
		t.Errorf("a binary file must be copied as is: %q", png)
	}
	m := string(read(t, root, "map_source/zm/zm_leviathan.map"))
	guids := regexp.MustCompile(`guid "(\{[0-9A-F]{8}-[0-9A-F]{4}-4[0-9A-F]{3}-[89AB][0-9A-F]{3}-[0-9A-F]{12}\})"`).FindAllStringSubmatch(m, -1)
	if len(guids) != 2 || guids[0][1] == guids[1][1] || strings.Contains(m, "156D057A") || !strings.Contains(m, "\r\n guid") {
		t.Errorf("every guid replaced by a fresh, distinct one, line layout kept: %q", m)
	}

	// a second create with the same name finds every file and writes nothing
	again, err := NewPlan(root, "zm_leviathan", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(again.Conflicts) != 4 || again.Write() == nil {
		t.Errorf("an existing map must not be overwritten: %+v", again.Conflicts)
	}
}

func TestCreateRules(t *testing.T) {
	root := fakeRoot(t)
	for _, c := range []struct{ name, template, want string }{
		{"zm_factory", "", "shipped map"},
		{"zm-bad", "", "letters, digits and _"},
		{"leviathan", "ZM Mod Level", "must start with zm_"},
		{"zm_x", "MP Mod Level", "must start with mp_"},
		{"zm_x", "ZM Fancy", "no template"},
	} {
		if _, err := NewPlan(root, c.name, c.template, nil); err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s/%s: %v, want %q", c.name, c.template, err, c.want)
		}
	}
	if p, err := NewPlan(root, "mp_dust", "", nil); err != nil || p.Template != "MP Mod Level" {
		t.Errorf("mp_ picks the MP template: %+v %v", p, err)
	}
}

func TestCreateModZones(t *testing.T) {
	root := fakeRoot(t)
	p, err := NewPlan(root, "my_mod", "", []string{"zm", "core"})
	if err != nil {
		t.Fatal(err)
	}
	var paths []string
	for _, f := range p.Files {
		paths = append(paths, f.Path)
	}
	if p.Kind != "mod" || strings.Join(paths, " ") != "mods/my_mod/zone_source/core_mod.zone mods/my_mod/zone_source/zm_mod.zone" {
		t.Errorf("only the chosen zones: %+v %v", p, paths)
	}
	if _, err := NewPlan(root, "my_mod", "", []string{"sp"}); err == nil {
		t.Error("an unknown zone must be an error")
	}
}

// A write that fails midway removes what it had written.
func TestCreateRollsBack(t *testing.T) {
	root := fakeRoot(t)
	p, err := NewPlan(root, "zm_half", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	// a file appears where the last destination goes, after the plan was made
	last := filepath.Join(root, filepath.FromSlash(p.Files[len(p.Files)-1].Path))
	if err := os.MkdirAll(filepath.Dir(last), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(last, []byte("theirs"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := p.Write(); err == nil {
		t.Fatal("want an error")
	}
	if _, err := os.Stat(filepath.Join(root, "map_source", "zm", "zm_half.map")); err == nil {
		t.Error("files written before the failure must be removed")
	}
	if _, err := os.Stat(filepath.Join(root, "usermaps", "zm_half", "scripts")); err == nil {
		t.Error("directories the write created must be removed too")
	}
	if b := read(t, root, p.Files[len(p.Files)-1].Path); string(b) != "theirs" {
		t.Error("the file that was in the way must be left alone")
	}
}
