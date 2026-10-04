package scaffold

import (
	"bytes"
	"os"
	"strings"
)

// assetlistDir holds the stock assetlists every map and mod of the install
// inherits. The ZM Advanced Level template ships its own copies of two of them,
// identical to the stock ones except for the entries it comments out (its
// overrides). Copying them over the install, as the Launcher does, would
// re-enable every line the user commented out to override a stock asset; so
// only the template's overrides are applied: lines are commented out, never
// re-enabled.
const assetlistDir = "zone_source/all/assetlist/"

// AssetlistEdit is the template's overrides of one stock assetlist: entries it
// comments out that are active in the install's copy.
type AssetlistEdit struct {
	File       string   `json:"file"`
	CommentOut []string `json:"comment_out"`
}

// entry reads an assetlist line: its `type,name` (lower-cased, spaces dropped)
// and whether it is commented out.
func entry(line []byte) (string, bool, bool) {
	s := strings.TrimSpace(string(line))
	commented := strings.HasPrefix(s, "//")
	e := strings.ToLower(strings.ReplaceAll(strings.TrimLeft(s, "/"), " ", ""))
	if !strings.Contains(e, ",") {
		return "", false, false
	}
	return e, commented, true
}

// planAssetlist lists the entries the template's copy comments out and the
// install's copy still has active.
func planAssetlist(templatePath, destPath, rel string) (*AssetlistEdit, error) {
	tb, err := os.ReadFile(templatePath)
	if err != nil {
		return nil, err
	}
	db, err := os.ReadFile(destPath)
	if err != nil {
		return nil, err
	}
	active := map[string]bool{}
	for _, l := range bytes.Split(db, []byte("\n")) {
		if e, commented, ok := entry(l); ok && !commented {
			active[e] = true
		}
	}
	edit := &AssetlistEdit{File: rel}
	for _, l := range bytes.Split(tb, []byte("\n")) {
		if e, commented, ok := entry(l); ok && commented && active[e] {
			edit.CommentOut = append(edit.CommentOut, e)
			active[e] = false // once
		}
	}
	return edit, nil
}

// applyAssetlist comments the edit's entries out of the install's copy,
// keeping every other byte, and returns how to put the original back.
func applyAssetlist(destPath string, edit AssetlistEdit) (func(), error) {
	orig, err := os.ReadFile(destPath)
	if err != nil {
		return nil, err
	}
	want := map[string]bool{}
	for _, e := range edit.CommentOut {
		want[e] = true
	}
	var out bytes.Buffer
	for b := orig; len(b) > 0; {
		i := bytes.IndexByte(b, '\n') + 1
		if i == 0 {
			i = len(b)
		}
		line := b[:i]
		b = b[i:]
		if e, commented, ok := entry(line); ok && !commented && want[e] {
			out.WriteString("//")
		}
		out.Write(line)
	}
	if err := writeAtomic(destPath, out.Bytes()); err != nil {
		return nil, err
	}
	return func() { _ = writeAtomic(destPath, orig) }, nil
}

func writeAtomic(path string, b []byte) error {
	tmp := path + ".t7kb.tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
