// Package xpak reads the index of a .xpak, the file a link writes beside a
// fastfile with what the game streams from disk (image mips, mesh LODs, light
// probes): a header, the data, then a table of entries, each a key the
// fastfile asks for and where its bytes lie in the data.
//
// The linker only appends to an existing .xpak: data a later link replaces
// stays in the file, read by nothing, and ships with it. Dead measures that
// from the index alone, as a lower bound: in a file written from scratch every
// entry starts where the previous one ends, rounded up to a power of two of at
// most 32 KiB, so data past that boundary belongs to no entry. Not counted: a
// replaced entry whose key stays in the table (only the fastfile knows which
// keys it reads) and a dead run within the padding.
package xpak

import (
	"cmp"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"slices"
)

const (
	magic     = "KAPI"
	version   = 10
	headerLen = 0x48
	entryLen  = 24
	// align is the largest padding between two entries of a fresh file.
	align = 0x8000
)

// Entry is one streamed blob: the key the fastfile reads it by, and its place
// in the data.
type Entry struct {
	Key    uint64
	Offset uint64 // from the start of the data
	Size   uint64
}

// Index is a .xpak's table of contents.
type Index struct {
	FileSize  int64
	DataStart uint64 // file offset of the data
	DataSize  uint64
	Entries   []Entry
}

// Open reads the index of the .xpak at path.
func Open(path string) (*Index, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return nil, err
	}
	return Read(f, st.Size())
}

// Read reads the index of a .xpak of the given size.
func Read(r io.ReaderAt, size int64) (*Index, error) {
	h := make([]byte, headerLen)
	if err := readAt(r, h, 0); err != nil {
		return nil, fmt.Errorf("xpak header: %w", err)
	}
	if string(h[:4]) != magic {
		return nil, errors.New("not a .xpak (no KAPI magic)")
	}
	le := binary.LittleEndian
	if v := le.Uint16(h[6:]); v != version {
		return nil, fmt.Errorf("unknown .xpak version %d", v)
	}
	x := &Index{FileSize: size, DataStart: le.Uint64(h[0x20:]), DataSize: le.Uint64(h[0x28:])}
	n, tableAt, tableLen := le.Uint64(h[0x30:]), le.Uint64(h[0x38:]), le.Uint64(h[0x40:])
	if x.DataStart > uint64(size) || x.DataSize > uint64(size)-x.DataStart {
		return nil, errors.New("xpak data runs past the file")
	}
	if n > uint64(size)/entryLen || tableLen != n*entryLen { // more entries than the file holds, or a count whose length wraps
		return nil, errors.New("xpak entry count doesn't fit the file")
	}
	t := make([]byte, tableLen)
	if err := readAt(r, t, int64(tableAt)); err != nil {
		return nil, fmt.Errorf("xpak entry table: %w", err)
	}
	x.Entries = make([]Entry, n)
	for i := range x.Entries {
		e := t[i*entryLen:]
		x.Entries[i] = Entry{Key: le.Uint64(e), Offset: le.Uint64(e[8:]), Size: le.Uint64(e[16:])}
		if x.Entries[i].Offset > x.DataSize || x.Entries[i].Size > x.DataSize-x.Entries[i].Offset {
			return nil, fmt.Errorf("xpak entry %d runs past the data", i)
		}
	}
	return x, nil
}

// Dead is how many bytes of the data no entry can be read from: at least what
// earlier links left behind (see the package doc).
func (x *Index) Dead() int64 {
	es := slices.Clone(x.Entries)
	slices.SortFunc(es, func(a, b Entry) int { return cmp.Compare(a.Offset, b.Offset) })
	var end, dead uint64
	for _, e := range es {
		if b := roundUp(end); e.Offset > b {
			dead += e.Offset - b
		}
		end = max(end, e.Offset+e.Size)
	}
	if b := roundUp(end); x.DataSize > b {
		dead += x.DataSize - b
	}
	return int64(dead)
}

func roundUp(v uint64) uint64 { return (v + align - 1) &^ (align - 1) }

// readAt fills b: io.ReaderAt may report io.EOF along with a complete read.
func readAt(r io.ReaderAt, b []byte, off int64) error {
	n, err := r.ReadAt(b, off)
	if n == len(b) {
		return nil
	}
	if err == nil || err == io.EOF {
		err = io.ErrUnexpectedEOF
	}
	return err
}
