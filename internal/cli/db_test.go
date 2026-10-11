package cli

import (
	"archive/zip"
	"os"
	"path/filepath"
	"strings"
	"testing"
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

func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func dbPath(t *testing.T) string { return filepath.Join(t.TempDir(), "t7kb.db") }

// A first run, and the first run after an update (the installer deletes the
// old t7kb.db), unpack the zip beside it, through a .tmp.
func TestEnsureDBUnpacksAMissingDB(t *testing.T) {
	path := dbPath(t)
	writeDBZip(t, path, "new db")
	if err := ensureDB(path); err != nil {
		t.Fatal(err)
	}
	if got := readFile(t, path); got != "new db" {
		t.Errorf("db = %q, want the zip's entry", got)
	}
	if _, err := os.Stat(path + ".tmp"); err == nil {
		t.Error("the .tmp the unpack wrote through is left behind")
	}
}

func TestEnsureDBWithoutDBOrZip(t *testing.T) {
	err := ensureDB(dbPath(t))
	if err == nil || !strings.Contains(err.Error(), "no t7kb.db.zip to unpack") {
		t.Errorf("err = %v, want it to name the missing zip", err)
	}
}

// An existing database is used as it is: unpacking 3.8 GB at every start is
// not an option, so an update replaces it by deleting it.
func TestEnsureDBKeepsAnExistingDB(t *testing.T) {
	path := dbPath(t)
	if err := os.WriteFile(path, []byte("old db"), 0o644); err != nil {
		t.Fatal(err)
	}
	writeDBZip(t, path, "new db")
	if err := ensureDB(path); err != nil {
		t.Fatal(err)
	}
	if got := readFile(t, path); got != "old db" {
		t.Errorf("db = %q", got)
	}
}

func TestEnsureDBCorruptZip(t *testing.T) {
	path := dbPath(t)
	if err := os.WriteFile(path+".zip", []byte("not a zip"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := ensureDB(path); err == nil || !strings.Contains(err.Error(), "unpack t7kb.db.zip") {
		t.Errorf("err = %v", err)
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
