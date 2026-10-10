// Package gdt reads, validates and writes Black Ops 3 GDT files — the text
// databases APE edits — using the mod tools' own schema sources: the deffiles
// (*.awi) that build APE's property pages, and the techsetdefs that decide which
// fields a material type actually exposes. It backs the gdt_find, gdt_get,
// gdt_schema, gdt_edit, gdt_check and gdt_refs tools (internal/cli/gdt_*.go).
//
// A GDT is a single brace block of assets:
//
//	{
//		"name" ( "type.gdf" )      // a full asset
//		{
//			"key" "value"
//		}
//		"child" [ "parent" ]         // a derived asset: only overrides are listed
//		{
//		}
//	}
//
// Values are stored exactly as written (backslashes already doubled); use Unquote
// and Quote to move between the file form and the real string.
//
// # Schema
//
// The schema comes from the install: deffiles/<type>.awi, the AngelScript that
// builds APE's property pages. There are no .gdf files: the "type.gdf" in a GDT
// names the .awi. AddEntry_* declarations are extracted statically, not
// interpreted; a combo's options too when held in a variable built only from
// literals, heredocs and other such variables (awivars.go). Anything assigned
// from a call, an index or += leaves the combo unchecked, and a true third
// argument makes an editable combo whose list is only suggestions; an
// AssetCombo whose type is such a variable references any of its values. A
// GenerateItemList( Asset, "<type>", …, "<prefix>", … ) call (the list helper
// declares its fields with variables) declares references <prefix>01,
// <prefix>02… and <prefix>Count; Refs reads only the items the count covers.
// A name built from a constant string array walked with a counter
// (gibPrefix + GIB_KEYS[keyIndex++]) is read element by element (awiarrays.go).
// A field declared with different kinds or targets in script branches is
// Varies, not type-checked; its Alts keep the asset types it may reference.
//
// Only the validation callbacks that matter are ported: ValidateLODs and
// surfaceType <error> (an UNRECOVERABLE link error once the material reaches
// collision) as gdt_check warnings, and the two that write fields on change — a
// glossSurfaceType preset writes glossRangeMin/Max, an image semantic sets
// premulAlpha/streamable — applied by gdt_edit (apeeffects.go) unless the
// request sets those fields itself. For materials the schema adds the techsetdef
// a materialType resolves to under share/raw/techsetdefs_stable, following
// #includes: texture slots with their GDT field and image semantic, the category
// materialCategory must equal, HLSL sources. The material type's own file is
// read first and wins; a one-line `Texture( "x" ).image = Image( <field,
// default> )` declares a slot like a block does, while `.tweak` and other
// property assignments only adjust a slot an #include declares. A slot no
// field sets (Image( rain_hit_n )) still carries its fixed image.
//
// # Index
//
// Every GDT in the directories gdtdb scans (bin/converter_gdt_dirs_0.txt) is
// indexed, gob-cached in the user cache dir and refreshed incrementally by
// mtime/size; the MCP server warms it at start and refresh (index.go) keeps it
// fresh in a long session. The techsetdef tree stays fresh too (techset.go): a
// material type it doesn't know rescans it, it is rescanned once it is two
// minutes old, and a resolved techset whose techsetdef or #includes changed is
// read again — a custom techset written mid-session is a material type at
// once. gdtdb's own database, gdtdb/gdt.db, can't stand in
// for it: it sees a GDT only after gdtdb /update, while an edit is checked the
// moment it is written, and it doesn't exist before gdtdb first runs. It is a
// SQLite file with a table per asset type and a column per field, which a
// standard SQLite build (modernc included) reads only after PRAGMA
// writable_schema=ON, and even then not the two tables wider than its
// 2,000-column limit, weaponcamo and scriptbundle ("too many columns on
// scriptbundle"). Parsed GDTs are
// cached by mtime/size (Workspace.Load): never mutate what Load returns; edits
// parse their own copy.
//
// # Editing
//
// gdt_edit is a dry run by default and refuses GDTs listed in stock.gdtdef, GDTs
// outside the directories gdtdb indexes (Workspace.InGDTDirs: they are never
// built), names and keys the format can't hold (ValidName), and a second asset
// of the same type and name. Writes are serialised per file (Workspace.lockFile),
// refused if the file changed on disk since it was read, verified by re-parsing
// before anything is written (File.Save), and atomic with a .bak.
//
// Writes are surgical: the source is kept byte for byte and only the edited
// asset is re-rendered, so a one-field change stays a one-field diff. When one
// GDT holds same-name assets of different types an edit must say which (type);
// lookups go by name and type or by file:line (File.FindAll/AtLine), never the
// first match by name. An edit takes one asset or a batch (assets, applied in
// order in memory so later items see earlier ones, written once and only if no
// item has an error), and can create an image from a texture: the semantic from
// the techset slot it fills, every other field the most common value across
// stock images of that semantic. After a write, gdtdb /update (the build's
// default) indexes the change; build's gdt_rebuild runs /rebuild for the "every
// asset missing" recovery.
//
// # Checking and references
//
// gdt_check is the whole-GDT pass: typed references (AssetCombo targets exist
// and have the right type; $-prefixed engine built-ins skipped), source files on
// disk (xmodel LODs under model_export/, xanim under xanim_export/, image
// baseImage under the root, other Path fields via SetRelativePath), parents,
// cross-GDT duplicates. It skips the "not declared in the .awi" warning for
// existing keys: APE writes script-built fields the static parse can't see.
//
// gdt_refs is the reverse lookup: field values (skinOverride entries too),
// derived parents, and, for a material, the xmodels whose LOD or collision files
// use it — read from the .xmodel_bin like the linker (APE's materials field is
// mostly empty or stale; the custom bullet mesh counts only when
// BulletCollisionLOD is Custom), cached per file (modelmats.go);
// TestModelMaterialsMatchLinker (T7KB_ORACLE=1) checks them against every
// linked map.
//
// # What counts as an error
//
// One rule: anything Treyarch's own stock GDTs do while still linking is not an
// error. TestCalibrate (calibrate_test.go, opt-in via T7KB_CALIBRATE) runs
// gdt_check over a whole install and tallies issues by kind; diff its report
// before and after changing any rule. A derived asset's parent must be in the
// same GDT (gdtdb otherwise fails with "Parent Entity '<name>' does not exist in
// GDT"): Resolved and TypeOf walk the chain inside one file, taking the first
// definition when the file defines the parent's name twice (a material and an
// xmodel), as gdtdb does; gdt_edit refuses a parent from another GDT, and
// deriving from a stock asset is impossible (copy_from it instead). The rules that follow from it are listed on Check (check.go).
//
// # Output size
//
// Answers land in an agent's context: each tool caps its lists or hides
// default-valued fields; the caps are named constants beside each operation in
// internal/cli/gdt_ops.go.
package gdt
