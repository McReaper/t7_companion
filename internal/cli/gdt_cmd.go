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

// gdtRun opens the workspace, runs fn and prints its answer as indented JSON.
type gdtRun func(out io.Writer, fn func(*gdt.Workspace) (any, error)) error

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
	root.AddCommand(gdtFindCmd(run), gdtGetCmd(run), gdtSchemaCmd(run), gdtEditCmd(run), gdtCheckCmd(run), gdtRefsCmd(run))
	return root
}

func gdtFindCmd(run gdtRun) *cobra.Command {
	return &cobra.Command{Use: "find <asset>", Short: "List every GDT defining an asset", Args: cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, a []string) error {
			return run(c.OutOrStdout(), func(w *gdt.Workspace) (any, error) { return gdtFind(w, a[0]) })
		}}
}

func gdtGetCmd(run gdtRun) *cobra.Command {
	var file, filter string
	var all bool
	cmd := &cobra.Command{Use: "get <asset>", Short: "Show an asset's fields and validation issues", Args: cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, a []string) error {
			return run(c.OutOrStdout(), func(w *gdt.Workspace) (any, error) { return gdtGet(w, a[0], file, filter, all) })
		}}
	cmd.Flags().StringVar(&file, "file", "", "GDT to read from when defined in several")
	cmd.Flags().StringVar(&filter, "filter", "", "only fields whose key contains this")
	cmd.Flags().BoolVar(&all, "all", false, "also show empty and zero fields")
	return cmd
}

func gdtSchemaCmd(run gdtRun) *cobra.Command {
	var mt, filter string
	cmd := &cobra.Command{Use: "schema [type]", Short: "Show the fields an asset type declares (and a material type's techset)", Args: cobra.MaximumNArgs(1),
		RunE: func(c *cobra.Command, a []string) error {
			typ := ""
			if len(a) == 1 {
				typ = a[0]
			}
			return run(c.OutOrStdout(), func(w *gdt.Workspace) (any, error) { return gdtSchema(w, typ, mt, filter) })
		}}
	cmd.Flags().StringVar(&mt, "material-type", "", "for materials: resolve this techset")
	cmd.Flags().StringVar(&filter, "filter", "", "only entries matching this")
	return cmd
}

// editFlags are `gdt edit`'s flags.
type editFlags struct {
	er    gdt.EditRequest
	sets  []string
	write bool
	batch string
	img   gdt.ImageSpec
}

func gdtEditCmd(run gdtRun) *cobra.Command {
	ef := &editFlags{}
	cmd := &cobra.Command{Use: "edit", Short: "Create or update an asset (dry run unless --write)", Args: cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error { return ef.run(c, run) }}
	fs := cmd.Flags()
	fs.StringVar(&ef.er.File, "file", "", "GDT path relative to the root (required)")
	fs.StringVar(&ef.er.Asset, "asset", "", "asset name (required)")
	fs.StringVar(&ef.er.Type, "type", "", "create a full asset of this type")
	fs.StringVar(&ef.er.Parent, "parent", "", "create a derived asset of this parent (must be in the same GDT)")
	fs.StringVar(&ef.er.CopyFrom, "copy-from", "", "create by copying this asset")
	fs.StringArrayVar(&ef.sets, "set", nil, "key=value (repeatable)")
	fs.StringArrayVar(&ef.er.Unset, "unset", nil, "key to remove (repeatable)")
	fs.StringVar(&ef.img.Texture, "image-texture", "", "create an image asset from this texture (relative to the root)")
	fs.StringVar(&ef.img.Semantic, "image-semantic", "", "image semantic (or derive it with --image-material-type + --image-field)")
	fs.StringVar(&ef.img.MaterialType, "image-material-type", "", "material type (techset) the image is for")
	fs.StringVar(&ef.img.Field, "image-field", "", "material field the image will fill, e.g. normalMap")
	fs.StringVar(&ef.batch, "batch", "", "JSON file (or - for stdin): a list of {asset, type, parent, copy_from, image, set, unset}")
	fs.BoolVar(&ef.write, "write", false, "save the change (default: dry run)")
	_ = cmd.MarkFlagRequired("file")
	return cmd
}

func (ef *editFlags) run(c *cobra.Command, run gdtRun) error {
	er := ef.er
	er.Set = map[string]string{}
	for _, s := range ef.sets {
		k, v, ok := strings.Cut(s, "=")
		if !ok {
			return fmt.Errorf("--set wants key=value, got %q", s)
		}
		er.Set[k] = v
	}
	er.DryRun = !ef.write
	if ef.batch != "" {
		items, err := ef.readBatch(c.InOrStdin())
		if err != nil {
			return err
		}
		return run(c.OutOrStdout(), func(w *gdt.Workspace) (any, error) { return gdtEditBatch(w, er.File, items, !ef.write) })
	}
	if er.Asset == "" {
		return fmt.Errorf("--asset is required (or --batch)")
	}
	if ef.img != (gdt.ImageSpec{}) {
		img := ef.img
		er.Image = &img
	}
	return run(c.OutOrStdout(), func(w *gdt.Workspace) (any, error) { return gdtEdit(w, er) })
}

// readBatch reads --batch: a JSON file, or stdin for "-".
func (ef *editFlags) readBatch(stdin io.Reader) ([]gdtEditItem, error) {
	var b []byte
	var err error
	if ef.batch == "-" {
		b, err = io.ReadAll(stdin)
	} else {
		b, err = os.ReadFile(ef.batch)
	}
	if err != nil {
		return nil, err
	}
	var items []gdtEditItem
	if err := decodeStrict(b, &items); err != nil {
		return nil, fmt.Errorf("--batch: %w", err)
	}
	return items, nil
}

func gdtCheckCmd(run gdtRun) *cobra.Command {
	var asset string
	cmd := &cobra.Command{Use: "check <file.gdt>", Short: "Diagnose a GDT's assets (references, source files, schema, duplicates)", Args: cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, a []string) error {
			return run(c.OutOrStdout(), func(w *gdt.Workspace) (any, error) { return gdtCheck(w, a[0], asset) })
		}}
	cmd.Flags().StringVar(&asset, "asset", "", "only check this asset")
	return cmd
}

func gdtRefsCmd(run gdtRun) *cobra.Command {
	return &cobra.Command{Use: "refs <asset>", Short: "List the assets that reference an asset (fields and derived parents)", Args: cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, a []string) error {
			return run(c.OutOrStdout(), func(w *gdt.Workspace) (any, error) { return gdtRefs(w, a[0]) })
		}}
}
