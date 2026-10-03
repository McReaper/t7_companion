package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/tiktoken-go/tokenizer"

	"github.com/McReaper/t7_companion/internal/embed"
	"github.com/McReaper/t7_companion/internal/gdt"
	"github.com/McReaper/t7_companion/internal/store"
	"github.com/McReaper/t7_companion/internal/zone"
)

// TestTokenReport measures what each MCP tool's answer costs an agent, in
// tokens, on the real corpus and install: the outputs are built by the same
// functions the MCP handlers use (formatHits, renderDoc, the gdt ops + the
// compact JSON of jsonResult). Counts use o200k_base, an offline BPE tokenizer —
// not Claude's own, which isn't public — so read them as relative: they are for
// comparing formats and spotting regressions, not for billing. Opt-in:
//
//	T7KB_BENCH_DB=<t7kb.db> HF_HOME=<models> [TA_TOOLS_PATH=<bo3 root>] \
//	  go test ./internal/cli -run TestTokenReport -v
//
// Set T7KB_TOKEN_REPORT=<file> to also write the table as markdown.

// benchQueries are phrased the way a modder asks, across the skills' domains.
var benchQueries = []string{
	"zombie spawner not spawning", "how to add a perk machine", "clientfield register set lua",
	"linker error could not find scriptparsetree", "material surfacetype error", "custom wallbuy cost 0",
	"how to make a moving platform elevator", "notetrack xanim fires early", "fog volume not working",
	"easter egg step script", "BSP leak sealing the map", "hintstring color code",
	"reflection probe lighting too dark", "port a weapon from black ops 2", "player respawn co-op",
	"sound alias plays silently",
}

type tokenRow struct {
	tool, what   string
	n            int
	p50, p90, mx int
	bytesP50     int
}

func TestTokenReport(t *testing.T) {
	db := os.Getenv("T7KB_BENCH_DB")
	if db == "" {
		t.Skip("set T7KB_BENCH_DB to a t7kb.db (and HF_HOME to the model cache)")
	}
	enc, err := tokenizer.Get(tokenizer.O200kBase)
	if err != nil {
		t.Fatal(err)
	}
	count := func(s string) int {
		n, err := enc.Count(s)
		if err != nil {
			t.Fatal(err)
		}
		return n
	}
	emb, err := embed.New()
	if err != nil {
		t.Fatal(err)
	}
	st, err := store.Open(db)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = st.Close() }()

	ctx := context.Background()
	samples := map[string][][2]int{} // "tool|what" -> (tokens, bytes)
	add := func(key, s string) { samples[key] = append(samples[key], [2]int{count(s), len(s)}) }
	var lat []time.Duration
	for _, q := range benchQueries {
		start := time.Now()
		v, err := emb.Embed(q)
		if err != nil {
			t.Fatal(err)
		}
		hits, err := st.SearchHybrid(ctx, q, v, 10)
		if err != nil {
			t.Fatal(err)
		}
		lat = append(lat, time.Since(start))
		add("search|10 results (default)", formatHits(hits))
		if len(hits) > 5 {
			add("search|5 results", formatHits(hits[:5]))
		}
		for i, h := range hits {
			d, err := st.Get(ctx, h.DocID)
			if err != nil || d == nil {
				continue
			}
			if i == 0 {
				add("get|top result", getPage(t, d))
			}
			if i < 3 {
				add("get|each of the top 3", getPage(t, d))
			}
		}
	}

	if root := os.Getenv("TA_TOOLS_PATH"); root != "" {
		gdtSamples(t, root, add)
		zoneSamples(t, root, add)
	}

	rows := make([]tokenRow, 0, len(samples))
	for key, ss := range samples {
		tool, what, _ := strings.Cut(key, "|")
		tok := make([]int, len(ss))
		byt := make([]int, len(ss))
		for i, s := range ss {
			tok[i], byt[i] = s[0], s[1]
		}
		sort.Ints(tok)
		sort.Ints(byt)
		rows = append(rows, tokenRow{tool, what, len(ss), pct(tok, .5), pct(tok, .9), tok[len(tok)-1], pct(byt, .5)})
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].tool != rows[j].tool {
			return rows[i].tool < rows[j].tool
		}
		return rows[i].what < rows[j].what
	})
	sort.Slice(lat, func(i, j int) bool { return lat[i] < lat[j] })

	var md bytes.Buffer
	fmt.Fprintf(&md, "| Tool | Call | Samples | Tokens p50 | p90 | max | Bytes p50 |\n|---|---|---:|---:|---:|---:|---:|\n")
	for _, r := range rows {
		fmt.Fprintf(&md, "| `%s` | %s | %d | %d | %d | %d | %d |\n", r.tool, r.what, r.n, r.p50, r.p90, r.mx, r.bytesP50)
	}
	fmt.Fprintf(&md, "\nsearch latency (embed + search): p50 %v, max %v over %d queries\n",
		lat[len(lat)/2].Round(time.Millisecond), lat[len(lat)-1].Round(time.Millisecond), len(lat))
	t.Log("\n" + md.String())
	if out := os.Getenv("T7KB_TOKEN_REPORT"); out != "" {
		if err := os.WriteFile(out, md.Bytes(), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

// gdtSamples measures the gdt_* tools on a few real assets: a material, its
// first image, the GDT holding it, and two schemas.
func gdtSamples(t *testing.T, root string, add func(key, s string)) {
	t.Helper()
	w, err := gdt.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	asJSON := func(v any) string { // what jsonResult sends
		var b bytes.Buffer
		e := json.NewEncoder(&b)
		e.SetEscapeHTML(false)
		if err := e.Encode(v); err != nil {
			t.Fatal(err)
		}
		return b.String()
	}
	run := func(key string, v any, err error) {
		if err != nil {
			t.Logf("%s: %v", key, err)
			return
		}
		add(key, asJSON(v))
	}
	const mat = "mtl_p7_cru_gun_range_booth_01"
	v, err := gdtFind(w, mat)
	run("gdt_find|a material", v, err)
	v, err = gdtGet(w, mat, "", "", false)
	run("gdt_get|a material (defaults hidden)", v, err)
	v, err = gdtGet(w, mat, "", "", true)
	run("gdt_get|a material, all=true", v, err)
	v, err = gdtSchema(w, "material", "", "")
	run("gdt_schema|material (names only)", v, err)
	v, err = gdtSchema(w, "material", "lit", "")
	run("gdt_schema|material + material_type lit", v, err)
	v, err = gdtSchema(w, "xmodel", "", "")
	run("gdt_schema|xmodel", v, err)
	locs, _ := w.Find(mat)
	if len(locs) > 0 {
		v, err = gdtCheck(w, locs[0].File, "")
		run("gdt_check|the whole GDT", v, err)
		v, err = gdtCheck(w, locs[0].File, mat)
		run("gdt_check|one asset", v, err)
	}
	v, err = gdtRefs(w, mat)
	run("gdt_refs|the material (GDT fields and LOD files)", v, err)
	if view, err := gdtGet(w, mat, "", "", false); err == nil {
		if img := view.(gdtAssetView).Own["colorMap"]; img != "" {
			v, err = gdtRefs(w, img)
			run("gdt_refs|its color image", v, err)
		}
	}
}

// getPage is what the get tool returns for a doc: its first page.
func getPage(t *testing.T, d *store.Doc) string {
	t.Helper()
	page, err := renderDoc(d, 0, pageSize(0))
	if err != nil {
		t.Fatal(err)
	}
	return page
}

func pct(xs []int, q float64) int { return xs[int(q*float64(len(xs)-1))] }

// zoneSamples measures the zone_* tools on the first linked map of the install:
// the asset with the longest chain, the whole zone by line, and its heaviest line.
func zoneSamples(t *testing.T, root string, add func(key, s string)) {
	t.Helper()
	maps, _ := filepath.Glob(filepath.Join(root, "usermaps", "*", "zone_source", "all", "assetinfo"))
	sort.Strings(maps)
	for _, dir := range maps {
		name := filepath.Base(filepath.Dir(filepath.Dir(filepath.Dir(dir))))
		r, err := zone.Load(dir, name)
		if err != nil || len(r.Assets) == 0 {
			continue
		}
		deepest := r.Assets[0]
		for _, p := range r.Assets {
			if len(p.Chain) > len(deepest.Chain) {
				deepest = p
			}
		}
		run := func(key string, v any, err error) {
			if err != nil {
				t.Logf("%s: %v", key, err)
				return
			}
			add(key, mcpJSON(t, v))
		}
		v, err := zoneExplain(root, name, deepest.Name, deepest.Type)
		run("zone_explain|the asset with the longest chain", v, err)
		all, err := zoneContents(root, name, "")
		run("zone_contents|every line of the zone", all, err)
		if err == nil && len(all.Lines) > 0 {
			heavy, err := zoneContents(root, name, all.Lines[0].Line)
			run("zone_contents|its heaviest line", heavy, err)
		}
		chk, err := zoneCheck(root, name)
		run("zone_check|the map", chk, err)
		return
	}
}

// mcpJSON is what jsonResult sends for v.
func mcpJSON(t *testing.T, v any) string {
	t.Helper()
	var b bytes.Buffer
	e := json.NewEncoder(&b)
	e.SetEscapeHTML(false)
	if err := e.Encode(v); err != nil {
		t.Fatal(err)
	}
	return b.String()
}
