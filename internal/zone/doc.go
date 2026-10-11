// Package zone reads what the BO3 linker records about a zone it built, and
// the zone sources that decide what it packs. It backs the zone_explain,
// zone_contents and zone_check tools (internal/cli/zone_*.go) and the list of
// missing lookups in build's link stage. It predicts nothing the linker
// decides: each answer comes from a file the linker or the install wrote.
//
// # The linker's records
//
// Every link writes, beside the fastfile, zone_source/all/assetinfo/<zone>.csv
// (Load: each packed asset, its resident and streamed bytes, and its
// parentStack, the chain of parents that pulled it in up to the zone line) and
// <zone>.deps: the lists the zone inherited, every source file the link read
// with the timestamp it had then, and each lookup it made. The report names
// some assets otherwise than the sources do (a material with its category,
// mc/x; a second packing x|dup): Canonical gives the sources' name, and every
// comparison with an assetlist or a GDT goes through it.
//
// A report describes the sources of its link only. ChangedSince lists what
// changed after it: a file in .deps modified since (or gone), and a non-stock
// GDT defining a packed asset saved since, since .deps records a GDT asset by
// hash, not by file.
//
// # What ships stock
//
// A zone's >class chain (share/zone_source/*.class; a .zpkg from the map's own
// zone_source/ first) inherits stock assetlists, zone_source/all/assetlist/
// <list>.csv. For an active entry of an `ignore` list the linker packs only a
// reference to the shipped asset, so a version of your own of it, zoned or in
// a GDT, doesn't ship (a Shadow); commenting the entry out with // makes the
// linker pack yours. `ignore_missing_shipped` lists are packed in full and
// shadow nothing. TestInheritMatchesLinker (opt-in: TA_TOOLS_PATH) checks
// Inherit against the chain the linker wrote atop each .deps of a real
// install.
//
// MissingLookups reads from .deps what the linker looked up and found in no
// GDT: it links on without it and logs nothing.
//
// # What publishing uploads
//
// The Launcher publishes the whole zone/ folder (UploadSize). The linker only
// ever appends to a .xpak, so the data a relink replaced stays in it, read by
// nothing. Images are most of an upload and the part a modder can shrink
// without cutting content: Image.StepSaves estimates what one mipBase step
// down saves.
package zone
