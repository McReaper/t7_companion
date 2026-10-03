// Package zone reads what the BO3 linker reports about a zone it built. Every
// link writes, beside the fastfile, zone_source/all/assetinfo/<zone>.csv (each
// packed asset with its size and the chain of parents that pulled it in, up to
// the zone line) and <zone>.deps (every source file the link read, with the
// timestamp it had then).
package zone

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Ref names an asset, or a zone file or zone package (type "csv") in a chain.
type Ref struct{ Type, Name string }

func (r Ref) String() string {
	if r.Type == "csv" || r.Type == "" {
		return r.Name
	}
	return r.Type + " " + r.Name
}

// Packed is one asset the link wrote into the fastfile.
type Packed struct {
	Ref
	Resident, Streamed int64 // bytes loaded with the zone, bytes streamed from disk
	Chain              []Ref // parents, nearest first; the last is the zone file
}

// Line is what pulled the asset in at the top: the zone line (the chain entry
// just below the .zone file, or the asset itself when the zone lists it
// directly), or the chain's root when it doesn't reach the zone — the linker
// also packs assets for the map's texture combos and BSP volumes.
func (p Packed) Line() Ref {
	n := len(p.Chain)
	switch {
	case n == 0:
		return p.Ref
	case !isZoneFile(p.Chain[n-1]):
		return p.Chain[n-1]
	case n == 1:
		return p.Ref
	default:
		return p.Chain[n-2]
	}
}

func isZoneFile(r Ref) bool { return r.Type == "csv" && strings.HasSuffix(r.Name, ".zone") }

// Report is a linker's assetinfo report for one zone.
type Report struct {
	Zone   string
	Dir    string    // the assetinfo directory
	Linked time.Time // when the link wrote the report
	Assets []Packed
}

// Find returns the packed assets named name (case-insensitively), of type typ
// when typ is set.
func (r *Report) Find(name, typ string) []Packed {
	var out []Packed
	for _, p := range r.Assets {
		if strings.EqualFold(p.Name, name) && (typ == "" || strings.EqualFold(p.Type, typ)) {
			out = append(out, p)
		}
	}
	return out
}

// Under returns the assets that ref pulled in: those whose chain holds it, and
// ref itself. An empty ref type matches any type.
func (r *Report) Under(ref Ref) []Packed {
	var out []Packed
	for _, p := range r.Assets {
		if matches(p.Ref, ref) || containsRef(p.Chain, ref) {
			out = append(out, p)
		}
	}
	return out
}

func containsRef(chain []Ref, ref Ref) bool {
	for _, c := range chain {
		if matches(c, ref) {
			return true
		}
	}
	return false
}

func matches(r, want Ref) bool {
	return strings.EqualFold(r.Name, want.Name) && (want.Type == "" || strings.EqualFold(r.Type, want.Type))
}

// ReportDir is where the linker writes a zone's reports: usermaps/<name> for a
// map, mods/<name> for a mod.
func ReportDir(root, name string) (string, error) {
	var tried []string
	for _, kind := range []string{"usermaps", "mods"} {
		dir := filepath.Join(root, kind, name, "zone_source", "all", "assetinfo")
		if _, err := os.Stat(dir); err == nil {
			return dir, nil
		}
		tried = append(tried, dir)
	}
	return "", fmt.Errorf("no linker report for %q: link it first (looked in %s)", name, strings.Join(tried, " and "))
}

// Load reads <dir>/<zone>.csv.
func Load(dir, zoneName string) (*Report, error) {
	path := filepath.Join(dir, zoneName+".csv")
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("no linker report %s: link the zone first (%w)", path, err)
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return nil, err
	}
	r := &Report{Zone: zoneName, Dir: dir, Linked: st.ModTime()}
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for first := true; sc.Scan(); first = false {
		if first {
			continue // header: index,type,name,resident,streamed,parentStack
		}
		if p, ok := parseLine(sc.Text()); ok {
			r.Assets = append(r.Assets, p)
		}
	}
	return r, sc.Err()
}

// parseLine reads `index,type,name,resident,streamed,parentStack`.
func parseLine(s string) (Packed, bool) {
	f := strings.SplitN(s, ",", 6)
	if len(f) < 6 {
		return Packed{}, false
	}
	res, err1 := strconv.ParseInt(f[3], 10, 64)
	str, err2 := strconv.ParseInt(f[4], 10, 64)
	if err1 != nil || err2 != nil {
		return Packed{}, false
	}
	return Packed{Ref: Ref{Type: f[1], Name: f[2]}, Resident: res, Streamed: str, Chain: parseChain(f[5])}, true
}

// parseChain reads a parentStack, `|name|type|name|type…`, nearest parent first.
func parseChain(s string) []Ref {
	parts := strings.Split(strings.TrimPrefix(s, "|"), "|")
	var out []Ref
	for i := 0; i+1 < len(parts); i += 2 {
		out = append(out, Ref{Name: parts[i], Type: parts[i+1]})
	}
	return out
}
