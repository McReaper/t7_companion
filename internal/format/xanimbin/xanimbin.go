// Package xanimbin reads the typed notetracks of an animation file
// (.xanim_bin, LZ4-compressed: internal/format/lz4). Decompressed, a typed
// notetrack is a NUL-terminated "<kind>nt#<value>" string: rmbnt#<rumble>
// plays a rumble, sndnt#<alias> a sound alias. Plain notes (start, end, script
// notifies) have no kind.
package xanimbin

import (
	"bytes"
	"fmt"

	"github.com/McReaper/t7_companion/internal/format/lz4"
)

// Note is a typed notetrack: its kind ("rmb", "snd") and value.
type Note struct{ Kind, Value string }

// Notes returns an animation file's typed notetracks, each once, in file order.
func Notes(b []byte) ([]Note, error) {
	d, err := lz4.Unpack(b)
	if err != nil {
		return nil, fmt.Errorf("xanim_bin: %w", err)
	}
	var out []Note
	seen := map[Note]bool{}
	for i := 0; ; {
		j := bytes.Index(d[i:], []byte("nt#"))
		if j < 0 {
			return out, nil
		}
		at := i + j
		i = at + 3
		if n, ok := noteAt(d, at); ok && !seen[n] {
			seen[n] = true
			out = append(out, n)
		}
	}
}

// noteAt reads the note whose "nt#" is at: a lower-case kind right after a NUL
// (or the start), and a printable value ending at a NUL.
func noteAt(d []byte, at int) (Note, bool) {
	start := at
	for start > 0 && d[start-1] >= 'a' && d[start-1] <= 'z' {
		start--
	}
	if start == at || at-start > 8 || (start > 0 && d[start-1] != 0) {
		return Note{}, false
	}
	end := at + 3
	for end < len(d) && d[end] >= 0x20 && d[end] < 0x7f {
		end++
	}
	if end == at+3 || end == len(d) || d[end] != 0 {
		return Note{}, false
	}
	return Note{Kind: string(d[start:at]), Value: string(d[at+3 : end])}, true
}
