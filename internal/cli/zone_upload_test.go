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
func TestUpload(t *testing.T) {
	dir := t.TempDir()
	write := func(rel string, b []byte) {
		p := filepath.Join(dir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, b, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("zm_x.xpak", xpakFile(0x40000, 0x8000)) // 224 KiB past the entry's padding
	write("en_zm_x.xpak", xpakFile(0x8000, 0x8000))
	write("zm_x.ff", make([]byte, 100))
	write("snd/zm_x.sabs", make([]byte, 50))
	u := upload(dir)
	var total int64
	for _, n := range []int{0x100 + 0x40000 + 24, 0x100 + 0x8000 + 24, 100, 50} {
		total += int64(n)
	}
	if u == nil || u.Bytes != total || u.XpakDead != 0x38000 || !strings.Contains(u.Advice, "fresh_xpak") {
		t.Errorf("got %+v, want %d bytes, %d dead, and the advice", u, total, 0x38000)
	}

	small := t.TempDir()
	if err := os.WriteFile(filepath.Join(small, "zm_x.xpak"), xpakFile(0x400000, 0x400000-0x8000), 0o644); err != nil {
		t.Fatal(err)
	}
	if u := upload(small); u == nil || u.XpakDead != 0x8000 || u.Advice != "" {
		t.Errorf("32 KiB dead of 4 MiB is under 1%%, no advice: %+v", u)
	}
	if u := upload(filepath.Join(small, "missing")); u != nil {
		t.Errorf("no zone/ folder: %+v", u)
	}
}
