package lz4

import (
	"bytes"
	"encoding/binary"
	"testing"
)

// Literals encodes data as one LZ4 sequence of literals only: valid LZ4,
// enough to build test files.
func Literals(data []byte) []byte {
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

func TestBlockMatch(t *testing.T) {
	// literals "abc", then a match of 6 at offset 3 that overlaps its own output
	got, err := Block([]byte{0x32, 'a', 'b', 'c', 3, 0}, 9)
	if err != nil || string(got) != "abcabcabc" {
		t.Errorf("got %q, %v", got, err)
	}
	long := bytes.Repeat([]byte("x"), 300) // a literal length past 15 continues in 255-steps
	if got, err := Block(Literals(long), 300); err != nil || !bytes.Equal(got, long) {
		t.Errorf("long literals: %v", err)
	}
}

func TestUnpack(t *testing.T) {
	packed := binary.LittleEndian.AppendUint32([]byte("*LZ4*"), 5)
	if got, err := Unpack(append(packed, Literals([]byte("hello"))...)); err != nil || string(got) != "hello" {
		t.Errorf("packed: %q, %v", got, err)
	}
	if got, err := Unpack([]byte("MODEL")); err != nil || string(got) != "MODEL" {
		t.Errorf("plain: %q, %v", got, err)
	}
	for name, b := range map[string][]byte{
		"truncated header": []byte("*LZ4*\x01"),
		"size mismatch":    append([]byte("*LZ4*\x10\x00\x00\x00"), Literals([]byte("abc"))...),
		"bad offset":       append([]byte("*LZ4*\x09\x00\x00\x00"), 0x32, 'a', 'b', 'c', 9, 0),
		"absurd size":      []byte("*LZ4*\xff\xff\xff\xff\x00"),
	} {
		if _, err := Unpack(b); err == nil {
			t.Errorf("%s: want an error", name)
		}
	}
}

func FuzzUnpack(f *testing.F) {
	f.Add([]byte("*LZ4*\x09\x00\x00\x00\x32abc\x03\x00"))
	f.Fuzz(func(t *testing.T, b []byte) {
		_, _ = Unpack(b) // must not panic on any input
	})
}
