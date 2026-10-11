package cli

import (
	"fmt"
	"strings"

	"github.com/McReaper/t7_companion/internal/zone"
)

// bareMaterial is a zone line naming a material on its own, which the linker
// builds under that bare name: a variant of its techset the shader cache
// doesn't have.
type bareMaterial struct {
	Line         string   `json:"line"`
	At           string   `json:"zoned_at"`
	MaterialType string   `json:"material_type"`
	Prefixes     []string `json:"techset_prefixes"`
}

const bareAdvice = "the linker builds a material a zone line names on its own under that bare name, a variant of its " +
	"techset the mod tools' shader cache doesn't have, so it tries to compile the stock shader sources, which aren't " +
	"shipped: the link fails with error X1507 failed to open source file 'techsetdef_….hlsl' (GetDrawMethod(…) Failed). " +
	"Remove the line, since a material is built through what uses it (as mc/<name> for a model, wc/<name> for map " +
	"geometry), or zone the prefixed name, material,mc/<name>"

// bareMaterials finds the zone's `material,<name>` lines (no prefix) whose
// techset has no bare variant: its availablePrefixes lacks "/" and no shipped
// fastfile holds it bare. A material no GDT defines is left to the linker.
func bareMaterials(root, zoneFile string) []bareMaterial {
	lines, err := zone.ZoneLines(root, zoneFile)
	if err != nil {
		return nil
	}
	var named []zone.Line
	for _, l := range lines {
		if strings.EqualFold(l.Type, "material") && !strings.Contains(l.Name, "/") {
			named = append(named, l)
		}
	}
	if len(named) == 0 {
		return nil // no GDT index to open
	}
	w, err := workspace(root)
	if err != nil {
		return nil
	}
	shipped, err := zone.BareTechsets(root)
	if err != nil {
		return nil
	}
	var out []bareMaterial
	for _, l := range named {
		ts, err := w.MaterialTechset(l.Name)
		if err != nil || ts.BuildsBare() || shipped[strings.ToLower(ts.MaterialType)] {
			continue
		}
		out = append(out, bareMaterial{Line: l.Type + "," + l.Name, At: fmt.Sprintf("%s:%d", relTo(root, l.File), l.N),
			MaterialType: ts.MaterialType, Prefixes: ts.Prefixes})
	}
	return out
}
