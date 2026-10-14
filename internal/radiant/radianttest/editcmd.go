package radianttest

import (
	"fmt"
	"strings"

	"github.com/McReaper/t7_companion/internal/format/mapfile"
)

// editBody is a bridge edit's body: sections from lines "delete <guid>…",
// "replace <guid>" and "import", as PROTOCOL.md's edit reads them.
type editBody struct {
	deletes  []string
	replaces [][2]string // GUID, text
	imp      string
}

func parseEditBody(body string) (editBody, error) {
	var e editBody
	section := ""
	for _, line := range strings.SplitAfter(body, "\n") {
		w := strings.Fields(line)
		switch {
		case strings.HasPrefix(line, "delete ") && len(w) > 1:
			e.deletes, section = append(e.deletes, w[1:]...), ""
		case strings.HasPrefix(line, "replace ") && len(w) == 2:
			e.replaces, section = append(e.replaces, [2]string{w[1], ""}), "replace"
		case strings.TrimSpace(line) == "import" && strings.HasPrefix(line, "import"):
			section = "import"
		case section == "replace":
			e.replaces[len(e.replaces)-1][1] += line
		case section == "import":
			e.imp += line
		case strings.TrimSpace(line) != "":
			return e, fmt.Errorf("text before any delete, replace or import: %s", strings.TrimSpace(line))
		}
	}
	return e, nil
}

// readable is the plugin's check of import text, as far as the fake goes:
// it parses, and every brush's faces (4 at least) or patch reads.
func readable(text string) (*mapfile.Map, error) {
	m, err := mapfile.Parse([]byte(text))
	if err != nil {
		return nil, err
	}
	for _, e := range m.Entities {
		for _, b := range e.Brushes() {
			switch b.Kind() {
			case mapfile.KindFaces:
				faces, err := b.Faces()
				if err != nil {
					return nil, err
				}
				if len(faces) < 4 {
					return nil, fmt.Errorf("a brush with %d faces: it needs 4 at least", len(faces))
				}
			case mapfile.KindCurve, mapfile.KindMesh:
				if _, err := b.Patch(); err != nil {
					return nil, err
				}
			}
		}
	}
	return m, nil
}

// edit applies a bridge edit to the fake's map as the plugin does, as one
// step: deletes, point entities replaced keeping their GUIDs, one import
// whose objects get fresh GUIDs. Its rows list what the import made, world
// brushes first, then each entity (a point entity with one placeholder brush).
func edit(f *Fake, _ string, body []byte) Reply {
	e, err := parseEditBody(string(body))
	if err != nil {
		return Err("bad-request", err.Error())
	}
	m := f.parsed()
	deleted, err := removeAll(m, e.deletes)
	if err != nil {
		return Err("not-found", err.Error())
	}
	var rows []string
	imp := e.imp
	for _, r := range e.replaces {
		if i := entityAt(m, r[0]); i < 0 || len(m.Entities[i].Brushes()) > 0 {
			return Err("refused", r[0]+" is no point entity of the map")
		}
		n, _ := removeAll(m, []string{r[0]})
		deleted += n
		imp += r[1]
	}
	if strings.TrimSpace(imp) != "" {
		in, err := readable(imp)
		if err != nil {
			return Err("bad-request", err.Error())
		}
		replaced := len(in.Entities) - len(e.replaces)
		for i, ent := range in.Entities {
			if ent.Class() == "worldspawn" && i == 0 {
				if row := f.addWorldBrushes(m, ent); row != "" {
					rows = append(rows, row)
				}
				continue
			}
			guid := ""
			if i >= replaced {
				guid = e.replaces[i-replaced][0]
			}
			rows = append(rows, f.addEntity(m, ent, guid))
		}
	}
	f.Text = string(m.Bytes())
	return OK(fmt.Sprintf("deleted=%d", deleted), []byte(strings.Join(rows, "")))
}

// fresh is a GUID the fake gives: one the map has not had.
func (f *Fake) fresh() string {
	f.made++
	return fmt.Sprintf("{%08X-0000-4000-8000-0000000000FA}", 0xFA000000+f.made)
}

func (f *Fake) addWorldBrushes(m *mapfile.Map, from *mapfile.Entity) string {
	w := m.Entities[entityOfClass(m, "worldspawn")]
	row := w.Guid()
	for _, b := range from.Brushes() {
		g := f.fresh()
		stamp(b, g)
		w.Items = append(w.Items, mapfile.Item{Brush: b})
		row += " " + g
	}
	if row == w.Guid() {
		return ""
	}
	return row + "\n"
}

// addEntity appends an imported entity under guid, or a fresh one.
func (f *Fake) addEntity(m *mapfile.Map, e *mapfile.Entity, guid string) string {
	if guid == "" {
		guid = f.fresh()
	}
	items := []mapfile.Item{{Line: "guid \"" + guid + "\"\n"}}
	for _, it := range e.Items {
		if it.Brush == nil && strings.HasPrefix(strings.TrimSpace(it.Line), "guid ") {
			continue
		}
		items = append(items, it)
	}
	e.Items = items
	row := guid
	for _, b := range e.Brushes() {
		g := f.fresh()
		stamp(b, g)
		row += " " + g
	}
	if len(e.Brushes()) == 0 {
		row += " " + f.fresh() // a point entity's placeholder
	}
	m.Entities = append(m.Entities, e)
	return row + "\n"
}

// stamp gives a brush a GUID line, in place of the one it had.
func stamp(b *mapfile.Brush, guid string) {
	body := []string{" guid \"" + guid + "\"\n"}
	for _, l := range b.Body {
		if !strings.HasPrefix(strings.TrimSpace(l), "guid ") {
			body = append(body, l)
		}
	}
	b.Body = body
}

// removeAll deletes entities and brushes by GUID, counting the brushes (a
// point entity's one included); a GUID the map lacks deletes nothing.
func removeAll(m *mapfile.Map, guids []string) (int, error) {
	n := 0
	for _, g := range guids {
		if i := entityAt(m, g); i >= 0 {
			n += max(1, len(m.Entities[i].Brushes()))
			m.Entities = append(m.Entities[:i], m.Entities[i+1:]...)
			continue
		}
		found := false
		for _, e := range m.Entities {
			for k, it := range e.Items {
				if it.Brush != nil && strings.EqualFold(it.Brush.Guid(), g) {
					e.Items = append(e.Items[:k], e.Items[k+1:]...)
					found, n = true, n+1
					break
				}
			}
		}
		if !found {
			return 0, fmt.Errorf("%s", g)
		}
	}
	return n, nil
}

func entityAt(m *mapfile.Map, guid string) int {
	for i, e := range m.Entities {
		if strings.EqualFold(e.Guid(), guid) {
			return i
		}
	}
	return -1
}

func entityOfClass(m *mapfile.Map, class string) int {
	for i, e := range m.Entities {
		if e.Class() == class {
			return i
		}
	}
	return 0
}
