package gdt

import (
	"strings"

	"github.com/McReaper/t7_companion/internal/asset"
)

// weaponTypes are the GDT weapon classes; the linker packs each as a "weapon".
var weaponTypes = map[string]bool{
	"bulletweapon": true, "projectileweapon": true, "grenadeweapon": true, "turretweapon": true,
	"meleeweapon": true, "dualwieldweapon": true, "dualwieldprojectileweapon": true,
	"gasweapon": true, "cybercomweapon": true,
}

// linkerNames are the other GDT types the linker packs under another name.
var linkerNames = map[string]string{
	"weaponcamotable":             "weaponcamo",
	"charactercustomizationtable": "customizationtable",
}

// LinkerType is the asset type the linker packs a GDT type as — the one its
// report and the zone files use: every weapon class is a "weapon", and a script
// bundle class (an .awi whose "type" combo has a single option, the bundle's
// own type: gibcharacterdef, aifxtable…) a "scriptbundle". An AssetCombo
// target "a | b" takes the first.
func (w *Workspace) LinkerType(gdtType string) string {
	t, _, _ := strings.Cut(strings.ToLower(gdtType), "|")
	t = strings.TrimSpace(t)
	switch {
	case weaponTypes[t]:
		return "weapon"
	case linkerNames[t] != "":
		return linkerNames[t]
	case w.isBundle(t):
		return "scriptbundle"
	}
	return t
}

func (w *Workspace) isBundle(t string) bool {
	sc, err := w.Schema(t)
	if err != nil {
		return false
	}
	e := sc.Lookup("type")
	return e != nil && e.Kind == "Combo" && len(e.Options) == 1 && !e.openOptions
}

// FindAsset returns the definitions of an asset as the linker names it: those
// whose GDT type packs as id.Type.
func (w *Workspace) FindAsset(id asset.ID) ([]Location, error) {
	locs, err := w.Find(id.Name)
	if err != nil {
		return nil, err
	}
	var out []Location
	for _, l := range locs {
		if lt := w.TypeOf(l); lt != "" && strings.EqualFold(w.LinkerType(lt), id.Type) {
			l.Type = lt
			out = append(out, l)
		}
	}
	return out, nil
}
