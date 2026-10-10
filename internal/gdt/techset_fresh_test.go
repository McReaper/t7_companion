package gdt

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"
)

// techsetTreeFor writes techsetdefs under a temp root, by path relative to it.
func techsetTreeFor(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	for rel, src := range files {
		writeTechset(t, root, rel, src)
	}
	return root
}

func writeTechset(t *testing.T, root, rel, src string) {
	t.Helper()
	p := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
}

// later sets a file's modification time an hour ahead, as an edit that the
// file system's clock granularity could otherwise hide.
func later(t *testing.T, p string) {
	t.Helper()
	at := time.Now().Add(time.Hour)
	if err := os.Chtimes(p, at, at); err != nil {
		t.Fatal(err)
	}
}

const oneSlot = `Globals() { category = "Geometry" }
Texture( "colorMap" ) { image = Image( <colorMap, $white> ) }
`

// A techsetdef written after the index was built is found: an unknown
// material type rescans the tree, at most every rescanEvery.
func TestTechsetsSeeANewTechsetdef(t *testing.T) {
	root := techsetTreeFor(t, map[string]string{"geometry/old.techsetdef": oneSlot})
	ts, err := OpenTechsets(root)
	if err != nil {
		t.Fatal(err)
	}
	writeTechset(t, root, "geometry/custom/new.techsetdef", oneSlot)
	if ts.Exists("new") {
		t.Fatal("a rescan inside rescanEvery of the last one")
	}
	ts.rescanEvery = 0
	if !ts.Exists("new") {
		t.Fatal("the new techsetdef isn't found")
	}
	if r, err := ts.Resolve("new"); err != nil || len(r.Textures) != 1 {
		t.Fatalf("Resolve = %+v, %v", r, err)
	}
	if got := ts.Names(); !slices.Equal(got, []string{"new", "old"}) {
		t.Errorf("Names = %v", got)
	}
}

// An edited techsetdef is read again: a resolved techset remembers the files
// it was read from, and the stamps are checked at most every restatEvery.
func TestTechsetsRereadAnEditedTechsetdef(t *testing.T) {
	root := techsetTreeFor(t, map[string]string{"geometry/mine.techsetdef": oneSlot})
	ts, err := OpenTechsets(root)
	if err != nil {
		t.Fatal(err)
	}
	if r, _ := ts.Resolve("mine"); len(r.Textures) != 1 {
		t.Fatalf("textures %+v", r.Textures)
	}
	writeTechset(t, root, "geometry/mine.techsetdef", oneSlot+`Texture( "normalMap" ) { image = Image( <normalMap, $identitynormalmap> ) }
`)
	later(t, filepath.Join(root, "geometry/mine.techsetdef"))
	if r, _ := ts.Resolve("mine"); len(r.Textures) != 1 {
		t.Fatal("stamps checked inside restatEvery of the last check")
	}
	ts.restatEvery = 0
	if r, _ := ts.Resolve("mine"); len(r.Textures) != 2 {
		t.Fatalf("the edit isn't read: %+v", r.Textures)
	}
}

// An included file's edit is read again too.
func TestTechsetsRereadAnEditedInclude(t *testing.T) {
	root := techsetTreeFor(t, map[string]string{
		"include/slots.techsetdef": oneSlot,
		"geometry/mine.techsetdef": `#include "slots"` + "\n",
	})
	ts, err := OpenTechsets(root)
	if err != nil {
		t.Fatal(err)
	}
	ts.restatEvery = 0
	if r, _ := ts.Resolve("mine"); len(r.Textures) != 1 {
		t.Fatalf("textures %+v", r.Textures)
	}
	writeTechset(t, root, "include/slots.techsetdef", oneSlot+`Texture( "specularMap" ) { image = Image( <specColorMap, $black> ) }
`)
	later(t, filepath.Join(root, "include/slots.techsetdef"))
	if r, _ := ts.Resolve("mine"); len(r.Textures) != 2 {
		t.Fatalf("the include's edit isn't read: %+v", r.Textures)
	}
}

// A deleted techsetdef is forgotten: a resolved one at once, and any of them
// once the tree is walkEvery old.
func TestTechsetsForgetADeletedTechsetdef(t *testing.T) {
	root := techsetTreeFor(t, map[string]string{
		"geometry/resolved.techsetdef": oneSlot,
		"geometry/listed.techsetdef":   oneSlot,
	})
	ts, err := OpenTechsets(root)
	if err != nil {
		t.Fatal(err)
	}
	ts.restatEvery = 0
	if _, err := ts.Resolve("resolved"); err != nil {
		t.Fatal(err)
	}
	remove := func(name string) {
		if err := os.Remove(filepath.Join(root, "geometry", name+".techsetdef")); err != nil {
			t.Fatal(err)
		}
	}
	remove("listed")
	if !ts.Exists("listed") {
		t.Fatal("the tree rescanned before walkEvery")
	}
	ts.scanMu.Lock()
	ts.lastScan = time.Now().Add(-walkEvery)
	ts.scanMu.Unlock()
	if ts.Exists("listed") {
		t.Error("a deleted techsetdef still listed after walkEvery")
	}
	remove("resolved")
	if _, err := ts.Resolve("resolved"); err == nil || !strings.Contains(err.Error(), "no techsetdef") {
		t.Errorf("Resolve of a deleted techsetdef: %v", err)
	}
}

// A failed rescan keeps the tree it had.
func TestTechsetsKeepTheirTreeWhenARescanFails(t *testing.T) {
	root := techsetTreeFor(t, map[string]string{"geometry/kept.techsetdef": oneSlot})
	ts, err := OpenTechsets(root)
	if err != nil {
		t.Fatal(err)
	}
	ts.root = filepath.Join(root, "missing")
	ts.rescan(true)
	if !ts.has("kept") {
		t.Error("the tree was lost")
	}
}

// Lookups run alongside rescans: the MCP server answers in parallel.
func TestTechsetsConcurrentLookups(t *testing.T) {
	root := techsetTreeFor(t, map[string]string{"geometry/a.techsetdef": oneSlot})
	ts, err := OpenTechsets(root)
	if err != nil {
		t.Fatal(err)
	}
	ts.rescanEvery, ts.restatEvery = 0, 0
	var wg sync.WaitGroup
	for i := range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 50 {
				if _, err := ts.Resolve("a"); err != nil {
					t.Error(err)
					return
				}
				ts.Exists("missing" + string(rune('a'+i)))
				ts.Names()
			}
		}()
	}
	wg.Wait()
}

// A root that can't be walked is an error, not an empty index.
func TestOpenTechsetsMissingRoot(t *testing.T) {
	if _, err := OpenTechsets(filepath.Join(t.TempDir(), "missing")); err == nil {
		t.Error("no error")
	}
}

// A material type the index knows never walks the tree, however often it is
// looked up: only an unknown one may.
func TestTechsetsKnownTypeDoesNotRescan(t *testing.T) {
	root := techsetTreeFor(t, map[string]string{"geometry/known.techsetdef": oneSlot})
	ts, err := OpenTechsets(root)
	if err != nil {
		t.Fatal(err)
	}
	ts.rescanEvery = 0
	if err := os.Remove(filepath.Join(root, "geometry", "known.techsetdef")); err != nil {
		t.Fatal(err)
	}
	if !ts.Exists("known") {
		t.Error("a known material type rescanned the tree")
	}
}

// A techsetdef outside include/ wins over an include of the same name, in
// whichever order the walk meets them (postfx/ comes after include/).
func TestTechsetsPreferTheMaterialTypeOverAnInclude(t *testing.T) {
	root := techsetTreeFor(t, map[string]string{
		"include/shared.techsetdef": oneSlot,
		"postfx/shared.techsetdef":  oneSlot,
	})
	ts, err := OpenTechsets(root)
	if err != nil {
		t.Fatal(err)
	}
	if !ts.Exists("shared") {
		t.Error("the include shadows the material type")
	}
}

// An edit that keeps the size, or one that keeps the time, is still seen.
func TestTechsetsSeeAnEditByTimeOrBySize(t *testing.T) {
	sameSize := strings.Replace(oneSlot, "$white", "$black", 1)
	bigger := oneSlot + `Texture( "normalMap" ) { image = Image( <normalMap, $identitynormalmap> ) }
`
	for name, edit := range map[string]func(p string){
		"same size, later time": func(p string) {
			writeTechset(t, filepath.Dir(p), filepath.Base(p), sameSize)
			later(t, p)
		},
		"same time, other size": func(p string) {
			fi, err := os.Stat(p)
			if err != nil {
				t.Fatal(err)
			}
			writeTechset(t, filepath.Dir(p), filepath.Base(p), bigger)
			if err := os.Chtimes(p, fi.ModTime(), fi.ModTime()); err != nil {
				t.Fatal(err)
			}
		},
	} {
		root := techsetTreeFor(t, map[string]string{"geometry/mine.techsetdef": oneSlot})
		ts, err := OpenTechsets(root)
		if err != nil {
			t.Fatal(err)
		}
		ts.restatEvery = 0
		before, _ := ts.Resolve("mine")
		edit(filepath.Join(root, "geometry", "mine.techsetdef"))
		if after, _ := ts.Resolve("mine"); after == before {
			t.Errorf("%s: the edit isn't seen", name)
		}
	}
}

// A deleted techsetdef that was resolved rescans the tree once, not on every
// later lookup.
func TestTechsetsDeletedTypeRescansOnce(t *testing.T) {
	root := techsetTreeFor(t, map[string]string{"geometry/gone.techsetdef": oneSlot})
	ts, err := OpenTechsets(root)
	if err != nil {
		t.Fatal(err)
	}
	ts.restatEvery = 0
	if _, err := ts.Resolve("gone"); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(root, "geometry", "gone.techsetdef")); err != nil {
		t.Fatal(err)
	}
	if _, err := ts.Resolve("gone"); err == nil {
		t.Fatal("resolved a deleted techsetdef")
	}
	ts.scanMu.Lock()
	scanned := ts.lastScan
	ts.scanMu.Unlock()
	if _, err := ts.Resolve("gone"); err == nil {
		t.Fatal("resolved a deleted techsetdef")
	}
	ts.scanMu.Lock()
	defer ts.scanMu.Unlock()
	if !ts.lastScan.Equal(scanned) {
		t.Error("the second lookup rescanned the tree")
	}
}
