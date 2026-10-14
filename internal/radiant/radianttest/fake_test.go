package radianttest

import (
	"encoding/binary"
	"io"
	"testing"
)

// exchange sends each request as a raw frame and checks the frame that answers it.
func exchange(t *testing.T, rw io.ReadWriter, pairs [][2]string) {
	t.Helper()
	for _, p := range pairs {
		if _, err := rw.Write(append(binary.LittleEndian.AppendUint32(nil, uint32(len(p[0]))), p[0]...)); err != nil {
			t.Fatal(err)
		}
		b, err := readFrame(rw)
		if err != nil || string(b) != p[1] {
			t.Errorf("%q: reply %q, %v, want %q", p[0], b, err, p[1])
		}
	}
}

// The fake reads a request's id as the plugin does: an id it can't read gets
// its error under id 0, having no id to answer with.
func TestTheFakeReadsIDsLikeThePlugin(t *testing.T) {
	rw := New(1, "").Dial()
	defer rw.Close()
	exchange(t, rw, [][2]string{
		{"hello 1\n", "0 err bad-request a request starts with its id\n"},
		{"0 hello 1\n", "0 err bad-request request ids start at 1 (0 is for events)\n"},
		{"1234567890123456789 hello 1\n", "0 err bad-request a request starts with its id\n"},
		{"-1 hello 1\n", "0 err bad-request a request starts with its id\n"},
		{"123456789012345678 hello 1\n", "123456789012345678 ok 1 plugin=1.0.0 host=1.1.0 pid=1\n"},
	})
}

// Hello comes first, a dialog makes all but hello, state and menus busy, and
// an unknown command is a bad request.
func TestTheFakeServesInThePluginsOrder(t *testing.T) {
	f := New(7, `C:\a.map`)
	rw := f.Dial()
	defer rw.Close()
	exchange(t, rw, [][2]string{
		{"1 state\n", "1 err bad-request hello first\n"},
		{"2 hello 2\n", "2 err version this plugin serves protocol 1\n"},
		{"3 state\n", "3 err bad-request hello first\n"},
		{"4 hello 1\n", "4 ok 1 plugin=1.0.0 host=1.1.0 pid=7\n"},
		{"5 frobnicate\n", "5 err bad-request unknown command frobnicate\n"},
		{"6 subscribe\n", "6 err bad-request subscribe <event>… (console, map)\n"},
		{"7 reload\n", "7 err bad-request reload <path…>\n"},
	})
	f.Update(func(f *Fake) { f.Dialog = "Save As" })
	exchange(t, rw, [][2]string{
		{"8 reload C:\\b.map\n", "8 err busy Save As\n"},
		{"9 state\n", "9 ok\nmap=C:\\a.map\nmodified=0\nprefab_depth=0\nentities=\nselected=\nminimised=0\ndialog=Save As\nrender=Real Lighting\nview2d=XY (Top)\n"},
	})
	if got := f.Requests(); len(got) != 9 || got[0] != "state" || got[8] != "state" {
		t.Errorf("requests = %q", got)
	}
}
