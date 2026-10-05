// Package asset names a Black Ops 3 asset the way the linker does: by the type
// it packs it as (material, xmodel, weapon…) and its name. Every package that
// passes assets to another (gdt, zone, deps) uses this one type, so none has to
// convert or import the other's.
package asset

import "strings"

// ID is an asset: its linker type and its name. The linker compares names
// case-insensitively: key maps on Key.
type ID struct {
	Type string `json:"type,omitempty"`
	Name string `json:"name"`
}

// String is "type name", or the bare name for a zone file or zone package
// (type "csv") and an untyped ID.
func (id ID) String() string {
	if id.Type == "csv" || id.Type == "" {
		return id.Name
	}
	return id.Type + " " + id.Name
}

// Key is id lower-cased, the form to key a map by.
func (id ID) Key() ID {
	return ID{Type: strings.ToLower(id.Type), Name: strings.ToLower(id.Name)}
}
