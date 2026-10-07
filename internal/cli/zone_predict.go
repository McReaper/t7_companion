package cli

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/McReaper/t7_companion/internal/asset"
	"github.com/McReaper/t7_companion/internal/deps"
	"github.com/McReaper/t7_companion/internal/gdt"
)

// A prediction lands in an agent's context like the report's answers: capped.
const (
	predictMaxLines    = 25
	predictMaxDangling = 40
	predictMaxDirect   = 40
)

type danglingOut struct {
	Asset   string `json:"asset"`
	NamedBy string `json:"named_by"`
}

type predictLineOut struct {
	Line   string `json:"line"`
	Assets int    `json:"assets"`
}

type zonePredictResult struct {
	Map        string           `json:"map"`
	Line       string           `json:"line,omitempty"`
	Assets     int              `json:"assets"`
	ByType     map[string]int   `json:"by_type,omitempty"`
	Lines      []predictLineOut `json:"lines,omitempty"`
	Direct     []string         `json:"names_directly,omitempty"`
	PulledInBy []string         `json:"pulled_in_by,omitempty"`
	NotPulled  string           `json:"not_pulled_in,omitempty"`
	Dangling   []danglingOut    `json:"no_source,omitempty"`
	Masked     []shadowGDT      `json:"gdt_versions_not_used,omitempty"`
	More       int              `json:"more,omitempty"`
	Advice     []string         `json:"advice,omitempty"`
}

// zonePredict predicts, from the sources, what a map's or mod's next link
// will pull in: the whole build, one zone line or asset (line), or the chain
// that will pull an asset in (assetName).
func zonePredict(toolsPath, name, line, assetName, typ string) (*zonePredictResult, error) {
	if name == "" {
		return nil, fmt.Errorf("name the map or mod, e.g. zm_mymap")
	}
	w, err := workspace(toolsPath)
	if err != nil {
		return nil, err
	}
	z, err := deps.OpenZone(w, name)
	if err != nil {
		return nil, err
	}
	switch {
	case assetName != "":
		return predictWhy(z, name, assetName, typ)
	case line != "":
		return predictLine(z, w, name, line)
	}
	return predictAll(z, name), nil
}

// predictAll is the whole build: by type, by zone line, what has no source,
// and the GDT versions stock replaces.
func predictAll(z *deps.Zone, name string) *zonePredictResult {
	c := z.Predict()
	res := &zonePredictResult{Map: name}
	summarize(res, c)
	res.Lines = predictLines(z, c)
	if len(res.Lines) > predictMaxLines {
		res.More += len(res.Lines) - predictMaxLines
		res.Lines = res.Lines[:predictMaxLines]
	}
	addDangling(res, z, c)
	var shadows []shadowOut
	for _, m := range z.Masked(c) {
		shadows = append(shadows, shadowOut{Asset: m.String(), Yours: fmt.Sprintf("defined in %s:%d", m.Yours.File, m.Yours.Line),
			Stock: fmt.Sprintf("%s:%d", m.Stock.File(), m.Stock.Line)})
	}
	res.Masked = groupByGDT(shadows)
	if len(res.Masked) > zoneMaxShadowed {
		res.More += len(res.Masked) - zoneMaxShadowed
		res.Masked = res.Masked[:zoneMaxShadowed]
	}
	if len(res.Masked) > 0 {
		res.Advice = append(res.Advice, "gdt_versions_not_used: the stock version of these ships, not yours; to ship your own, "+
			"comment its active stock line out with // (in the BO3 root's zone_source/all/assetlist) and relink")
	}
	return res
}

// predictLine is what one zone line, or any asset, pulls in.
func predictLine(z *deps.Zone, w *gdt.Workspace, name, line string) (*zonePredictResult, error) {
	id, err := typedAsset(z, w, line)
	if err != nil {
		return nil, err
	}
	c := z.Closure([]asset.ID{id})
	res := &zonePredictResult{Map: name, Line: id.String()}
	summarize(res, c)
	for _, d := range z.Children(id) {
		res.Direct = append(res.Direct, d.String())
	}
	sort.Strings(res.Direct)
	if len(res.Direct) > predictMaxDirect {
		res.More += len(res.Direct) - predictMaxDirect
		res.Direct = res.Direct[:predictMaxDirect]
	}
	addDangling(res, z, c)
	return res, nil
}

// predictWhy is the chain that will pull an asset into the build.
func predictWhy(z *deps.Zone, name, assetName, typ string) (*zonePredictResult, error) {
	c := z.Predict()
	res := &zonePredictResult{Map: name, Assets: len(c)}
	var found []asset.ID
	for _, n := range c {
		if strings.EqualFold(n.Name, assetName) && (typ == "" || strings.EqualFold(n.Type, typ)) {
			found = append(found, n.ID)
		}
	}
	if len(found) == 0 {
		res.NotPulled = fmt.Sprintf("nothing the zone's lines or map source pull in names %q: the next link won't pack it", assetName)
		return res, nil
	}
	sort.Slice(found, func(i, j int) bool { return found[i].String() < found[j].String() })
	chain := c.Chain(found[0])
	for _, a := range chain {
		res.PulledInBy = append(res.PulledInBy, a.String())
	}
	res.PulledInBy = append(res.PulledInBy, rootLabel(z, chain[len(chain)-1]))
	if len(found) > 1 {
		res.Advice = append(res.Advice, fmt.Sprintf("%d assets are named %q (pass type for another): %s", len(found), assetName, ids(found)))
	}
	return res, nil
}

// typedAsset reads a line or asset ("type,name", "type name", or a name the
// zone's lines or the GDTs give one type).
func typedAsset(z *deps.Zone, w *gdt.Workspace, s string) (asset.ID, error) {
	id := parseRef(s)
	if id.Type != "" {
		return id, nil
	}
	types := map[string]bool{}
	for _, l := range z.Lines {
		if strings.EqualFold(l.Name, id.Name) {
			types[l.Type] = true
		}
	}
	if len(types) == 0 {
		types = gdtTypesOf(w, id.Name)
	}
	switch len(types) {
	case 0:
		return id, fmt.Errorf("no zone line or GDT asset is named %q: give its type, as \"type,name\"", id.Name)
	case 1:
		for t := range types {
			id.Type = t
		}
		return id, nil
	}
	var ts []string
	for t := range types {
		ts = append(ts, t)
	}
	sort.Strings(ts)
	return id, fmt.Errorf("%q names a %s: give its type, as \"type,name\"", id.Name, strings.Join(ts, " and a "))
}

func gdtTypesOf(w *gdt.Workspace, name string) map[string]bool {
	out := map[string]bool{}
	locs, err := w.Find(name)
	if err != nil {
		return out
	}
	for _, l := range locs {
		if t := w.TypeOf(l); t != "" {
			out[w.LinkerType(t)] = true
		}
	}
	return out
}

// summarize counts a closure's assets, those packed on their own.
func summarize(res *zonePredictResult, c deps.Closure) {
	res.ByType = map[string]int{}
	for _, n := range c {
		if deps.Packed(n.Type) {
			res.Assets++
			res.ByType[n.Type]++
		}
	}
}

// predictLines counts what each zone line, and the map source, pulls in first.
func predictLines(z *deps.Zone, c deps.Closure) []predictLineOut {
	count := map[string]int{}
	for _, n := range c {
		if deps.Packed(n.Type) {
			count[rootLabel(z, c.Root(n.ID))]++
		}
	}
	out := make([]predictLineOut, 0, len(count))
	for l, n := range count {
		out = append(out, predictLineOut{Line: l, Assets: n})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Assets != out[j].Assets {
			return out[i].Assets > out[j].Assets
		}
		return out[i].Line < out[j].Line
	})
	return out
}

// rootLabel names where a closure starts: a zone line, or the map source.
func rootLabel(z *deps.Zone, root asset.ID) string {
	for _, l := range z.Lines {
		if l.Key() == root.Key() {
			return "zone line " + l.String()
		}
	}
	return "map source " + filepath.Base(z.MapSource)
}

// addDangling lists what no source defines, with what names it.
func addDangling(res *zonePredictResult, z *deps.Zone, c deps.Closure) {
	for _, d := range z.Dangling(c) {
		by := rootLabel(z, d.ID)
		if d.From.Name != "" {
			by = d.From.String()
		}
		res.Dangling = append(res.Dangling, danglingOut{Asset: d.String(), NamedBy: by})
	}
	if len(res.Dangling) > predictMaxDangling {
		res.More += len(res.Dangling) - predictMaxDangling
		res.Dangling = res.Dangling[:predictMaxDangling]
	}
	if len(res.Dangling) > 0 {
		res.Advice = append(res.Advice, "no_source: no GDT, file or stock list defines these, so the link has nothing of "+
			"yours to pack for them (only engine or shipped data could stand in): check the name that references them")
	}
}

func ids(xs []asset.ID) string {
	s := make([]string, len(xs))
	for i, x := range xs {
		s[i] = x.String()
	}
	return strings.Join(s, ", ")
}
