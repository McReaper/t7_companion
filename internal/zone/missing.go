package zone

import (
	"bufio"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/McReaper/t7_companion/internal/asset"
)

// Missing is an asset the linker looked for during the last link and found in
// no GDT. The linker links on without it and logs nothing: a material then
// shows in game without that texture.
type Missing struct {
	asset.ID
	By asset.ID `json:"named_by"` // the asset whose lookups asked for it
}

func (m Missing) String() string { return m.ID.String() + " (named by " + m.By.String() + ")" }

// variantPrefix is how the linker also looks a name up per platform and
// language: pc:x, [french]x, pc:[french]x.
var variantPrefix = regexp.MustCompile(`^(pc:)?(\[[a-z]+\])?`)

// MissingLookups reads the lookups the last link recorded in dir/<zone>.deps.
// Each `\tgdt,<type>,<name>,<hash>` line is one, its hash empty when no GDT
// defined the asset, under the asset line that asked for it. The linker tries
// a name under its platform and language variants and, for some fields, as
// several types (an icon as emblem and image): a name is missing when none of
// its lookups found it. Engine built-ins ($white) and names a list the zone
// inherits ships (the .deps head names them) are left out.
func MissingLookups(root, dir, zoneName string) ([]Missing, error) {
	f, err := os.Open(filepath.Join(dir, zoneName+".deps"))
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var (
		lists  []string
		parent asset.ID
		order  []string
		first  = map[string]Missing{}  // lower-cased name -> its first failed lookup
		types  = map[string][]string{} // the types it was looked up as
		found  = map[string]bool{}
	)
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() {
		line := sc.Text()
		if body, ok := strings.CutPrefix(line, "\tgdt,"); ok {
			parts := strings.Split(body, ",")
			if len(parts) < 3 {
				continue
			}
			name := variantPrefix.ReplaceAllString(parts[1], "")
			key := strings.ToLower(name)
			if parts[2] != "" {
				found[key] = true
				continue
			}
			if _, seen := first[key]; !seen {
				first[key] = Missing{ID: asset.ID{Type: parts[0], Name: name}, By: parent}
				order = append(order, key)
			}
			types[key] = append(types[key], parts[0])
			continue
		}
		kind, rest, ok := strings.Cut(line, ",")
		switch {
		case !ok || strings.HasPrefix(line, "\t"):
		case kind == "ignore" || kind == "ignore_missing_shipped":
			lists = append(lists, rest)
		default:
			parent = asset.ID{Type: kind, Name: rest}
		}
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	shipped := loadLists(root, lists)
	var out []Missing
	for _, key := range order {
		m := first[key]
		if found[key] || strings.HasPrefix(key, "$") || shipped.any(m.Name, types[key]) {
			continue
		}
		out = append(out, m)
	}
	return out, nil
}

// loadLists reads each list it can: a list the class names but the install
// lacks only means nothing is known shipped from it.
func loadLists(root string, lists []string) Assetlists {
	out := Assetlists{}
	for _, l := range lists {
		one, err := LoadAssetlists(root, []string{l})
		if err != nil {
			continue
		}
		for k, v := range one {
			out[k] = append(out[k], v...)
		}
	}
	return out
}

// any reports whether a list has the name as one of the types.
func (a Assetlists) any(name string, types []string) bool {
	for _, t := range types {
		if len(a.Lookup(asset.ID{Type: t, Name: name})) > 0 {
			return true
		}
	}
	return false
}
