package zone

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// Inherited is what a zone takes from its >class chain: the stock assetlists
// whose assets the shipped fastfiles already provide.
//
// For an active entry of an Ignore list, the linker writes only a reference to
// the shipped asset (a few dozen bytes, nothing streamed), so a version of that
// asset in your own GDTs or files is not what ships. Commenting the entry out
// (`//`) makes the linker pack yours. IgnoreMissingShipped lists may be absent
// from the build without an error.
type Inherited struct {
	Ignore               []string `json:"ignore"`
	IgnoreMissingShipped []string `json:"ignore_missing_shipped"`
}

// ZoneFile is <map>/zone_source/<map>.zone (or the mod's).
func ZoneFile(root, name string) (string, error) {
	for _, kind := range []string{"usermaps", "mods"} {
		p := filepath.Join(root, kind, name, "zone_source", name+".zone")
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
	}
	return "", fmt.Errorf("no zone file for %q under usermaps/ or mods/", name)
}

// Inherit resolves a zone's >class chain the way the linker does: each class
// from share/zone_source/<class>.class, following `#include "x.class"`.
func Inherit(root, zoneFile string) (Inherited, error) {
	lines, err := readLines(zoneFile)
	if err != nil {
		return Inherited{}, err
	}
	var inh Inherited
	seen := map[string]bool{}
	for _, l := range lines {
		if class, ok := strings.CutPrefix(l, ">class,"); ok {
			if err := inh.class(root, strings.TrimSpace(class), seen); err != nil {
				return inh, err
			}
		}
	}
	return inh, nil
}

func (inh *Inherited) class(root, name string, seen map[string]bool) error {
	name = strings.TrimSuffix(name, ".class")
	if seen[strings.ToLower(name)] {
		return nil
	}
	seen[strings.ToLower(name)] = true
	lines, err := readLines(filepath.Join(root, "share", "zone_source", name+".class"))
	if err != nil {
		return fmt.Errorf("class %s: %w", name, err)
	}
	for _, l := range lines {
		if inc, ok := strings.CutPrefix(l, "#include"); ok {
			if err := inh.class(root, strings.Trim(strings.TrimSpace(inc), `"`), seen); err != nil {
				return err
			}
			continue
		}
		kind, list, ok := strings.Cut(l, ",")
		list = strings.TrimSpace(list)
		switch {
		case !ok:
		case kind == "ignore" && !slices.Contains(inh.Ignore, list):
			inh.Ignore = append(inh.Ignore, list)
		case kind == "ignore_missing_shipped" && !slices.Contains(inh.IgnoreMissingShipped, list):
			inh.IgnoreMissingShipped = append(inh.IgnoreMissingShipped, list)
		}
	}
	return nil
}

// readLines returns a zone-source file's lines, trimmed, without blank lines
// and `//` comments.
func readLines(path string) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var out []string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		l := strings.TrimSpace(sc.Text())
		if l != "" && !strings.HasPrefix(l, "//") {
			out = append(out, l)
		}
	}
	return out, sc.Err()
}

// ListEntry is where an asset appears in a stock assetlist.
type ListEntry struct {
	List   string `json:"list"`
	Line   int    `json:"line"`
	Active bool   `json:"active"` // false: the line is commented out with //
}

// File is the entry's path relative to the mod-tools root.
func (e ListEntry) File() string { return "zone_source/all/assetlist/" + e.List + ".csv" }

// Assetlists indexes stock assetlist entries by lower-cased type and name.
type Assetlists map[Ref][]ListEntry

// LoadAssetlists reads zone_source/all/assetlist/<list>.csv for each list:
// `type,name` lines, commented-out ones included (that is how an override is made).
func LoadAssetlists(root string, lists []string) (Assetlists, error) {
	out := Assetlists{}
	for _, list := range lists {
		f, err := os.Open(filepath.Join(root, "zone_source", "all", "assetlist", list+".csv"))
		if err != nil {
			return nil, err
		}
		sc := bufio.NewScanner(f)
		for n := 1; sc.Scan(); n++ {
			l := strings.TrimSpace(sc.Text())
			body, commented := strings.CutPrefix(l, "//")
			typ, name, ok := strings.Cut(strings.TrimSpace(body), ",")
			if !ok || typ == "" || name == "" {
				continue
			}
			key := Ref{Type: strings.ToLower(strings.TrimSpace(typ)), Name: strings.ToLower(strings.TrimSpace(name))}
			out[key] = append(out[key], ListEntry{List: list, Line: n, Active: !commented})
		}
		err = sc.Err()
		f.Close()
		if err != nil {
			return nil, err
		}
	}
	return out, nil
}

// Lookup returns the entries for an asset.
func (a Assetlists) Lookup(r Ref) []ListEntry {
	return a[Ref{Type: strings.ToLower(r.Type), Name: strings.ToLower(r.Name)}]
}

// Line is an asset line of a zone source file.
type Line struct {
	Ref
	File string `json:"file"`
	N    int    `json:"line"`
}

// ZoneLines returns the asset lines of a zone file and of the .zpkg packages it
// includes (`include,<name>`), each looked up beside the zone first, then in
// share/zone_source, as the linker does. Directives (`>…`) and the ignore lines
// are left out.
func ZoneLines(root, zoneFile string) ([]Line, error) {
	var out []Line
	err := zoneLines(root, filepath.Dir(zoneFile), zoneFile, map[string]bool{}, &out)
	return out, err
}

func zoneLines(root, mapDir, path string, seen map[string]bool, out *[]Line) error {
	key := strings.ToLower(filepath.Clean(path))
	if seen[key] {
		return nil
	}
	seen[key] = true
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for n := 1; sc.Scan(); n++ {
		l := strings.TrimSpace(sc.Text())
		if l == "" || strings.HasPrefix(l, "//") || strings.HasPrefix(l, ">") || strings.HasPrefix(l, "#") {
			continue
		}
		typ, name, ok := strings.Cut(l, ",")
		typ, name = strings.TrimSpace(typ), strings.TrimSpace(name)
		switch {
		case !ok || name == "", typ == "ignore", typ == "ignore_missing_shipped":
		case typ == "include":
			pkg := findPackage(root, mapDir, name)
			if pkg == "" {
				continue // the linker reports a missing package itself
			}
			if err := zoneLines(root, mapDir, pkg, seen, out); err != nil {
				return err
			}
		default:
			*out = append(*out, Line{Ref: Ref{Type: typ, Name: name}, File: path, N: n})
		}
	}
	return sc.Err()
}

func findPackage(root, mapDir, name string) string {
	for _, dir := range []string{mapDir, filepath.Join(root, "share", "zone_source")} {
		p := filepath.Join(dir, name+".zpkg")
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
}
