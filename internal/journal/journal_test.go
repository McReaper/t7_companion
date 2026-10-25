package journal

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// The variable names a folder; 1 and its likes mean debug/ beside the binary,
// and an unset, empty or off value turns the journal off.
func TestDirReadsTheVariable(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	beside := filepath.Join(filepath.Dir(exe), "debug")
	for v, want := range map[string]string{"": "", "0": "", "off": "", " False ": "", "no": "", "1": beside, "TRUE": beside, "on": beside, "yes": beside, `C:\logs\t7kb`: `C:\logs\t7kb`, " /tmp/j ": "/tmp/j"} {
		t.Setenv(Env, v)
		if got := Dir(); got != want {
			t.Errorf("%s=%q: %q, want %q", Env, v, got, want)
		}
	}
}

// Each entry is one JSON line appended, the folder made on the first; an
// image goes beside the journal under the call's time and command.
func TestRecordAppendsOneLineAnEntry(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "made", "here")
	at := time.Date(2026, 10, 25, 21, 35, 1, 123e6, time.UTC)
	name, err := Image(dir, at, "map view", 0, []byte("png"))
	if err != nil || name != "20261025-213501.123-map_view-0.png" {
		t.Fatalf("image %q, %v", name, err)
	}
	if b, err := os.ReadFile(filepath.Join(dir, name)); err != nil || string(b) != "png" {
		t.Errorf("image file: %q, %v", b, err)
	}
	if n, _ := Image(dir, at, `a/b\c d`, 2, nil); n != "20261025-213501.123-a_b_c_d-2.png" {
		t.Errorf("a command's separators: %q", n)
	}
	for _, e := range []Entry{
		{Time: Stamp(at), Via: "cli", Command: "map view", Args: []string{"map", "view", "--out", "v.png"}, Output: "{}", MS: 12, Images: []string{name}},
		{Time: Stamp(at), Via: "mcp", Command: "map_find", Input: map[string]any{"class": "light"}, Failed: true, Output: "no Radiant"},
	} {
		if err := Record(dir, e); err != nil {
			t.Fatal(err)
		}
	}
	f, err := os.Open(filepath.Join(dir, File))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	var got []Entry
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		var e Entry
		if err := json.Unmarshal(sc.Bytes(), &e); err != nil {
			t.Fatalf("line %q: %v", sc.Text(), err)
		}
		got = append(got, e)
	}
	if len(got) != 2 || got[0].Time != "2026-10-25T21:35:01.123" || got[0].Images[0] != name || got[1].Via != "mcp" || !got[1].Failed || got[1].Input["class"] != "light" {
		t.Errorf("journal = %+v", got)
	}
}

// A runaway field is cut at a megabyte, saying how much was left out.
func TestRecordCutsARunawayField(t *testing.T) {
	dir := t.TempDir()
	if err := Record(dir, Entry{Command: "get", Output: strings.Repeat("x", maxField+5), Stdin: "ops"}); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(filepath.Join(dir, File))
	var e Entry
	if err := json.Unmarshal(b, &e); err != nil {
		t.Fatal(err)
	}
	if len(e.Output) != maxField+len("\n… (5 more bytes)") || !strings.HasSuffix(e.Output, "(5 more bytes)") || e.Stdin != "ops" {
		t.Errorf("cut to %d, ends %q", len(e.Output), e.Output[len(e.Output)-20:])
	}
	if err := Record(filepath.Join(dir, File, "sub"), Entry{}); err == nil {
		t.Error("a folder under a file is no error")
	}
}
