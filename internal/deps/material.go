package deps

import (
	"strings"

	"github.com/McReaper/t7_companion/internal/asset"
	"github.com/McReaper/t7_companion/internal/gdt"
)

// materialChildren adds a material's images as the linker packs them: one per
// texture slot of the techset its materialType resolves to — the image the
// slot's field names or, when the field is empty, the slot's default. An image
// in a field no slot reads isn't packed. It reports false when the type has no
// techsetdef to read.
func (g *Graph) materialChildren(fields []gdt.Field, out *idSet) bool {
	v := fieldMap(fields)
	if g.w.Techsets == nil {
		return false
	}
	ts, err := g.w.Techsets.Resolve(v["materialtype"])
	if err != nil {
		return false
	}
	for _, s := range ts.Textures {
		img := v[strings.ToLower(s.Field)]
		if img == "" {
			img = s.DefaultImage
		}
		out.add(asset.ID{Type: "image", Name: img})
	}
	for _, r := range g.w.Refs("material", fields) {
		if r.Type != "image" {
			out.add(asset.ID{Type: g.w.LinkerType(r.Type), Name: r.Target})
		}
	}
	return true
}
