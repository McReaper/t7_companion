package radianttest

import (
	"encoding/binary"
	"testing"
)

// The fake reads a request's id as the plugin does: an id it can't read gets
// its error under id 0, having no id to answer with.
func TestTheFakeReadsIDsLikeThePlugin(t *testing.T) {
	rw := New(1, "").Dial()
	defer rw.Close()
	for _, tc := range []struct{ request, reply string }{
		{"hello 1\n", "0 err bad-request a request starts with its id\n"},
		{"0 hello 1\n", "0 err bad-request request ids start at 1 (0 is for events)\n"},
		{"1234567890123456789 hello 1\n", "0 err bad-request a request starts with its id\n"},
		{"-1 hello 1\n", "0 err bad-request a request starts with its id\n"},
		{"123456789012345678 hello 1\n", "123456789012345678 ok 1 plugin=1.0.0 host=1.1.0 pid=1\n"},
	} {
		if _, err := rw.Write(append(binary.LittleEndian.AppendUint32(nil, uint32(len(tc.request))), tc.request...)); err != nil {
			t.Fatal(err)
		}
		b, err := readFrame(rw)
		if err != nil || string(b) != tc.reply {
			t.Errorf("%q: reply %q, %v, want %q", tc.request, b, err, tc.reply)
		}
	}
}
