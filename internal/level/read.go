package level

import (
	"context"
	"fmt"

	"github.com/McReaper/t7_companion/internal/format/mapfile"
	"github.com/McReaper/t7_companion/internal/radiant"
)

// Snapshot is the map open in a Radiant, read once.
type Snapshot struct {
	State radiant.State
	Map   *mapfile.Map
	IDs   *IDs
	Sel   Selection
}

// Read reads the map open in the Radiant: its state, its entities and the
// selection.
func Read(ctx context.Context, c *radiant.Client) (*Snapshot, error) {
	st, err := c.State(ctx)
	if err != nil {
		return nil, err
	}
	if st.Dialog != "" {
		return nil, fmt.Errorf("the dialog %q is open in Radiant: the map can't be read until it closes", st.Dialog)
	}
	r, err := c.Call(ctx, "entities", nil)
	if err != nil {
		return nil, err
	}
	m, err := mapfile.Parse(r.Body)
	if err != nil {
		return nil, fmt.Errorf("the map text Radiant gave: %w", err)
	}
	sel, err := c.Call(ctx, "selection", nil)
	if err != nil {
		return nil, err
	}
	return &Snapshot{State: st, Map: m, IDs: NewIDs(GUIDs(m)), Sel: ParseSelection(string(sel.Body))}, nil
}
