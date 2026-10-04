// Package scaffold creates a new map or mod from the mod tools' own templates,
// the way the Mod Tools Launcher's File > New does: rex/templates/<template> is
// copied onto the root, "template" in every path and line replaced by the name,
// and each Radiant `guid "{…}"` given a fresh one. On top of the Launcher's
// behaviour: no file is ever overwritten (a template also writes map_source/ and,
// for the ZM Basic/Advanced levels, share/raw/sound/ files), binary files are
// copied as they are, and a failed write removes what it had written.
package scaffold

import (
	"bytes"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"
)

// shipped are the stock maps a new map may not be named after (the Launcher's list).
var shipped = []string{
	"mp_aerospace", "mp_apartments", "mp_arena", "mp_banzai", "mp_biodome", "mp_chinatown", "mp_city", "mp_conduit",
	"mp_crucible", "mp_cryogen", "mp_ethiopia", "mp_freerun_01", "mp_freerun_02", "mp_freerun_03", "mp_freerun_04",
	"mp_havoc", "mp_infection", "mp_kung_fu", "mp_metro", "mp_miniature", "mp_nuketown_x", "mp_redwood", "mp_rise",
	"mp_rome", "mp_ruins", "mp_sector", "mp_shrine", "mp_skyjacked", "mp_spire", "mp_stronghold", "mp_veiled",
	"mp_waterpark", "mp_western", "zm_castle", "zm_factory", "zm_genesis", "zm_island", "zm_levelcommon",
	"zm_stalingrad", "zm_zod",
}

// ModZones are the zones a mod template carries, all kept unless chosen otherwise.
var ModZones = []string{"core", "mp", "cp", "zm"}

var validName = regexp.MustCompile(`^[a-z0-9_]+$`)

// File is one file the plan writes.
type File struct {
	Path   string // destination, relative to the root
	source string
}

// MarshalJSON writes a file as its path alone: the answer lands in an agent's context.
func (f File) MarshalJSON() ([]byte, error) { return json.Marshal(f.Path) }

// Plan is what creating a map or mod would write.
type Plan struct {
	Name      string   `json:"name"`
	Kind      string   `json:"kind"` // "map" or "mod"
	Template  string   `json:"template"`
	Files     []File   `json:"files"`
	Conflicts []string `json:"conflicts,omitempty"` // destinations that already exist
	// Assetlists are stock assetlists the template has its own copy of: only its
	// overrides are applied (see assetlistDir).
	Assetlists []AssetlistEdit `json:"assetlist_edits,omitempty"`
	root       string
}

// Templates lists the templates under rex/templates.
func Templates(root string) ([]string, error) {
	entries, err := os.ReadDir(filepath.Join(root, "rex", "templates"))
	if err != nil {
		return nil, fmt.Errorf("no templates: %w", err)
	}
	var out []string
	for _, e := range entries {
		if e.IsDir() {
			out = append(out, e.Name())
		}
	}
	return out, nil
}

// NewPlan validates the name against the template and lists what it would
// write. An empty template picks the plain one for the name's prefix (zm_ →
// ZM Mod Level, mp_ → MP Mod Level, anything else → Mod); zones (mods only)
// defaults to all of ModZones.
func NewPlan(root, name, template string, zones []string) (*Plan, error) {
	name = strings.ToLower(strings.TrimSpace(name))
	if !validName.MatchString(name) {
		return nil, fmt.Errorf("name %q: use only letters, digits and _ (it becomes folder and file names)", name)
	}
	if slices.Contains(shipped, name) {
		return nil, fmt.Errorf("%s is a shipped map: pick another name", name)
	}
	template, err := pickTemplate(root, name, template)
	if err != nil {
		return nil, err
	}
	p := &Plan{Name: name, Kind: "map", Template: template, root: root}
	if strings.EqualFold(template, "mod") {
		p.Kind = "mod"
	} else if err := checkPrefix(name, template); err != nil {
		return nil, err
	}
	keep, err := zoneFilter(p.Kind, zones)
	if err != nil {
		return nil, err
	}
	src := filepath.Join(root, "rex", "templates", template)
	err = filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, _ := filepath.Rel(src, path)
		rel = strings.ReplaceAll(filepath.ToSlash(rel), "template", name)
		if !keep(rel) {
			return nil
		}
		dest := filepath.Join(root, filepath.FromSlash(rel))
		_, statErr := os.Stat(dest)
		if strings.HasPrefix(rel, assetlistDir) && statErr == nil {
			e, err := planAssetlist(path, dest, rel)
			if err == nil && len(e.CommentOut) > 0 {
				p.Assetlists = append(p.Assetlists, *e)
			}
			return err
		}
		p.Files = append(p.Files, File{Path: rel, source: path})
		if statErr == nil {
			p.Conflicts = append(p.Conflicts, rel)
		}
		return nil
	})
	sort.Slice(p.Files, func(i, j int) bool { return p.Files[i].Path < p.Files[j].Path })
	return p, err
}

func pickTemplate(root, name, template string) (string, error) {
	all, err := Templates(root)
	if err != nil {
		return "", err
	}
	if template == "" {
		switch {
		case strings.HasPrefix(name, "zm_"):
			template = "ZM Mod Level"
		case strings.HasPrefix(name, "mp_"):
			template = "MP Mod Level"
		default:
			template = "Mod"
		}
	}
	for _, t := range all {
		if strings.EqualFold(t, template) {
			return t, nil
		}
	}
	return "", fmt.Errorf("no template %q; this install has: %s", template, strings.Join(all, ", "))
}

// checkPrefix: a map's name sets its mode (zm_ or mp_), and must match the template's.
func checkPrefix(name, template string) error {
	up := strings.ToUpper(template)
	switch {
	case strings.HasPrefix(up, "ZM") && !strings.HasPrefix(name, "zm_"):
		return fmt.Errorf("a zombies map's name must start with zm_ (template %s)", template)
	case strings.HasPrefix(up, "MP") && !strings.HasPrefix(name, "mp_"):
		return fmt.Errorf("a multiplayer map's name must start with mp_ (template %s)", template)
	}
	return nil
}

// zoneFilter keeps a mod's <zone>_mod.zone files for the chosen zones only.
func zoneFilter(kind string, zones []string) (func(rel string) bool, error) {
	if kind != "mod" || len(zones) == 0 {
		return func(string) bool { return true }, nil
	}
	for _, z := range zones {
		if !slices.Contains(ModZones, z) {
			return nil, fmt.Errorf("unknown zone %q: choose among %s", z, strings.Join(ModZones, ", "))
		}
	}
	return func(rel string) bool {
		base := filepath.Base(rel)
		z, isZone := strings.CutSuffix(base, "_mod.zone")
		return !isZone || slices.Contains(zones, z)
	}, nil
}

// Write creates the files. It refuses if any destination exists, and removes
// what it wrote if one fails.
func (p *Plan) Write() error {
	if len(p.Conflicts) > 0 {
		return fmt.Errorf("refusing to overwrite %d existing file(s), e.g. %s", len(p.Conflicts), p.Conflicts[0])
	}
	newDirs := p.missingDirs()
	var written []string
	var restores []func()
	undo := func() {
		for _, r := range restores {
			r()
		}
		for _, w := range written {
			_ = os.Remove(w)
		}
		for _, d := range newDirs { // deepest first; os.Remove leaves a directory that isn't empty
			_ = os.Remove(d)
		}
	}
	for _, f := range p.Files {
		dst := filepath.Join(p.root, filepath.FromSlash(f.Path))
		if err := p.writeOne(f.source, dst); err != nil {
			undo()
			return fmt.Errorf("%s: %w (nothing kept)", f.Path, err)
		}
		written = append(written, dst)
	}
	for _, e := range p.Assetlists {
		restore, err := applyAssetlist(filepath.Join(p.root, filepath.FromSlash(e.File)), e)
		if err != nil {
			undo()
			return fmt.Errorf("%s: %w (nothing kept)", e.File, err)
		}
		restores = append(restores, restore)
	}
	return nil
}

// missingDirs lists the directories the files need that don't exist yet,
// deepest first, so a rollback can remove what the write created.
func (p *Plan) missingDirs() []string {
	seen := map[string]bool{}
	var out []string
	for _, f := range p.Files {
		for d := filepath.Dir(filepath.Join(p.root, filepath.FromSlash(f.Path))); len(d) > len(p.root); d = filepath.Dir(d) {
			if seen[d] {
				break
			}
			seen[d] = true
			if _, err := os.Stat(d); err == nil {
				break
			}
			out = append(out, d)
		}
	}
	sort.Slice(out, func(i, j int) bool { return len(out[i]) > len(out[j]) })
	return out
}

func (p *Plan) writeOne(src, dst string) error {
	b, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	if !isBinary(src, b) {
		if b, err = fillTemplate(b, p.Name); err != nil {
			return err
		}
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644) // O_EXCL: never overwrite, even in a race
	if err != nil {
		return err
	}
	if _, err := f.Write(b); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

var guidRE = regexp.MustCompile(`guid "\{(.*)\}"`)

// fillTemplate is the Launcher's per-line rule: a line holding a Radiant guid
// gets a fresh one, every other line has "template" replaced by the name. Line
// endings are kept as they are.
func fillTemplate(b []byte, name string) ([]byte, error) {
	var out bytes.Buffer
	for len(b) > 0 {
		i := bytes.IndexByte(b, '\n') + 1
		if i == 0 {
			i = len(b)
		}
		line := b[:i]
		b = b[i:]
		if bytes.Contains(line, []byte("guid")) {
			g, err := newGUID()
			if err != nil {
				return nil, err
			}
			out.Write(guidRE.ReplaceAllLiteral(line, []byte(`guid "`+g+`"`)))
			continue
		}
		out.Write(bytes.ReplaceAll(line, []byte("template"), []byte(name)))
	}
	return out.Bytes(), nil
}

// newGUID returns a random (version 4) GUID in Radiant's form, {XXXXXXXX-…}.
func newGUID() (string, error) {
	var u [16]byte
	if _, err := rand.Read(u[:]); err != nil {
		return "", err
	}
	u[6] = u[6]&0x0f | 0x40
	u[8] = u[8]&0x3f | 0x80
	return fmt.Sprintf("{%X-%X-%X-%X-%X}", u[0:4], u[4:6], u[6:8], u[8:10], u[10:16]), nil
}

// isBinary: images and other binary files are copied byte for byte.
func isBinary(path string, b []byte) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".png", ".jpg", ".jpeg", ".tga", ".tif", ".tiff", ".dds", ".iwi", ".wav", ".bin":
		return true
	}
	return bytes.IndexByte(b[:min(len(b), 8000)], 0) >= 0
}
