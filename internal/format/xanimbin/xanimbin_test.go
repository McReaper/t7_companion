package xanimbin

import (
	"bytes"
	"encoding/binary"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// literals encodes data as one LZ4 sequence of literals only.
func literals(data []byte) []byte {
	var b bytes.Buffer
	if len(data) < 15 {
		b.WriteByte(byte(len(data) << 4))
	} else {
		b.WriteByte(0xF0)
		for rest := len(data) - 15; ; rest -= 255 {
			if rest < 255 {
				b.WriteByte(byte(rest))
				break
			}
			b.WriteByte(255)
		}
	}
	b.Write(data)
	return b.Bytes()
}

func bin(stream []byte) []byte {
	out := binary.LittleEndian.AppendUint32([]byte("*LZ4*"), uint32(len(stream)))
	return append(out, literals(stream)...)
}

func TestNotes(t *testing.T) {
	stream := []byte("\x75\x16\x00\x00\x0d\x00\x00\x00rmbnt#reload_medium\x00\x00" +
		"j_shoulder_le\x00end\x00" + // bones and plain notes are not typed
		"\x2a\x00\x00\x00sndnt#wpn_reload\x00" + // a note follows its frame (uint32)
		"\x00rmbnt#reload_medium\x00" + // repeated: kept once
		"xyznt#\x00" + // no value
		"\x00toolongkindnt#x\x00")
	got, err := Notes(bin(stream))
	if err != nil {
		t.Fatal(err)
	}
	want := []Note{{"rmb", "reload_medium"}, {"snd", "wpn_reload"}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

// Every animation of a real install decodes; set XANIM_EXPORT to its
// xanim_export directory.
func TestNotesOnInstall(t *testing.T) {
	dir := os.Getenv("XANIM_EXPORT")
	if dir == "" {
		t.Skip("XANIM_EXPORT not set")
	}
	files, _ := filepath.Glob(filepath.Join(dir, "*", "*", "*.xanim_bin"))
	rumbles := 0
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		notes, err := Notes(b)
		if err != nil {
			t.Errorf("%s: %v", f, err)
		}
		for _, n := range notes {
			if n.Kind == "rmb" {
				rumbles++
			}
		}
	}
	t.Logf("%d files, %d rumble notes", len(files), rumbles)
}

func FuzzNotes(f *testing.F) {
	f.Add(bin([]byte("\x00rmbnt#x\x00")))
	f.Fuzz(func(t *testing.T, b []byte) {
		_, _ = Notes(b) // must not panic on any input
	})
}
