package xpak

import (
	"bytes"
	"encoding/binary"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

// file lays out a .xpak: the header, data of dataSize bytes from 0x100, and
// the entry table after it.
func file(dataSize uint64, es ...Entry) []byte {
	const dataStart = 0x100
	le := binary.LittleEndian
	b := make([]byte, dataStart+dataSize)
	copy(b, magic)
	le.PutUint16(b[6:], version)
	tableAt := uint64(len(b))
	for _, e := range es {
		b = le.AppendUint64(b, e.Key)
		b = le.AppendUint64(b, e.Offset)
		b = le.AppendUint64(b, e.Size)
	}
	le.PutUint64(b[0x10:], uint64(len(b)))
	le.PutUint64(b[0x18:], uint64(len(es)))
	le.PutUint64(b[0x20:], dataStart)
	le.PutUint64(b[0x28:], dataSize)
	le.PutUint64(b[0x30:], uint64(len(es)))
	le.PutUint64(b[0x38:], tableAt)
	le.PutUint64(b[0x40:], uint64(len(es))*entryLen)
	return b
}

func read(t *testing.T, b []byte) *Index {
	t.Helper()
	x, err := Read(bytes.NewReader(b), int64(len(b)))
	if err != nil {
		t.Fatal(err)
	}
	return x
}

func TestRead(t *testing.T) {
	x := read(t, file(0x20000, Entry{1, 0, 0x100}, Entry{2, 0x100, 0x50}))
	if x.DataStart != 0x100 || x.DataSize != 0x20000 || len(x.Entries) != 2 || x.Entries[1] != (Entry{2, 0x100, 0x50}) {
		t.Errorf("got %+v", x)
	}
}

// Padding up to the next 32 KiB boundary is a fresh file's layout; anything
// past it is data no entry reads, wherever it lies.
func TestDead(t *testing.T) {
	cases := []struct {
		name string
		data uint64
		es   []Entry
		dead int64
	}{
		{"packed back to back", 0x300, []Entry{{1, 0, 0x100}, {2, 0x100, 0x200}}, 0},
		{"padded to 32 KiB", 0x8100, []Entry{{1, 0, 0x10}, {2, 0x8000, 0x100}}, 0},
		{"padded to 16 bytes, unsorted", 0x130, []Entry{{2, 0x110, 0x20}, {1, 0, 0x101}}, 0},
		{"a replaced entry's old bytes", 0x30000, []Entry{{1, 0, 0x8000}, {2, 0x20000, 0x10000}}, 0x18000},
		{"left at the end", 0x40000, []Entry{{1, 0, 0x8000}}, 0x38000},
		{"overlapping entries", 0x10000, []Entry{{1, 0, 0x10000}, {2, 0x100, 0x10}}, 0},
		{"empty", 0x10000, nil, 0x10000},
	}
	for _, c := range cases {
		if got := read(t, file(c.data, c.es...)).Dead(); got != c.dead {
			t.Errorf("%s: dead %#x, want %#x", c.name, got, c.dead)
		}
	}
}

func TestReadRejects(t *testing.T) {
	good := file(0x100, Entry{1, 0, 0x100})
	cases := map[string]func(b []byte){
		"magic":         func(b []byte) { copy(b, "KAPX") },
		"version":       func(b []byte) { b[6] = 11 },
		"data size":     func(b []byte) { binary.LittleEndian.PutUint64(b[0x28:], 1<<40) },
		"table length":  func(b []byte) { binary.LittleEndian.PutUint64(b[0x40:], 25) },
		"table offset":  func(b []byte) { binary.LittleEndian.PutUint64(b[0x38:], 1<<40) },
		"entry offset":  func(b []byte) { binary.LittleEndian.PutUint64(b[len(b)-16:], 0x200) },
		"entry size":    func(b []byte) { binary.LittleEndian.PutUint64(b[len(b)-8:], 0x101) },
		"entry count":   func(b []byte) { binary.LittleEndian.PutUint64(b[0x30:], 1<<60) },
		"short header":  func(b []byte) {},
		"no entry room": func(b []byte) { binary.LittleEndian.PutUint64(b[0x30:], 2) },
	}
	for name, corrupt := range cases {
		b := bytes.Clone(good)
		corrupt(b)
		if name == "short header" {
			b = b[:0x20]
		}
		if _, err := Read(bytes.NewReader(b), int64(len(b))); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}

func FuzzRead(f *testing.F) {
	f.Add(file(0x300, Entry{1, 0, 0x100}, Entry{2, 0x8000 - 0x100, 0x10}))
	f.Add([]byte(magic))
	f.Fuzz(func(t *testing.T, b []byte) {
		x, err := Read(bytes.NewReader(b), int64(len(b)))
		if err != nil {
			return
		}
		if d := x.Dead(); d < 0 || uint64(d) > x.DataSize {
			t.Fatalf("dead %d of %d bytes", d, x.DataSize)
		}
	})
}

// Every .xpak in a mod-tools install (TA_TOOLS_PATH) reads: entries inside the
// data, no bits above an entry's size, no two entries sharing bytes.
func TestInstallXpaks(t *testing.T) {
	root := os.Getenv("TA_TOOLS_PATH")
	if root == "" {
		t.Skip("set TA_TOOLS_PATH to read the install's .xpak files")
	}
	paths, _ := filepath.Glob(filepath.Join(root, "usermaps", "*", "zone", "*.xpak"))
	mods, _ := filepath.Glob(filepath.Join(root, "mods", "*", "zone", "*.xpak"))
	paths = append(paths, mods...)
	if len(paths) == 0 {
		t.Skip("no .xpak under usermaps/ or mods/")
	}
	for _, p := range paths {
		x, err := Open(p)
		if err != nil {
			t.Errorf("%s: %v", p, err)
			continue
		}
		if overlapping(x) > 0 {
			t.Errorf("%s: %d entries share bytes with another", p, overlapping(x))
		}
		t.Logf("%-60s %6.1f MB, %5.1f MB dead", filepath.Base(p), float64(x.FileSize)/1e6, float64(x.Dead())/1e6)
	}
}

// overlapping counts entries that start before an earlier one ends.
func overlapping(x *Index) int {
	es := slices.Clone(x.Entries)
	slices.SortFunc(es, func(a, b Entry) int { return cmpU64(a.Offset, b.Offset) })
	var end uint64
	n := 0
	for _, e := range es {
		if e.Offset < end && e.Size > 0 {
			n++
		}
		end = max(end, e.Offset+e.Size)
	}
	return n
}
