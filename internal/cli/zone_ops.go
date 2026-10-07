package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/McReaper/t7_companion/internal/asset"
	"github.com/McReaper/t7_companion/internal/gdt"
	"github.com/McReaper/t7_companion/internal/zone"
)

// Answers are capped: a map packs ~10k assets, and this lands in an agent's context.
const (
	zoneMaxLines   = 50
	zoneMaxLargest = 25
	zoneMaxChanged = 5
	zoneMaxSimilar = 10
)

// zoneStatus says when the report was written and whether it still describes
// the sources: a file the link read, or a GDT defining a packed asset, changed since.
type zoneStatus struct {
	Linked      string   `json:"linked"`
	Stale       bool     `json:"stale,omitempty"`
	Changed     int      `json:"changed_since_link,omitempty"`
	ChangedEg   []string `json:"changed,omitempty"`
	StaleAdvice string   `json:"advice,omitempty"`
}

type packedOut struct {
	Asset      string   `json:"asset"`
	Resident   int64    `json:"resident"`
	Streamed   int64    `json:"streamed,omitempty"`
	PulledInBy []string `json:"pulled_in_by"`
	Stock      string   `json:"stock,omitempty"`
}

type zoneExplainResult struct {
	Map string `json:"map"`
	zoneStatus
	Packed    []packedOut `json:"packed,omitempty"`
	NotPacked string      `json:"not_packed,omitempty"`
	Similar   []string    `json:"similar,omitempty"`
}

type sizedOut struct {
	Asset    string `json:"asset"`
	Resident int64  `json:"resident"`
	Streamed int64  `json:"streamed,omitempty"`
}

type zoneLineOut struct {
	Line     string `json:"line"`
	Assets   int    `json:"assets"`
	Resident int64  `json:"resident"`
	Streamed int64  `json:"streamed,omitempty"`
}

type zoneContentsResult struct {
	Map string `json:"map"`
	zoneStatus
	Line     string         `json:"line,omitempty"`
	Assets   int            `json:"assets"`
	Resident int64          `json:"resident"`
	Streamed int64          `json:"streamed"`
	ByType   map[string]int `json:"by_type,omitempty"`
	Largest  []sizedOut     `json:"largest,omitempty"`
	Lines    []zoneLineOut  `json:"lines,omitempty"`
	More     int            `json:"more,omitempty"`
	Upload   *uploadOut     `json:"upload,omitempty"`
}

// zoneCtx is a map's last linker report, opened under a mod-tools root.
type zoneCtx struct {
	root, toolsPath, name string
	report                *zone.Report
	status                zoneStatus
}

// openZone loads a map's (or mod's) last linker report and checks it is current.
func openZone(toolsPath, name string) (*zoneCtx, error) {
	root := strings.TrimRight(firstNonEmpty(toolsPath, os.Getenv("TA_TOOLS_PATH")), `\/`)
	if root == "" {
		return nil, fmt.Errorf("no mod-tools path: pass tools_path / --tools-path or set TA_TOOLS_PATH")
	}
	if name == "" {
		return nil, fmt.Errorf("name the map or mod, e.g. zm_mymap")
	}
	dir, err := zone.ReportDir(root, name)
	if err != nil {
		return nil, err
	}
	r, err := zone.Load(dir, name)
	if err != nil {
		return nil, err
	}
	return &zoneCtx{root: root, toolsPath: toolsPath, name: name, report: r, status: reportStatus(root, toolsPath, r)}, nil
}

// ignoreLists loads the stock assetlists the map's >class chain marks `ignore`:
// for their active entries the linker packs only a reference to the shipped
// asset. (Its ignore_missing_shipped lists are packed in full, so they don't
// shadow anything.)
func (z *zoneCtx) ignoreLists() (zone.Assetlists, zone.Inherited, error) {
	zf, err := zone.ZoneFile(z.root, z.name)
	if err != nil {
		return nil, zone.Inherited{}, err
	}
	inh, err := zone.Inherit(z.root, zf)
	if err != nil {
		return nil, inh, err
	}
	lists, err := zone.LoadAssetlists(z.root, inh.Ignore)
	return lists, inh, err
}

// stockNote says what an asset's stock assetlist entry means for the build.
func stockNote(entries []zone.ListEntry) string {
	for _, e := range entries {
		if e.Active {
			return fmt.Sprintf("only a reference to the shipped stock asset is packed (%s:%d is active): a version "+
				"of your own in a GDT or file would not be used; comment that line out with // to ship it", e.File(), e.Line)
		}
	}
	if len(entries) > 0 {
		return fmt.Sprintf("%s:%d is commented out, so this build packs its own version", entries[0].File(), entries[0].Line)
	}
	return ""
}

// reportStatus compares the report with the sources: the files the link read
// (from .deps) and the non-stock GDTs that define a packed asset.
func reportStatus(root, toolsPath string, r *zone.Report) zoneStatus {
	st := zoneStatus{Linked: r.Linked.Format("2006-01-02 15:04")}
	changed, _ := zone.Changed(r.Dir, r.Zone)
	changed = append(changed, changedGDTs(toolsPath, r)...)
	if len(changed) == 0 {
		return st
	}
	st.Stale, st.Changed = true, len(changed)
	for _, c := range changed[:min(len(changed), zoneMaxChanged)] {
		if rel, err := filepath.Rel(root, c); err == nil && !strings.HasPrefix(rel, "..") {
			c = filepath.ToSlash(rel)
		}
		st.ChangedEg = append(st.ChangedEg, c)
	}
	st.StaleAdvice = "files changed since this link: relink for a report that matches them"
	return st
}

// changedGDTs lists the non-stock GDTs defining a packed asset that were saved
// after the link. The linker's .deps records GDT assets by hash, not by file.
func changedGDTs(toolsPath string, r *zone.Report) []string {
	w, err := workspace(toolsPath)
	if err != nil {
		return nil
	}
	seen := map[string]bool{}
	var out []string
	for _, p := range r.Assets {
		locs, err := w.Find(p.Name)
		if err != nil {
			continue
		}
		for _, l := range locs {
			if seen[l.File] {
				continue
			}
			seen[l.File] = true
			if newerThanLink(w, l, r) {
				out = append(out, w.Abs(l.File))
			}
		}
	}
	sort.Strings(out)
	return out
}

func newerThanLink(w *gdt.Workspace, l gdt.Location, r *zone.Report) bool {
	if l.Stock {
		return false
	}
	st, err := os.Stat(w.Abs(l.File))
	return err == nil && st.ModTime().After(r.Linked)
}

// zoneExplain says why an asset is in the build: the chain of parents that pulled
// it in, up to the zone line — or that the last link didn't pack it.
func zoneExplain(toolsPath, name, asset, typ string) (*zoneExplainResult, error) {
	z, err := openZone(toolsPath, name)
	if err != nil {
		return nil, err
	}
	lists, _, _ := z.ignoreLists() // a map without a readable zone file still gets its chains
	res := &zoneExplainResult{Map: name, zoneStatus: z.status}
	for _, p := range z.report.Find(asset, typ) {
		out := packedOut{Asset: p.String(), Resident: p.Resident, Streamed: p.Streamed, Stock: stockNote(lists.Lookup(p.ID))}
		for _, c := range p.Chain {
			out.PulledInBy = append(out.PulledInBy, c.String())
		}
		res.Packed = append(res.Packed, out)
	}
	if len(res.Packed) == 0 {
		res.NotPacked = fmt.Sprintf("%s is not in %s's last link: nothing zoned references it, or the link failed before packing it", asset, name)
		res.Similar = similarNames(z.report, asset)
	}
	return res, nil
}

// similarNames suggests packed assets whose name contains the query.
func similarNames(r *zone.Report, q string) []string {
	q = strings.ToLower(q)
	seen := map[string]bool{}
	var out []string
	for _, p := range r.Assets {
		s := p.String()
		if strings.Contains(strings.ToLower(p.Name), q) && !seen[s] {
			seen[s] = true
			out = append(out, s)
			if len(out) == zoneMaxSimilar {
				break
			}
		}
	}
	return out
}

// zoneContents says what a zone line pulls into the build, or, with no line,
// how much each line of the zone weighs.
func zoneContents(toolsPath, name, line string) (*zoneContentsResult, error) {
	z, err := openZone(toolsPath, name)
	if err != nil {
		return nil, err
	}
	r := z.report
	res := &zoneContentsResult{Map: name, zoneStatus: z.status}
	if line == "" {
		res.Assets, res.Resident, res.Streamed = totals(r.Assets)
		res.Lines, res.More = lineSummary(r.Assets)
		res.Upload = upload(r.OutDir())
		return res, nil
	}
	ref := parseRef(line)
	under := r.Under(ref)
	if len(under) == 0 {
		return nil, fmt.Errorf("nothing in %s's last link comes from %q (try the asset name alone, or a .zpkg name)", name, line)
	}
	res.Line = ref.String()
	res.Assets, res.Resident, res.Streamed = totals(under)
	res.ByType = map[string]int{}
	for _, p := range under {
		res.ByType[p.Type]++
	}
	sort.Slice(under, func(i, j int) bool { return size(under[i]) > size(under[j]) })
	for _, p := range under[:min(len(under), zoneMaxLargest)] {
		res.Largest = append(res.Largest, sizedOut{p.String(), p.Resident, p.Streamed})
	}
	return res, nil
}

// parseRef reads "type,name", "type name" or a bare name.
func parseRef(s string) asset.ID {
	s = strings.TrimSpace(s)
	if t, n, ok := strings.Cut(s, ","); ok {
		return asset.ID{Type: strings.TrimSpace(t), Name: strings.TrimSpace(n)}
	}
	if t, n, ok := strings.Cut(s, " "); ok && !strings.ContainsAny(t, "/\\.") {
		return asset.ID{Type: t, Name: strings.TrimSpace(n)}
	}
	return asset.ID{Name: s}
}

func size(p zone.Packed) int64 { return p.Resident + p.Streamed }

func totals(ps []zone.Packed) (n int, res, str int64) {
	for _, p := range ps {
		res += p.Resident
		str += p.Streamed
	}
	return len(ps), res, str
}

// lineSummary groups assets by the zone line that pulled them in, heaviest first.
func lineSummary(ps []zone.Packed) ([]zoneLineOut, int) {
	by := map[asset.ID]*zoneLineOut{}
	for _, p := range ps {
		l := p.Line()
		o := by[l]
		if o == nil {
			o = &zoneLineOut{Line: l.String()}
			by[l] = o
		}
		o.Assets++
		o.Resident += p.Resident
		o.Streamed += p.Streamed
	}
	out := make([]zoneLineOut, 0, len(by))
	for _, o := range by {
		out = append(out, *o)
	}
	sort.Slice(out, func(i, j int) bool {
		a, b := out[i].Resident+out[i].Streamed, out[j].Resident+out[j].Streamed
		if a != b {
			return a > b
		}
		return out[i].Line < out[j].Line
	})
	if len(out) > zoneMaxLines {
		return out[:zoneMaxLines], len(out) - zoneMaxLines
	}
	return out, 0
}
