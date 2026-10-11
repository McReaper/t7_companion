package zone

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
)

// xpakFile is a .xpak whose data (from 0x100) holds one entry at its start.
func xpakFile(dataSize, entrySize uint64) []byte {
	le := binary.LittleEndian
	b := make([]byte, 0x100+dataSize)
	copy(b, "KAPI")
	le.PutUint16(b[6:], 10)
	tableAt := uint64(len(b))
	b = le.AppendUint64(b, 1)
	b = le.AppendUint64(b, 0)
	b = le.AppendUint64(b, entrySize)
	for off, v := range map[int]uint64{0x18: 1, 0x20: 0x100, 0x28: dataSize, 0x30: 1, 0x38: tableAt, 0x40: 24} {
		le.PutUint64(b[off:], v)
	}
	return b
}

// The upload is the whole zone/ folder; its .xpak files' dead data is worth a
// fresh link once it is a share of them.
// writeFiles lays files out under dir.
func writeFiles(t *testing.T, dir string, files map[string][]byte) {
	t.Helper()
	for rel, b := range files {
		p := filepath.Join(dir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, b, 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

// The upload is the whole zone/ folder; dead data is counted in the .xpak
// files beside the fastfiles only, where the linker writes them.
func TestUploadSize(t *testing.T) {
	dir := t.TempDir()
	writeFiles(t, dir, map[string][]byte{
		"zm_x.xpak":     xpakFile(0x40000, 0x8000), // 224 KiB past the entry's padding
		"en_zm_x.xpak":  xpakFile(0x10000, 0x8000), // and 32 KiB more
		"zm_x.ff":       make([]byte, 100),
		"snd/zm_x.sabs": make([]byte, 50),
		"snd/old.xpak":  xpakFile(0x40000, 0x8000), // uploads, but no fastfile reads a .xpak there
		"bad.xpak":      []byte("not an xpak"),     // weighed, nothing dead read from it
	})
	u, err := UploadSize(dir)
	xpaks := int64(0x100+0x40000+24) + int64(0x100+0x10000+24) + 11
	want := Upload{Bytes: xpaks + 100 + 50 + int64(0x100+0x40000+24), Xpak: xpaks, XpakDead: 0x38000 + 0x8000}
	if err != nil || u != want {
		t.Errorf("got %+v, %v; want %+v", u, err, want)
	}

	none := t.TempDir()
	writeFiles(t, none, map[string][]byte{"zm_x.ff": make([]byte, 100)})
	if u, err := UploadSize(none); err != nil || u != (Upload{Bytes: 100}) {
		t.Errorf("no .xpak: %+v, %v", u, err)
	}
	if _, err := UploadSize(filepath.Join(none, "missing")); err == nil {
		t.Error("no zone/ folder: want an error")
	}
}
