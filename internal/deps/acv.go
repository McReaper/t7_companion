package deps

import (
	"regexp"
	"strings"

	"github.com/McReaper/t7_companion/internal/gdt"
)

// An attachment cosmetic variant asset lists up to three variants, acv0_…
// acv2_…; a variant with neither a view nor a world model is unused, and the
// linker packs nothing it names (a parent's placeholder names and icons).

// acvField is a variant's field: acv<n>_<rest>.
var acvField = regexp.MustCompile(`(?i)^(acv\d+)_(.+)$`)

// usedVariants keeps the fields of the variants that have a model.
func usedVariants(fields []gdt.Field) []gdt.Field {
	used := map[string]bool{}
	for _, f := range fields {
		if m := acvField.FindStringSubmatch(f.Key); m != nil && f.Value != "" &&
			(strings.HasPrefix(strings.ToLower(m[2]), "viewmodel_") || strings.HasPrefix(strings.ToLower(m[2]), "worldmodel_") ||
				strings.HasPrefix(strings.ToLower(m[2]), "viewmodelads_")) {
			used[strings.ToLower(m[1])] = true
		}
	}
	out := fields[:0:0]
	for _, f := range fields {
		if m := acvField.FindStringSubmatch(f.Key); m != nil && !used[strings.ToLower(m[1])] {
			continue
		}
		out = append(out, f)
	}
	return out
}
