package cli

import (
	"archive/zip"
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// writeDBZip writes path+".zip" holding one t7kb.db entry with content.
func writeDBZip(t *testing.T, path, content string) {
	t.Helper()
	f, err := os.Create(path + ".zip")
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(f)
	w, err := zw.Create(filepath.Base(path))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write([]byte(content)); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// inUse makes every rename fail, as Windows does while a t7kb server holds the db open.
func inUse(t *testing.T) {
	old := rename
	rename = func(string, string) error { return errors.New("file in use") }
	t.Cleanup(func() { rename = old })
}

func dbPath(t *testing.T) string { return filepath.Join(t.TempDir(), "t7kb.db") }

func TestEnsureDBUnpacksAMissingDB(t *testing.T) {
	path := dbPath(t)
	writeDBZip(t, path, "new db")
	var log bytes.Buffer
	if err := ensureDB(path, &log); err != nil {
		t.Fatal(err)
	}
	if got := readFile(t, path); got != "new db" {
		t.Errorf("db = %q, want the zip's entry", got)
	}
	if !strings.Contains(log.String(), "unpacking t7kb.db.zip") {
		t.Errorf("log = %q, want the one-time unpack announced", log.String())
	}
	if exists(path + ".tmp") {
		t.Error("the .tmp the unpack wrote through is left behind")
	}
}

func TestEnsureDBWithoutDBOrZip(t *testing.T) {
	err := ensureDB(dbPath(t), &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "no t7kb.db.zip to unpack") {
		t.Errorf("err = %v, want it to name the missing zip", err)
	}
}

func TestEnsureDBKeepsAnExistingDB(t *testing.T) {
	path := dbPath(t)
	writeFile(t, path, "old db")
	writeDBZip(t, path, "new db") // what an update downloads: unpack-db's job, not ensureDB's
	var log bytes.Buffer
	if err := ensureDB(path, &log); err != nil {
		t.Fatal(err)
	}
	if got := readFile(t, path); got != "old db" {
		t.Errorf("db = %q: ensureDB must not unpack over an existing db at every start", got)
	}
	if log.Len() != 0 {
		t.Errorf("log = %q, want nothing", log.String())
	}
}

func TestEnsureDBSwapsInTheStagedDB(t *testing.T) {
	path := dbPath(t)
	writeFile(t, path, "old db")
	writeFile(t, stagedDB(path), "new db")
	var log bytes.Buffer
	if err := ensureDB(path, &log); err != nil {
		t.Fatal(err)
	}
	if got := readFile(t, path); got != "new db" {
		t.Errorf("db = %q, want the staged one", got)
	}
	if exists(stagedDB(path)) {
		t.Error("the staged db is still there after the swap")
	}
	if log.Len() != 0 {
		t.Errorf("log = %q, want nothing", log.String())
	}
}

func TestEnsureDBServesTheOldDBWhileItIsInUse(t *testing.T) {
	path := dbPath(t)
	writeFile(t, path, "old db")
	writeFile(t, stagedDB(path), "new db")
	inUse(t)
	var log bytes.Buffer
	if err := ensureDB(path, &log); err != nil {
		t.Fatalf("err = %v: an update waiting must not stop the old db from serving", err)
	}
	if got := readFile(t, path); got != "old db" {
		t.Errorf("db = %q", got)
	}
	if got := readFile(t, stagedDB(path)); got != "new db" {
		t.Errorf("staged = %q, want it kept for the next start", got)
	}
	for _, want := range []string{"t7kb.db.new", "file in use", "no other t7kb runs"} {
		if !strings.Contains(log.String(), want) {
			t.Errorf("log = %q, want %q", log.String(), want)
		}
	}
}

func TestSyncDBUnpacksAFirstInstall(t *testing.T) {
	path := dbPath(t)
	writeDBZip(t, path, "new db")
	var out bytes.Buffer
	if err := syncDB(path, &out); err != nil {
		t.Fatal(err)
	}
	if got := readFile(t, path); got != "new db" {
		t.Errorf("db = %q", got)
	}
	if exists(stagedDB(path)) || exists(stagedDB(path)+".tmp") {
		t.Error("staging files left behind")
	}
	for _, want := range []string{"unpacking t7kb.db.zip (0.0 GB)", "t7kb.db now matches t7kb.db.zip."} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("out = %q, want %q", out.String(), want)
		}
	}
}

func TestSyncDBReplacesAStaleDB(t *testing.T) {
	path := dbPath(t)
	writeFile(t, path, "old db") // same size, other bytes: the CRC tells them apart
	writeDBZip(t, path, "new db")
	var out bytes.Buffer
	if err := syncDB(path, &out); err != nil {
		t.Fatal(err)
	}
	if got := readFile(t, path); got != "new db" {
		t.Errorf("db = %q, want the zip's", got)
	}
	if !strings.Contains(out.String(), "now matches") {
		t.Errorf("out = %q", out.String())
	}
}

func TestSyncDBLeavesACurrentDBAlone(t *testing.T) {
	path := dbPath(t)
	writeDBZip(t, path, "same db")
	writeFile(t, path, "same db")
	writeFile(t, stagedDB(path), "older db") // left by an earlier update
	old := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	if err := os.Chtimes(path, old, old); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := syncDB(path, &out); err != nil {
		t.Fatal(err)
	}
	if info, err := os.Stat(path); err != nil || !info.ModTime().Equal(old) {
		t.Errorf("db rewritten (%v): an update with the same database must not unpack it again", err)
	}
	if exists(stagedDB(path)) {
		t.Error("a staged db older than the current one would replace it at the next start")
	}
	if got := out.String(); got != "t7kb.db already matches t7kb.db.zip.\n" {
		t.Errorf("out = %q", got)
	}
}

func TestSyncDBStagesWhileTheDBIsInUse(t *testing.T) {
	path := dbPath(t)
	writeFile(t, path, "old")
	writeDBZip(t, path, "new db")
	inUse(t)
	var out bytes.Buffer
	if err := syncDB(path, &out); err != nil {
		t.Fatalf("err = %v: a db in use is not a failed install", err)
	}
	if got := readFile(t, path); got != "old" {
		t.Errorf("db = %q", got)
	}
	if got := readFile(t, stagedDB(path)); got != "new db" {
		t.Errorf("staged = %q, want the zip's", got)
	}
	for _, want := range []string{"unpacked as t7kb.db.new", "t7kb.db is in use (file in use)", "close every session"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("out = %q, want %q", out.String(), want)
		}
	}
	if strings.Contains(out.String(), "now matches") {
		t.Errorf("out = %q claims the swap happened", out.String())
	}
}

func TestSyncDBReusesTheStagedDB(t *testing.T) {
	path := dbPath(t)
	writeFile(t, path, "old")
	writeDBZip(t, path, "new db")
	writeFile(t, stagedDB(path), "new db") // staged by an earlier run while the db was in use
	old := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	if err := os.Chtimes(stagedDB(path), old, old); err != nil {
		t.Fatal(err)
	}
	inUse(t)
	var out bytes.Buffer
	if err := syncDB(path, &out); err != nil {
		t.Fatal(err)
	}
	if info, err := os.Stat(stagedDB(path)); err != nil || !info.ModTime().Equal(old) {
		t.Errorf("staged db rewritten (%v): it already held the zip's database", err)
	}
	if strings.Contains(out.String(), "unpacking") {
		t.Errorf("out = %q, want no second unpack", out.String())
	}
}

func TestSyncDBReplacesAStaleStagedDB(t *testing.T) {
	path := dbPath(t)
	writeFile(t, path, "old")
	writeDBZip(t, path, "new db")
	writeFile(t, stagedDB(path), "mid db")
	var out bytes.Buffer
	if err := syncDB(path, &out); err != nil {
		t.Fatal(err)
	}
	if got := readFile(t, path); got != "new db" {
		t.Errorf("db = %q, want the zip's, not the stale staged one", got)
	}
}

func TestSyncDBErrors(t *testing.T) {
	path := dbPath(t)
	if err := syncDB(path, &bytes.Buffer{}); err == nil {
		t.Error("no zip: want an error")
	}

	writeFile(t, path+".zip", "not a zip")
	if err := syncDB(path, &bytes.Buffer{}); err == nil {
		t.Error("a corrupt zip (an interrupted download): want an error")
	}

	f, err := os.Create(path + ".zip")
	if err != nil {
		t.Fatal(err)
	}
	if err := zip.NewWriter(f).Close(); err != nil {
		t.Fatal(err)
	}
	f.Close()
	if err := syncDB(path, &bytes.Buffer{}); err == nil || !strings.Contains(err.Error(), "no database entry") {
		t.Errorf("an empty zip: err = %v", err)
	}
}

// A database or staged copy that can't be read is an error, not "it differs":
// unpacking over it would hide the fault. A directory the entry's size stands in
// for the file; reading it fails on every OS.
func TestSyncDBReportsAnUnreadableFile(t *testing.T) {
	for _, suffix := range []string{"", ".new"} {
		path := dbPath(t)
		if err := os.Mkdir(path+suffix, 0o755); err != nil {
			t.Fatal(err)
		}
		info, err := os.Stat(path + suffix)
		if err != nil {
			t.Fatal(err)
		}
		writeDBZip(t, path, strings.Repeat("x", int(info.Size())))
		if suffix != "" {
			writeFile(t, path, "a db the zip doesn't hold")
		}
		if err := syncDB(path, &bytes.Buffer{}); err == nil {
			t.Errorf("t7kb.db%s unreadable: want an error", suffix)
		}
	}
}

func TestSyncDBReportsAFailedUnpack(t *testing.T) {
	path := dbPath(t)
	writeFile(t, path, "old")
	writeDBZip(t, path, "new db")
	if err := os.Mkdir(stagedDB(path)+".tmp", 0o755); err != nil { // where the unpack writes
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := syncDB(path, &out); err == nil {
		t.Errorf("want the unpack's error; out = %q", out.String())
	}
	if got := readFile(t, path); got != "old" {
		t.Errorf("db = %q", got)
	}
}

func TestHolds(t *testing.T) {
	path := dbPath(t)
	writeDBZip(t, path, "abc")
	zr, err := zip.OpenReader(path + ".zip")
	if err != nil {
		t.Fatal(err)
	}
	defer zr.Close()
	entry := zr.File[0]
	for _, c := range []struct {
		content string
		want    bool
	}{{"abc", true}, {"abd", false}, {"ab", false}, {"abcd", false}} {
		writeFile(t, path, c.content)
		if got, err := holds(path, entry); err != nil || got != c.want {
			t.Errorf("holds(%q) = %v, %v; want %v", c.content, got, err, c.want)
		}
	}
	if _, err := holds(path+".missing", entry); !os.IsNotExist(err) {
		t.Errorf("a missing file: err = %v, want not-exist", err)
	}
}

func TestPickDBEntry(t *testing.T) {
	a, b := &zip.File{FileHeader: zip.FileHeader{Name: "README"}}, &zip.File{FileHeader: zip.FileHeader{Name: "dir/t7kb.db"}}
	if got := pickDBEntry([]*zip.File{a, b}, "t7kb.db"); got != b {
		t.Errorf("got %v, want the entry named like the db", got)
	}
	if got := pickDBEntry([]*zip.File{a}, "t7kb.db"); got != a {
		t.Errorf("got %v, want the first entry when none is named like the db", got)
	}
	if got := pickDBEntry(nil, "t7kb.db"); got != nil {
		t.Errorf("got %v, want nil", got)
	}
}

// The unpack-db command resolves the db like every other command (--db here).
func TestUnpackDBCommand(t *testing.T) {
	path := dbPath(t)
	writeDBZip(t, path, "new db")
	old := dbFlag
	t.Cleanup(func() { dbFlag = old })
	root := newRootCmd()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetArgs([]string{"unpack-db", "--db", path})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	if got := readFile(t, path); got != "new db" {
		t.Errorf("db = %q", got)
	}
	if !strings.Contains(out.String(), "now matches") {
		t.Errorf("out = %q", out.String())
	}
}
