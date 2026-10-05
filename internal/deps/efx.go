package deps

import (
	"bufio"
	"bytes"
	"os"
	"path"
	"strings"

	"github.com/McReaper/t7_companion/internal/asset"
)

// efxBlocks are the element blocks of an .efx whose quoted lines name assets,
// by the type they name: an element's visuals.
var efxBlocks = map[string]string{
	"billboardSprite": "material", "orientedSprite": "material", "rotatedSprite": "material",
	"tail": "material", "line": "material", "trail": "material", "cloud": "material",
	"decal": "material", "dynamicLight": "material",
	"model":     "xmodel",
	"runner":    "fx",
	"lensFlare": "klf",
}

// efxKeys are the element keys whose quoted value names another fx.
var efxKeys = map[string]bool{"fxOnImpact": true, "fxOnDeath": true, "emission": true, "attachment": true}

// fxChildren adds what an effect, fx/<name>.efx, names.
func (g *Graph) fxChildren(name string, out *idSet) {
	p, ok := g.raw("fx/" + name + ".efx")
	if !ok {
		return
	}
	b, err := os.ReadFile(p)
	if err != nil {
		return
	}
	for _, id := range efxRefs(b) {
		out.add(id)
	}
}

// efxRefs reads the assets an .efx names: in each element (a brace block of
// tab-indented `key value;` lines), the quoted lines of a visuals block
// (`model` then `{ "name" … };`) and the fx of efxKeys. Sound blocks name sound
// aliases, which the zone's sound bank packs, not the effect.
func efxRefs(b []byte) []asset.ID {
	var out []asset.ID
	block := ""
	sc := bufio.NewScanner(bytes.NewReader(b))
	sc.Buffer(make([]byte, 64<<10), 1<<20)
	for sc.Scan() {
		l := strings.TrimRight(sc.Text(), " \r")
		inner, nested := strings.CutPrefix(l, "\t\t")
		switch {
		case nested && block != "" && strings.HasPrefix(inner, `"`):
			if v := strings.Trim(inner, `"`); v != "" {
				out = append(out, efxRef(efxBlocks[block], v))
			}
		case nested, l == "	{":
		case strings.HasPrefix(l, "\t") && !strings.ContainsAny(l[1:], " \t{};"):
			block = l[1:]
			if efxBlocks[block] == "" {
				block = ""
			}
		default:
			block = ""
			key, v, ok := strings.Cut(strings.TrimPrefix(l, "\t"), " ")
			if v = strings.Trim(strings.TrimSuffix(v, ";"), `"`); ok && efxKeys[key] && v != "" {
				out = append(out, efxRef("fx", v))
			}
		}
	}
	return out
}

// efxRef names an asset an .efx names; an fx may be written as its file
// (`runner { "smoke/fx_smk.efx" }`).
func efxRef(typ, v string) asset.ID {
	if typ == "fx" {
		v = strings.ReplaceAll(v, `\`, "/")
		if strings.EqualFold(path.Ext(v), ".efx") {
			v = strings.TrimSuffix(v, path.Ext(v))
		}
	}
	return asset.ID{Type: typ, Name: v}
}
