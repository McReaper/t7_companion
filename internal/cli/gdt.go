package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"sync"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/spf13/cobra"

	"github.com/McReaper/t7_companion/internal/gdt"
)

// GDT authoring: find/read/validate/edit the text databases APE edits, using the
// install's own deffiles (*.awi) and techsetdefs as the schema. Exposed both as
// `t7kb gdt …` and as the gdt_* MCP tools, with identical behaviour.

var (
	wsMu    sync.Mutex
	wsCache = map[string]*gdt.Workspace{}
)

// workspace opens (and caches, for the long-lived MCP server) the mod-tools root.
func workspace(toolsPath string) (*gdt.Workspace, error) {
	root := strings.TrimRight(firstNonEmpty(toolsPath, os.Getenv("TA_TOOLS_PATH")), `\/`)
	if root == "" {
		return nil, fmt.Errorf("no mod-tools path: pass tools_path / --tools-path or set TA_TOOLS_PATH")
	}
	wsMu.Lock()
	defer wsMu.Unlock()
	if w, ok := wsCache[root]; ok {
		return w, nil
	}
	if _, err := os.Stat(root + "/deffiles"); err != nil {
		return nil, fmt.Errorf("%s has no deffiles/ — not a BO3 mod-tools root", root)
	}
	w, err := gdt.Open(root)
	if err != nil {
		return nil, err
	}
	wsCache[root] = w
	return w, nil
}

// ---- operations shared by CLI and MCP

type gdtAssetView struct {
	Asset     string            `json:"asset"`
	File      string            `json:"file"`
	Line      int               `json:"line"`
	Type      string            `json:"type"`
	Parent    string            `json:"parent,omitempty"`
	Stock     bool              `json:"stock"`
	Own       map[string]string `json:"own_fields"`
	Inherited map[string]string `json:"inherited_fields,omitempty"`
	Hidden    string            `json:"hidden,omitempty"`
	Issues    []gdt.Issue       `json:"issues,omitempty"`
	Others    []gdt.Location    `json:"other_definitions,omitempty"`
}

func gdtFind(w *gdt.Workspace, name string) (any, error) {
	locs, err := w.Find(name)
	if err != nil {
		return nil, err
	}
	out := map[string]any{"asset": name, "definitions": locs}
	if len(locs) > 1 {
		out["warning"] = "defined more than once — the linker fails with `Duplicate asset`; delete the copy you don't want"
	}
	if len(locs) == 0 {
		out["note"] = "not in any GDT under the root — it may still ship inside a stock fastfile"
	}
	return out, nil
}

func gdtGet(w *gdt.Workspace, name, file string, filter string, all bool) (any, error) {
	locs, err := w.Find(name)
	if err != nil {
		return nil, err
	}
	if len(locs) == 0 {
		return nil, fmt.Errorf("asset %q not found in any GDT", name)
	}
	loc := locs[0]
	if file != "" {
		found := false
		for _, l := range locs {
			if strings.EqualFold(l.File, strings.ReplaceAll(file, `\`, "/")) {
				loc, found = l, true
			}
		}
		if !found {
			return nil, fmt.Errorf("asset %q is not defined in %s", name, file)
		}
	}
	f, err := w.Load(loc.File)
	if err != nil {
		return nil, err
	}
	a := f.Find(name)
	typ, fields, err := w.Resolved(a)
	if err != nil {
		return nil, err
	}
	v := gdtAssetView{Asset: name, File: loc.File, Line: loc.Line, Type: typ, Parent: a.Parent, Stock: loc.Stock, Own: map[string]string{}}
	own := map[string]bool{}
	match := func(k string) bool {
		return filter == "" || strings.Contains(strings.ToLower(k), strings.ToLower(filter))
	}
	hidden := 0
	sc, _ := w.Schema(typ)
	isDefault := func(k, val string) bool {
		if val == "" || val == "0" {
			return true
		}
		if sc == nil {
			return false
		}
		e := sc.Lookup(k)
		return e != nil && e.Default != "" && sameValue(e.Default, val)
	}
	show := func(k, val string) bool {
		if !match(k) {
			return false
		}
		if !all && isDefault(k, val) {
			hidden++
			return false
		}
		return true
	}
	for _, fl := range a.Fields {
		own[fl.Key] = true
		if val := gdt.Unquote(fl.Value); show(fl.Key, val) {
			v.Own[fl.Key] = val
		}
	}
	if a.Parent != "" {
		v.Inherited = map[string]string{}
		for _, fl := range fields {
			if val := gdt.Unquote(fl.Value); !own[fl.Key] && show(fl.Key, val) {
				v.Inherited[fl.Key] = val
			}
		}
	}
	if hidden > 0 {
		v.Hidden = fmt.Sprintf("%d empty, zero or default-valued fields not shown (pass all=true)", hidden)
	}
	if iss, _, err := w.Validate(a); err == nil {
		v.Issues = iss
	}
	if len(locs) > 1 {
		v.Others = locs
	}
	return v, nil
}

func gdtSchema(w *gdt.Workspace, typ, materialType, filter string) (any, error) {
	if typ == "" {
		types, err := gdt.AssetTypes(w.Deffiles)
		return map[string]any{"asset_types": types}, err
	}
	sc, err := w.Schema(typ)
	if err != nil {
		return nil, err
	}
	var keep func(*gdt.Entry) bool
	var ts *gdt.Techset
	if typ == "material" && materialType != "" {
		var err error
		if ts, err = w.Techsets.Resolve(materialType); err != nil {
			return nil, err
		}
		ts.File = w.Rel(ts.File)
	}
	switch {
	case filter != "":
		f := strings.ToLower(filter)
		keep = func(e *gdt.Entry) bool { return strings.Contains(strings.ToLower(e.Name+" "+e.Title), f) }
	case ts != nil:
		exposed := ts.ExposedFields()
		core := map[string]bool{"materialType": true, "materialCategory": true, "surfaceType": true, "usage": true}
		keep = func(e *gdt.Entry) bool { return exposed[e.Name] || core[e.Name] }
	}
	entries := []*gdt.Entry{}
	var names []string
	for _, e := range sc.Ordered() {
		if keep == nil || keep(e) {
			entries = append(entries, e)
			names = append(names, e.Name)
		}
	}
	out := map[string]any{"type": typ, "deffile": w.Rel(sc.File), "count": len(entries),
		"note": "declarations read statically from the .awi; its script validation functions are not run"}
	if keep == nil && len(entries) > schemaDetailLimit {
		// Too big to detail in one answer: list names only; filter for details.
		out["fields"] = strings.Join(names, " ")
		out["hint"] = fmt.Sprintf("%d fields: names only. Pass filter (e.g. \"lod\", \"gloss\") for kinds, ranges, options and defaults", len(entries))
	} else {
		out["entries"] = entries
	}
	if len(sc.Entries) < 3 {
		out["warning"] = "this deffile builds most of its fields from script functions at runtime (e.g. list types via asset_list_helper.h), " +
			"so few or no fields can be read statically — copy an existing asset of this type (gdt_get) rather than relying on this schema"
	}
	if typ == "material" {
		if ts != nil {
			out["techset"] = ts
		} else {
			out["material_types"] = len(w.Techsets.Names())
			out["material_types_hint"] = "pass material_type to get that techset's texture slots and only the fields it reads"
		}
	}
	return out, nil
}

// sameValue compares two GDT values, numerically when both parse as numbers
// ("1" == "1.000000").
func sameValue(a, b string) bool {
	if a == b {
		return true
	}
	fa, ea := strconv.ParseFloat(strings.TrimSpace(a), 64)
	fb, eb := strconv.ParseFloat(strings.TrimSpace(b), 64)
	return ea == nil && eb == nil && fa == fb
}

// schemaDetailLimit is the most entries gdt_schema details in one answer; above
// it only names are returned (a material has ~660 fields, a weapon ~1,000).
const schemaDetailLimit = 80

func gdtEdit(w *gdt.Workspace, req gdt.EditRequest) (any, error) {
	return w.Edit(req)
}

// ---- MCP

func gdtToolDefs() []server.ServerTool {
	tp := mcp.WithString("tools_path", mcp.Description("BO3 mod-tools root (default $TA_TOOLS_PATH)."))
	return []server.ServerTool{
		{Tool: mcp.NewTool("gdt_find",
			mcp.WithDescription("Find every GDT that defines an asset (file, line, type, parent, whether the GDT is stock). "+
				"More than one definition is a `Duplicate asset` link error."),
			mcp.WithString("name", mcp.Required(), mcp.Description("Asset name, e.g. \"mtl_my_wall\".")), tp),
			Handler: gdtHandler(func(w *gdt.Workspace, r mcp.CallToolRequest) (any, error) {
				return gdtFind(w, r.GetString("name", ""))
			})},
		{Tool: mcp.NewTool("gdt_get",
			mcp.WithDescription("Read an asset from its GDT: own fields, fields inherited through a derived asset's parent chain, "+
				"and validation issues against the deffile schema (and, for a material, its techset)."),
			mcp.WithString("name", mcp.Required(), mcp.Description("Asset name.")),
			mcp.WithString("file", mcp.Description("Which GDT, when the asset is defined in several (relative to the root).")),
			mcp.WithString("filter", mcp.Description("Only return fields whose key contains this text (e.g. \"lod\", \"map\").")),
			mcp.WithBoolean("all", mcp.Description("Also show empty and zero fields (hidden by default to keep the answer small).")), tp),
			Handler: gdtHandler(func(w *gdt.Workspace, r mcp.CallToolRequest) (any, error) {
				return gdtGet(w, r.GetString("name", ""), r.GetString("file", ""), r.GetString("filter", ""), r.GetBool("all", false))
			})},
		{Tool: mcp.NewTool("gdt_schema",
			mcp.WithDescription("The fields APE declares for an asset type, read from deffiles/<type>.awi: kind, default, min/max, "+
				"combo options, target asset type. For type \"material\" with material_type, also the techsetdef's texture slots "+
				"(GDT field + required image semantic), parameters, category and HLSL sources. No type lists all asset types."),
			mcp.WithString("type", mcp.Description("Asset type (the GDT's \"<type>.gdf\"), e.g. material, xmodel, image, xanim.")),
			mcp.WithString("material_type", mcp.Description("For materials: the materialType (techset) to resolve, e.g. lit, lit_emissive.")),
			mcp.WithString("filter", mcp.Description("Only entries whose name or title contains this text.")), tp),
			Handler: gdtHandler(func(w *gdt.Workspace, r mcp.CallToolRequest) (any, error) {
				return gdtSchema(w, r.GetString("type", ""), r.GetString("material_type", ""), r.GetString("filter", ""))
			})},
		{Tool: mcp.NewTool("gdt_edit",
			mcp.WithDescription("Create or update one asset in a GDT, validated against the deffile schema and (for materials) the "+
				"techset: numeric ranges, combo values, checkboxes, unknown keys, materialType/materialCategory agreement, texture "+
				"fields the techset doesn't read, image semantic mismatches, duplicate names. DRY RUN BY DEFAULT — returns the "+
				"changes and issues; pass write=true to save (atomic, with a .bak). Refuses stock Treyarch GDTs: derive instead "+
				"(parent). After writing, build with gdt_rebuild=true and add the asset to the zone."),
			mcp.WithString("file", mcp.Required(), mcp.Description("GDT path relative to the root, e.g. \"source_data/my_map.gdt\" (created if missing).")),
			mcp.WithString("asset", mcp.Required(), mcp.Description("Asset name.")),
			mcp.WithString("type", mcp.Description("Create a full asset of this type (material, xmodel, image, …).")),
			mcp.WithString("parent", mcp.Description("Create a derived asset of this parent (only overridden fields are stored).")),
			mcp.WithString("copy_from", mcp.Description("Create by copying this asset's type and resolved fields (an xmodel's LOD paths are cleared).")),
			mcp.WithObject("set", mcp.Description("Fields to set, as {\"key\": \"value\"} with real values — paths use single backslashes; escaping is handled.")),
			mcp.WithArray("unset", mcp.Description("Field keys to remove."), mcp.WithStringItems()),
			mcp.WithBoolean("write", mcp.Description("Actually save (default false = dry run).")), tp),
			Handler: gdtHandler(func(w *gdt.Workspace, r mcp.CallToolRequest) (any, error) {
				set := map[string]string{}
				if m, ok := r.GetArguments()["set"].(map[string]any); ok {
					for k, v := range m {
						set[k] = fmt.Sprint(v)
					}
				}
				return gdtEdit(w, gdt.EditRequest{
					File: r.GetString("file", ""), Asset: r.GetString("asset", ""),
					Type: r.GetString("type", ""), Parent: r.GetString("parent", ""), CopyFrom: r.GetString("copy_from", ""),
					Set: set, Unset: r.GetStringSlice("unset", nil), DryRun: !r.GetBool("write", false),
				})
			})},
	}
}

func gdtHandler(fn func(*gdt.Workspace, mcp.CallToolRequest) (any, error)) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		w, err := workspace(req.GetString("tools_path", ""))
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		v, err := fn(w, req)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		out, err := json.Marshal(v) // compact: indentation costs the agent tokens
		if err != nil {
			return mcp.NewToolResultErrorFromErr("marshal", err), nil
		}
		return mcp.NewToolResultText(string(out)), nil
	}
}

// ---- CLI

func newGDTCmd() *cobra.Command {
	var toolsPath string
	root := &cobra.Command{
		Use:   "gdt",
		Short: "Find, read, validate and edit GDT assets (schema from the install's deffiles and techsetdefs)",
	}
	root.PersistentFlags().StringVar(&toolsPath, "tools-path", "", "BO3 mod-tools root (default: $TA_TOOLS_PATH)")
	run := func(out io.Writer, fn func(*gdt.Workspace) (any, error)) error {
		w, err := workspace(toolsPath)
		if err != nil {
			return err
		}
		v, err := fn(w)
		if err != nil {
			return err
		}
		enc := json.NewEncoder(out)
		enc.SetIndent("", "  ")
		return enc.Encode(v)
	}

	find := &cobra.Command{Use: "find <asset>", Short: "List every GDT defining an asset", Args: cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, a []string) error {
			return run(c.OutOrStdout(), func(w *gdt.Workspace) (any, error) { return gdtFind(w, a[0]) })
		}}

	var getFile, getFilter string
	var getAll bool
	get := &cobra.Command{Use: "get <asset>", Short: "Show an asset's fields and validation issues", Args: cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, a []string) error {
			return run(c.OutOrStdout(), func(w *gdt.Workspace) (any, error) { return gdtGet(w, a[0], getFile, getFilter, getAll) })
		}}
	get.Flags().StringVar(&getFile, "file", "", "GDT to read from when defined in several")
	get.Flags().StringVar(&getFilter, "filter", "", "only fields whose key contains this")
	get.Flags().BoolVar(&getAll, "all", false, "also show empty and zero fields")

	var mt, schFilter string
	schema := &cobra.Command{Use: "schema [type]", Short: "Show the fields an asset type declares (and a material type's techset)", Args: cobra.MaximumNArgs(1),
		RunE: func(c *cobra.Command, a []string) error {
			typ := ""
			if len(a) == 1 {
				typ = a[0]
			}
			return run(c.OutOrStdout(), func(w *gdt.Workspace) (any, error) { return gdtSchema(w, typ, mt, schFilter) })
		}}
	schema.Flags().StringVar(&mt, "material-type", "", "for materials: resolve this techset")
	schema.Flags().StringVar(&schFilter, "filter", "", "only entries matching this")

	var er gdt.EditRequest
	var sets []string
	var write bool
	edit := &cobra.Command{Use: "edit", Short: "Create or update an asset (dry run unless --write)", Args: cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			er.Set = map[string]string{}
			for _, s := range sets {
				k, v, ok := strings.Cut(s, "=")
				if !ok {
					return fmt.Errorf("--set wants key=value, got %q", s)
				}
				er.Set[k] = v
			}
			er.DryRun = !write
			return run(c.OutOrStdout(), func(w *gdt.Workspace) (any, error) { return gdtEdit(w, er) })
		}}
	ef := edit.Flags()
	ef.StringVar(&er.File, "file", "", "GDT path relative to the root (required)")
	ef.StringVar(&er.Asset, "asset", "", "asset name (required)")
	ef.StringVar(&er.Type, "type", "", "create a full asset of this type")
	ef.StringVar(&er.Parent, "parent", "", "create a derived asset of this parent")
	ef.StringVar(&er.CopyFrom, "copy-from", "", "create by copying this asset")
	ef.StringArrayVar(&sets, "set", nil, "key=value (repeatable)")
	ef.StringArrayVar(&er.Unset, "unset", nil, "key to remove (repeatable)")
	ef.BoolVar(&write, "write", false, "save the change (default: dry run)")
	_ = edit.MarkFlagRequired("file")
	_ = edit.MarkFlagRequired("asset")

	root.AddCommand(find, get, schema, edit)
	return root
}
