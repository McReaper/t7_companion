package gdt

import (
	"regexp"
	"strings"
)

// Some .awi validation callbacks don't validate: they write other fields when one
// changes in APE. gdt_edit does the same, so an edit leaves what APE would have
// saved — unless the request sets the written field itself.
//
//   - material.awi ValidateGlossSurfaceType: a glossSurfaceType preset writes its
//     glossRangeMin/glossRangeMax ("<custom>" leaves them alone). Only the range
//     is what the material gets: 406 stock materials keep a preset that disagrees
//     with their range, so a mismatch is not an error — but setting the preset
//     alone would change nothing.
//   - image.awi ValidatePremulAlpha: semantic effectMap sets premulAlpha and
//     clears streamable; any other semantic does the reverse.
//
// The .awi's other callbacks only refresh labels (sanim, xanim offsets), ask a
// question (xanim compression), or are covered as checks (xmodel ValidateLODs).

// glossPresets reads ValidateGlossSurfaceType's table: preset -> {min, max}.
func glossPresets(code string) map[string][2]string {
	// the definition, not the "void ValidateGlossSurfaceType(…)" string a
	// SetValidateCallback names it by
	loc := glossFuncRE.FindStringIndex(code)
	if loc == nil {
		return nil
	}
	body := code[loc[0]:]
	if end := strings.Index(body, "\n}"); end > 0 {
		body = body[:end]
	}
	out := map[string][2]string{}
	for _, m := range glossCaseRE.FindAllStringSubmatch(body, -1) {
		var mn, mx string
		for _, s := range glossSetRE.FindAllStringSubmatch(m[2], -1) {
			if s[1] == "Min" {
				mn = s[2]
			} else {
				mx = s[2]
			}
		}
		if mn != "" && mx != "" {
			out[strings.ToLower(m[1])] = [2]string{mn, mx}
		}
	}
	return out
}

var (
	glossFuncRE = regexp.MustCompile(`(?m)^\s*void\s+ValidateGlossSurfaceType\s*\(`)
	glossCaseRE = regexp.MustCompile(`surfType\s*==\s*"([^"]+)"\s*\)\s*\{([^}]*)\}`)
	glossSetRE  = regexp.MustCompile(`entryGloss(Min|Max)\.SetFloat\(\s*([-0-9.]+)\s*\)`)
)

// apeEffects returns the fields APE would write after the request's own sets.
func (w *Workspace) apeEffects(typ string, set map[string]string) map[string]string {
	out := map[string]string{}
	put := func(k, v string) {
		if _, own := set[k]; !own {
			out[k] = v
		}
	}
	switch typ {
	case "material":
		p, ok := set["glossSurfaceType"]
		if !ok {
			break
		}
		if sc, err := w.Schema(typ); err == nil {
			if r, ok := sc.glossPresets[strings.ToLower(p)]; ok {
				put("glossRangeMin", r[0])
				put("glossRangeMax", r[1])
			}
		}
	case "image":
		s, ok := set["semantic"]
		if !ok {
			break
		}
		effect := s == "effectMap"
		put("premulAlpha", boolField(effect))
		put("streamable", boolField(!effect))
	}
	return out
}

func boolField(b bool) string {
	if b {
		return "1"
	}
	return "0"
}
