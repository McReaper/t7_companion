package zone

import (
	"bufio"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// The linker writes the class chain it resolved at the head of every .deps:
// Inherit must find the same lists, for every map of a real install that has
// been linked. Runs when TA_TOOLS_PATH names one.
func TestInheritMatchesLinker(t *testing.T) {
	root := os.Getenv("TA_TOOLS_PATH")
	if root == "" {
		t.Skip("TA_TOOLS_PATH not set")
	}
	maps, _ := filepath.Glob(filepath.Join(root, "usermaps", "*", "zone_source", "all", "assetinfo"))
	checked := 0
	for _, dir := range maps {
		name := filepath.Base(filepath.Dir(filepath.Dir(filepath.Dir(dir))))
		want, ok := depsHead(t, filepath.Join(dir, name+".deps"))
		if !ok {
			continue
		}
		zf, err := ZoneFile(root, name)
		if err != nil {
			continue
		}
		got, err := Inherit(root, zf)
		if err != nil {
			t.Errorf("%s: %v", name, err)
			continue
		}
		checked++
		if !sameSet(got.Ignore, want.Ignore) || !sameSet(got.IgnoreMissingShipped, want.IgnoreMissingShipped) {
			t.Errorf("%s: resolved %+v, the linker wrote %+v", name, got, want)
		}
	}
	t.Logf("%d linked maps checked", checked)
}

// depsHead reads the ignore lines the linker wrote before the first asset.
func depsHead(t *testing.T, path string) (Inherited, bool) {
	f, err := os.Open(path)
	if err != nil {
		return Inherited{}, false
	}
	defer f.Close()
	var inh Inherited
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		kind, list, _ := strings.Cut(sc.Text(), ",")
		switch kind {
		case "version":
		case "ignore":
			inh.Ignore = append(inh.Ignore, list)
		case "ignore_missing_shipped":
			inh.IgnoreMissingShipped = append(inh.IgnoreMissingShipped, list)
		default:
			return inh, true
		}
	}
	return inh, true
}

func sameSet(a, b []string) bool {
	a, b = slices.Clone(a), slices.Clone(b)
	slices.Sort(a)
	slices.Sort(b)
	return slices.Equal(slices.Compact(a), slices.Compact(b))
}
