package cli

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"strings"
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

func TestUpload(t *testing.T) {
	dir := t.TempDir()
	writeFiles(t, dir, map[string][]byte{
		"zm_x.xpak":     xpakFile(0x40000, 0x8000), // 224 KiB past the entry's padding
		"en_zm_x.xpak":  xpakFile(0x10000, 0x8000), // and 32 KiB more
		"zm_x.ff":       make([]byte, 100),
		"snd/zm_x.sabs": make([]byte, 50),
		"snd/old.xpak":  xpakFile(0x40000, 0x8000), // uploads, but no fastfile reads a .xpak there
	})
	u := upload(dir)
	var total int64
	for _, n := range []int{0x100 + 0x40000 + 24, 0x100 + 0x10000 + 24, 100, 50, 0x100 + 0x40000 + 24} {
		total += int64(n)
	}
	if u == nil || u.Bytes != total || u.XpakDead != 0x38000+0x8000 || !strings.Contains(u.Advice, "fresh_xpak") {
		t.Errorf("got %+v, want %d bytes, %d dead, and the advice", u, total, 0x38000+0x8000)
	}

	// the share is of every .xpak: 32 KiB dead in 4 MiB plus a clean 32 KiB file is under 1%
	small := t.TempDir()
	writeFiles(t, small, map[string][]byte{"en_zm_x.xpak": xpakFile(0x400000, 0x400000-0x8000), "zm_x.xpak": xpakFile(0x8000, 0x8000)})
	if u := upload(small); u == nil || u.XpakDead != 0x8000 || u.Advice != "" {
		t.Errorf("32 KiB dead of 4 MiB is under 1%%, no advice: %+v", u)
	}

	none := t.TempDir()
	writeFiles(t, none, map[string][]byte{"zm_x.ff": make([]byte, 100)})
	if u := upload(none); u == nil || u.Bytes != 100 || u.XpakDead != 0 || u.Advice != "" {
		t.Errorf("no .xpak, nothing dead, no advice: %+v", u)
	}
	if u := upload(filepath.Join(small, "missing")); u != nil {
		t.Errorf("no zone/ folder: %+v", u)
	}
}
