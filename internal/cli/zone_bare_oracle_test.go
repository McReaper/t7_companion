package cli

import (
	"bufio"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/McReaper/t7_companion/internal/zone"
)

// What zone_check's bare-material check rests on, against a real install
// (opt-in: T7KB_ORACLE=1 and TA_TOOLS_PATH; it resolves ~20,000 materials):
// every material a shipped fastfile holds (the stock assetlists) or a linked
// map's report shows was built under a prefix its techset declares, or bare
// when the techset builds bare or stock ships it bare; and no zone that linked
// since it last changed has a line the check reports.
func TestBareMaterialRuleMatchesLinker(t *testing.T) {
	root := os.Getenv("TA_TOOLS_PATH")
	if root == "" || os.Getenv("T7KB_ORACLE") == "" {
		t.Skip("set T7KB_ORACLE=1 and TA_TOOLS_PATH")
	}
	w, err := workspace(root)
	if err != nil {
		t.Fatal(err)
	}
	shipped, err := zone.BareTechsets(root)
	if err != nil {
		t.Fatal(err)
	}
	lists, _ := filepath.Glob(filepath.Join(root, "zone_source", "all", "assetlist", "*.csv"))
	reports, _ := filepath.Glob(filepath.Join(root, "usermaps", "*", "zone_source", "all", "assetinfo", "*.csv"))
	checked, unresolved := map[string]int{}, 0
	for _, file := range append(lists, reports...) {
		for _, name := range materialNames(t, file) {
			prefix, bare := "/", name
			if i := strings.LastIndexByte(name, '/'); i >= 0 {
				prefix, bare = name[:i+1], name[i+1:]
			}
			ts, err := w.MaterialTechset(bare)
			if err != nil {
				unresolved++
				continue
			}
			checked[prefix]++
			ok := slices.Contains(ts.Prefixes, prefix) || (prefix == "/" && (ts.BuildsBare() || shipped[strings.ToLower(ts.MaterialType)]))
			if !ok {
				t.Errorf("%s: %s built as %s, its techset %s declares %q", filepath.Base(file), bare, prefix, ts.MaterialType, ts.Prefixes)
			}
		}
	}
	t.Logf("materials checked by prefix %v; %d with no GDT or techsetdef", checked, unresolved)
	linkedZonesReportNothing(t, root, reports)
}

// linkedZonesReportNothing runs the check on every map whose last link is
// newer than its zone files: that zone linked, so no line of it may fail.
func linkedZonesReportNothing(t *testing.T, root string, reports []string) {
	maps := 0
	for _, rep := range reports {
		name := strings.TrimSuffix(filepath.Base(rep), ".csv")
		zf, err := zone.ZoneFile(root, name)
		if err != nil || !newerThanZone(t, rep, filepath.Dir(zf)) {
			continue
		}
		maps++
		for _, b := range bareMaterials(root, zf) {
			t.Errorf("%s linked, yet the check reports %+v", name, b)
		}
	}
	t.Logf("%d linked zones checked", maps)
}

func newerThanZone(t *testing.T, report, zoneDir string) bool {
	ri, err := os.Stat(report)
	if err != nil {
		t.Fatal(err)
	}
	src, _ := filepath.Glob(filepath.Join(zoneDir, "*.z*")) // .zone and .zpkg
	for _, s := range src {
		if si, err := os.Stat(s); err == nil && si.ModTime().After(ri.ModTime()) {
			return false
		}
	}
	return true
}

// materialNames reads the material names of an assetlist (commented-out lines
// too: they are shipped) or of a link report.
func materialNames(t *testing.T, path string) []string {
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	var out []string
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() {
		cols := strings.Split(strings.TrimPrefix(strings.TrimSpace(sc.Text()), "//"), ",")
		if len(cols) >= 3 && cols[1] == "material" { // report: index,type,name,…
			cols = cols[1:]
		}
		if len(cols) >= 2 && cols[0] == "material" {
			name, _, _ := strings.Cut(cols[1], "|") // a second packing is x|dup
			out = append(out, name)
		}
	}
	return out
}
