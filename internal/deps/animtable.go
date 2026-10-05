package deps

import (
	"bufio"
	"os"
	"strings"

	"github.com/McReaper/t7_companion/internal/asset"
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
