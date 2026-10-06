package deps

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"

	"github.com/McReaper/t7_companion/internal/asset"
	"github.com/McReaper/t7_companion/internal/format/xanimbin"
	"github.com/McReaper/t7_companion/internal/gdt"
)

// mappingChildren adds the xanims of an animation mapping table,
// animtables/<name>: `alias,xanim[,xanim…]` lines (variants the AI picks
// from), # comments.
func (g *Graph) mappingChildren(name string, out *idSet) {
	p, ok := g.raw("animtables/" + name)
	if !ok {
		return
	}
	f, err := os.Open(p)
	if err != nil {
		return
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64<<10), 1<<20)
	for sc.Scan() {
		l := strings.TrimSpace(sc.Text())
		if l == "" || strings.HasPrefix(l, "#") {
			continue
		}
		cols := strings.Split(l, ",")
		for _, c := range cols[1:] {
			out.add(asset.ID{Type: "xanim", Name: strings.TrimSpace(c)})
		}
	}
}

// animChildren adds the rumbles an animation's notetracks play (rmbnt#<rumble>
// notes in its xanim_export file; the notes its GDT declares are fields).
func (g *Graph) animChildren(fields []gdt.Field, out *idSet) {
	file := fieldMap(fields)["filename"]
	if file == "" {
		return
	}
	b, err := os.ReadFile(filepath.Join(g.w.Root, "xanim_export", filepath.FromSlash(strings.ReplaceAll(file, `\`, "/"))))
	if err != nil {
		return // a missing animation file is gdt_check's to report
	}
	notes, _ := xanimbin.Notes(b)
	for _, n := range notes {
		if n.Kind == "rmb" {
			out.add(asset.ID{Type: "rumble", Name: n.Value})
		}
	}
}
