package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/McReaper/t7_companion/internal/gdt"
)

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
		enc.SetEscapeHTML(false)
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
	var batch string
	var img gdt.ImageSpec
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
			if batch != "" {
				var b []byte
				var err error
				if batch == "-" {
					b, err = io.ReadAll(c.InOrStdin())
				} else {
					b, err = os.ReadFile(batch)
				}
				if err != nil {
					return err
				}
				var items []gdtEditItem
				if err := decodeStrict(b, &items); err != nil {
					return fmt.Errorf("--batch: %w", err)
				}
				return run(c.OutOrStdout(), func(w *gdt.Workspace) (any, error) { return gdtEditBatch(w, er.File, items, !write) })
			}
			if er.Asset == "" {
				return fmt.Errorf("--asset is required (or --batch)")
			}
			if img != (gdt.ImageSpec{}) {
				er.Image = &img
			}
			return run(c.OutOrStdout(), func(w *gdt.Workspace) (any, error) { return gdtEdit(w, er) })
		}}
	ef := edit.Flags()
	ef.StringVar(&er.File, "file", "", "GDT path relative to the root (required)")
	ef.StringVar(&er.Asset, "asset", "", "asset name (required)")
	ef.StringVar(&er.Type, "type", "", "create a full asset of this type")
	ef.StringVar(&er.Parent, "parent", "", "create a derived asset of this parent (must be in the same GDT)")
	ef.StringVar(&er.CopyFrom, "copy-from", "", "create by copying this asset")
	ef.StringArrayVar(&sets, "set", nil, "key=value (repeatable)")
	ef.StringArrayVar(&er.Unset, "unset", nil, "key to remove (repeatable)")
	ef.StringVar(&img.Texture, "image-texture", "", "create an image asset from this texture (relative to the root)")
	ef.StringVar(&img.Semantic, "image-semantic", "", "image semantic (or derive it with --image-material-type + --image-field)")
	ef.StringVar(&img.MaterialType, "image-material-type", "", "material type (techset) the image is for")
	ef.StringVar(&img.Field, "image-field", "", "material field the image will fill, e.g. normalMap")
	ef.StringVar(&batch, "batch", "", "JSON file (or - for stdin): a list of {asset, type, parent, copy_from, image, set, unset}")
	ef.BoolVar(&write, "write", false, "save the change (default: dry run)")
	_ = edit.MarkFlagRequired("file")

	var chkAsset string
	check := &cobra.Command{Use: "check <file.gdt>", Short: "Diagnose a GDT's assets (references, source files, schema, duplicates)", Args: cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, a []string) error {
			return run(c.OutOrStdout(), func(w *gdt.Workspace) (any, error) { return gdtCheck(w, a[0], chkAsset) })
		}}
	check.Flags().StringVar(&chkAsset, "asset", "", "only check this asset")

	refs := &cobra.Command{Use: "refs <asset>", Short: "List the assets that reference an asset (fields and derived parents)", Args: cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, a []string) error {
			return run(c.OutOrStdout(), func(w *gdt.Workspace) (any, error) { return gdtRefs(w, a[0]) })
		}}

	root.AddCommand(find, get, schema, edit, check, refs)
	return root
}
