package deps

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/McReaper/t7_companion/internal/gdt"
)

// A weapon's camo table (GDT type weaponcamotable) is packed as one weaponcamo
// asset holding its camo sets (GDT type weaponcamo) inline: a set's materials
// and images are the table's children, and the set itself isn't packed. Only
// what the counts enable counts — the first numCamoTables sets, a set's first
// numCamos camos, a camo part's first numBaseMaterials base materials; APE
// keeps the rest as hidden, stale fields.

// camoTableChildren adds what a camo table's enabled sets name.
func (g *Graph) camoTableChildren(fields []gdt.Field, out *idSet) {
	v := fieldMap(fields)
	n := atoi(v["numcamotables"], 1)
	for t := 1; t <= n; t++ {
		set := v[fmt.Sprintf("table_%02d_name", t)]
		if set == "" {
			continue
		}
		if _, typ, fs, ok := g.resolve(g.w.FindTyped(set, "weaponcamo")); ok {
			g.fieldChildren(typ, enabledCamos(fs), out)
		}
	}
}

// camoField is a camo's field: material<part>_<camo>_<rest>.
var camoField = regexp.MustCompile(`(?i)^material(\d+)_(\d+)_(.+)$`)

// baseField is a base material's: base_material_<n> or camo_mask_<n>.
var baseField = regexp.MustCompile(`(?i)^(?:base_material|camo_mask)_(\d+)$`)

// enabledCamos keeps a camo set's fields that its counts enable.
func enabledCamos(fields []gdt.Field) []gdt.Field {
	v := fieldMap(fields)
	numCamos := atoi(v["numcamos"], 0)
	var out []gdt.Field
	for _, f := range fields {
		m := camoField.FindStringSubmatch(f.Key)
		if m == nil {
			continue
		}
		if c := atoi(m[2], 0); c < 1 || c > numCamos {
			continue
		}
		if b := baseField.FindStringSubmatch(m[3]); b != nil {
			prefix := "material" + m[1] + "_" + m[2] + "_"
			if atoi(b[1], 0) > atoi(v[prefix+"numbasematerials"], 0) {
				continue
			}
		}
		out = append(out, f)
	}
	return out
}

// fieldMap is fields by lower-cased key, values unquoted.
func fieldMap(fields []gdt.Field) map[string]string {
	m := make(map[string]string, len(fields))
	for _, f := range fields {
		m[strings.ToLower(f.Key)] = gdt.Unquote(f.Value)
	}
	return m
}

func atoi(s string, def int) int {
	if n, err := strconv.Atoi(strings.TrimSpace(s)); err == nil {
		return n
	}
	return def
}
