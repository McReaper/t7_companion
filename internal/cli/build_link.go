package cli

import (
	"path/filepath"
	"strings"
	"time"
)

// linkStage links the fast files.
func (p *buildPlan) linkStage() (stageResult, bool) {
	if !p.stages["link"] {
		return stageResult{}, false
	}
	if !p.o.freshXpak {
		return p.linkTarget(), true
	}
	aside, err := setXpaksAside(p.zoneDir())
	if err != nil {
		return stageResult{Name: "link", Errors: []string{err.Error()}}, true
	}
	sr := p.linkTarget()
	sr.Note = joinNotes(sr.Note, aside.settle(sr.OK))
	return sr, true
}

func (p *buildPlan) linkTarget() stageResult {
	if p.o.isMod {
		return p.linkMod()
	}
	start := time.Now()
	sr := p.link("-modsource", p.name)
	zoneSource := filepath.Join(p.game, "usermaps", p.name, "zone_source")
	applyLinkLog(&sr, zoneSource, "", start)
	if sr.OK {
		sr.Missing = missingAfter(p.game, filepath.Join(zoneSource, "all", "assetinfo"), p.name, start)
	}
	return sr
}

func (p *buildPlan) link(args ...string) stageResult {
	args = append([]string{"-language", p.o.language}, args...)
	// 60 min: a map's first link converts every image it packs (~23 min for the ZM Advanced template)
	return stageRunner("link", p.bin, filepath.Join(p.bin, "linker_modtools.exe"), 60*time.Minute, p.o.verbose, p.stdout, args...)
}

// modZones are the zones a mod can have, linked one by one in this order, as
// the Launcher does (`-fs_game <mod> -modsource <zone>` for each present).
var modZones = []string{"core_mod", "mp_mod", "cp_mod", "zm_mod"}

func (p *buildPlan) linkMod() stageResult {
	total := stageResult{Name: "link", OK: true}
	var linked []string
	zoneSource := filepath.Join(p.game, "mods", p.name, "zone_source")
	for _, z := range modZones {
		if fileMTime(filepath.Join(p.game, "mods", p.name, "zone_source", z+".zone")).IsZero() {
			continue
		}
		start := time.Now()
		sr := p.link("-fs_game", p.name, "-modsource", z)
		applyLinkLog(&sr, zoneSource, z+": ", start)
		total.Seconds = round1(total.Seconds + sr.Seconds)
		total.Warnings = append(total.Warnings, sr.Warnings...)
		if !sr.OK {
			total.OK = false
			for _, e := range sr.Errors {
				total.Errors = append(total.Errors, z+": "+strings.TrimPrefix(e, z+": "))
			}
			return total
		}
		linked = append(linked, z)
		for _, m := range missingAfter(p.game, filepath.Join(zoneSource, "all", "assetinfo"), z, start) {
			total.Missing = append(total.Missing, z+": "+m)
		}
	}
	if len(linked) == 0 {
		total.OK = false
		total.Errors = []string{"no core_mod, mp_mod, cp_mod or zm_mod .zone in mods/" + p.name + "/zone_source"}
		return total
	}
	total.Note = "linked " + strings.Join(linked, ", ")
	return total
}
