package radiant

import (
	"context"
	"path"
	"strconv"
	"strings"
)

// State is a Radiant's state, as the bridge's state command gives it.
type State struct {
	Map         string // the open map's path, as Radiant holds it
	Modified    bool   // unsaved changes
	PrefabDepth int    // >0 while editing inside a prefab
	Entities    int    // 0 while a dialog is open
	Selected    int    // selected brushes
	Minimised   bool
	Dialog      string // the open modal dialog's title, "" when none
	Render      string // the 3D view's render method
	View2D      string // the 2D view's axis: XY (Top), XZ or YZ
}

// State reads the Radiant's state; it answers even under a modal dialog.
func (c *Client) State(ctx context.Context) (State, error) {
	r, err := c.Call(ctx, "state", nil)
	if err != nil {
		return State{}, err
	}
	return parseState(string(r.Body)), nil
}

// parseState reads state's key=value lines; a key it doesn't know is a newer
// plugin's, and ignored.
func parseState(body string) State {
	var s State
	for _, line := range strings.Split(body, "\n") {
		k, v, _ := strings.Cut(strings.TrimRight(line, "\r"), "=")
		switch k {
		case "map":
			s.Map = v
		case "modified":
			s.Modified = v == "1"
		case "prefab_depth":
			s.PrefabDepth, _ = strconv.Atoi(v)
		case "entities":
			s.Entities, _ = strconv.Atoi(v)
		case "selected":
			s.Selected, _ = strconv.Atoi(v)
		case "minimised":
			s.Minimised = v == "1"
		case "dialog":
			s.Dialog = v
		case "render":
			s.Render = v
		case "view2d":
			s.View2D = v
		}
	}
	return s
}

// SamePath reports whether two Windows paths name the same file, whatever
// their case and slashes.
func SamePath(a, b string) bool {
	clean := func(p string) string { return path.Clean(strings.ToLower(strings.ReplaceAll(p, `\`, "/"))) }
	return a != "" && b != "" && clean(a) == clean(b)
}
