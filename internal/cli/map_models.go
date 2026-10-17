package cli

import (
	"cmp"
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"

	"github.com/McReaper/t7_companion/internal/gdt"
	"github.com/McReaper/t7_companion/internal/level"
	"github.com/McReaper/t7_companion/internal/level/picture"
	"github.com/McReaper/t7_companion/internal/radiant"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/spf13/cobra"
)

// mapModelsArgs are map_models' filter, and whether to lay the models found
// out on a board in Radiant.
type mapModelsArgs struct {
	Name      string // comma-separated alternatives; a name's words must all be in it
	Limit     int
	Board     bool
	Size      int
	ToolsPath string
	PID       int
}

// mapModelsOut is a map_models answer: the models found, most placed by the
// install's maps first, and with board, which label of the image is which.
type mapModelsOut struct {
	Found  int          `json:"found"` // how many match, before the limit
	Models []modelOut   `json:"models"`
	Board  []modelLabel `json:"board,omitempty"`
	W      int          `json:"w,omitempty"`
	H      int          `json:"h,omitempty"`
	Notes  []string     `json:"notes,omitempty"`
}

type modelOut struct {
	Name string `json:"name"`
	Uses int    `json:"uses,omitempty"` // entities of the install's maps that place it
}

type modelLabel struct {
	Label string `json:"label"` // at the model's foot in the image
	Model string `json:"model"`
	At    [2]int `json:"at"` // the label's pixel
}

// The install's model uses, read once per install.
var (
	modelUseMu sync.Mutex
	modelUse   = map[string]map[string]int{} // lower-cased root -> model -> entities placing it
)

func modelUses(root string) map[string]int {
	key := strings.ToLower(filepath.Clean(root))
	modelUseMu.Lock()
	defer modelUseMu.Unlock()
	if u, ok := modelUse[key]; ok {
		return u
	}
	u := level.ScanModels(root)
	modelUse[key] = u
	return u
}

// mapModels lists the models whose names match, most placed first, and with
// board places the first 16 in Radiant and shoots them.
func mapModels(ctx context.Context, a mapModelsArgs) (*mapModelsOut, []byte, error) {
	if a.Limit < 0 || a.Limit > maxMaterialsLimit {
		return nil, nil, fmt.Errorf("limit %d: at most %d", a.Limit, maxMaterialsLimit)
	}
	if a.Size < 0 || a.Size > maxViewSize {
		return nil, nil, fmt.Errorf("size %d: at most %d pixels", a.Size, maxViewSize)
	}
	root := toolsRoot(a.ToolsPath)
	var c *radiant.Client
	var snap *level.Snapshot
	if a.Board || root == "" {
		var err error
		if c, err = radiant.Connect(ctx, radiantMachine, a.PID); err != nil {
			return nil, nil, err
		}
		defer c.Close()
		if snap, err = level.Read(ctx, c); err != nil {
			return nil, nil, err
		}
		root = installOf(snap.State.Map)
	}
	w, err := workspace(root)
	if err != nil {
		return nil, nil, err
	}
	out := listModels(w, root, a)
	if !a.Board || len(out.Models) == 0 {
		return out, nil, nil
	}
	return showModels(ctx, c, snap, out, a.Size)
}

// listModels filters the install's models by name, most placed first.
func listModels(w *gdt.Workspace, root string, a mapModelsArgs) *mapModelsOut {
	uses := modelUses(root)
	var found []modelOut
	for _, m := range w.Models() {
		if nameOf(m.Name, a.Name) {
			found = append(found, modelOut{Name: m.Name, Uses: uses[m.Name]})
		}
	}
	slices.SortFunc(found, func(x, y modelOut) int { return cmp.Or(cmp.Compare(y.Uses, x.Uses), cmp.Compare(x.Name, y.Name)) })
	limit := cmp.Or(a.Limit, materialsLimit)
	return &mapModelsOut{Found: len(found), Models: append([]modelOut{}, found[:min(limit, len(found))]...)}
}

// showModels places the first models listed in Radiant and shoots them.
func showModels(ctx context.Context, c *radiant.Client, snap *level.Snapshot, out *mapModelsOut, size int) (*mapModelsOut, []byte, error) {
	var names []string
	for _, m := range out.Models[:min(len(out.Models), shownLimit)] {
		names = append(names, m.Name)
	}
	b, err := level.ShowModels(ctx, c, snap, names, size)
	if err != nil {
		return nil, nil, err
	}
	png, err := picture.PNG(b.Shot.Image)
	if err != nil {
		return nil, nil, err
	}
	out.W, out.H, out.Notes = b.Shot.Image.Rect.Dx(), b.Shot.Image.Rect.Dy(), b.Shot.Notes
	at := map[string][2]int{}
	for _, l := range b.Shot.Labels {
		at[l.ID] = [2]int{l.X, l.Y}
	}
	for _, s := range b.Swatches {
		out.Board = append(out.Board, modelLabel{Label: s.Label, Model: s.Name, At: at[s.Label]})
	}
	if len(out.Models) > len(names) {
		out.Notes = append(out.Notes, fmt.Sprintf("the board shows the first %d", len(names)))
	}
	return out, png, nil
}

func mapModelsTool() server.ServerTool {
	return server.ServerTool{Tool: mcp.NewTool("map_models",
		mcp.WithDescription("Find models (xmodels) to place, a misc_model's model key: by name words (comma-separated "+
			"alternatives), most placed by the install's maps first (uses: their entities that place it). board places "+
			"the first 16 above the map in Radiant, a misc_model each 256 units apart, shoots them from the south-west "+
			"under fake lighting and takes them off the map again (Radiant's redo holds them): the image, then board "+
			"says which label is which model."),
		mcp.WithString("name", mcp.Description("Words the name holds, all of them; comma-separated alternatives (\"chair wood, barrel\").")),
		mcp.WithNumber("limit", mcp.Description("How many to list (default 20, at most 200).")),
		mcp.WithBoolean("board", mcp.Description("Place the first 16 in Radiant and answer their image too (default false).")),
		mcp.WithNumber("size", mcp.Description("With board: the image's longer side in pixels (default 768, at most 1568).")),
		mcp.WithString("tools_path", mcp.Description("BO3 mod-tools root (default $TA_TOOLS_PATH, else the running Radiant's).")),
		mcp.WithNumber("pid", mcp.Description("With board: the Radiant to use when several run."))),
		Handler: func(ctx context.Context, r mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			out, png, err := mapModels(ctx, mapModelsArgs{Name: r.GetString("name", ""), Limit: r.GetInt("limit", 0),
				Board: r.GetBool("board", false), Size: r.GetInt("size", 0), ToolsPath: r.GetString("tools_path", ""), PID: r.GetInt("pid", 0)})
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			res := jsonResult(out)
			if png != nil {
				res.Content = append(res.Content, mcp.NewImageContent(base64.StdEncoding.EncodeToString(png), "image/png"))
			}
			return res, nil
		}}
}

func newMapModelsCmd() *cobra.Command {
	var a mapModelsArgs
	var out string
	models := &cobra.Command{Use: "models",
		Short: "Find models to place, most placed by the install's maps first; --board places them in Radiant",
		Args:  cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			res, png, err := mapModels(c.Context(), a)
			if err != nil {
				return err
			}
			if png != nil {
				if err := os.WriteFile(out, png, 0o644); err != nil {
					return err
				}
				fmt.Fprintf(c.ErrOrStderr(), "t7kb: wrote %s\n", out)
			}
			return printIndented(c.OutOrStdout())(res, nil)
		}}
	fl := models.Flags()
	fl.StringVar(&a.Name, "name", "", "words the name holds, all of them; comma-separated alternatives")
	fl.IntVar(&a.Limit, "limit", 0, "how many to list (default 20, at most 200)")
	fl.BoolVar(&a.Board, "board", false, "place the first 16 in Radiant and write their image")
	fl.IntVar(&a.Size, "size", 0, "with --board: the image's longer side in pixels (default 768)")
	fl.StringVar(&a.ToolsPath, "tools-path", "", "BO3 mod-tools root (default: $TA_TOOLS_PATH, else the running Radiant's)")
	fl.IntVar(&a.PID, "pid", 0, "with --board: the Radiant to use when several run")
	fl.StringVar(&out, "out", "map_models.png", "with --board: where to write the image")
	return models
}
