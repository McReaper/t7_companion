package cli

import (
	"archive/zip"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// ensureDB makes sure the resolved t7kb.db exists. If it's missing but a
// sibling `t7kb.db.zip` is present, it unpacks it once (so the user just drops
// the downloaded zip next to the binary — no manual decompress step). The
// extract is atomic: it writes a .tmp and renames, so a crash mid-unpack never
// leaves a half-written db that looks valid.
func ensureDB(path string) error {
	if _, err := os.Stat(path); err == nil {
		return nil
	}
	zipPath := path + ".zip"
	if _, err := os.Stat(zipPath); err != nil {
		return fmt.Errorf("database not found: %s (and no %s to unpack beside it)",
			path, filepath.Base(zipPath))
	}
	fmt.Fprintf(os.Stderr, "unpacking %s (one-time)…\n", filepath.Base(zipPath))
	if err := unpackDB(zipPath, path); err != nil {
		return fmt.Errorf("unpack %s: %w", filepath.Base(zipPath), err)
	}
	return nil
}

// unpackDB extracts the t7kb.db entry from zipPath to dst (atomic via .tmp).
func unpackDB(zipPath, dst string) error {
	zr, err := zip.OpenReader(zipPath)
	if err != nil {
		return err
	}
	defer zr.Close()

	entry := pickDBEntry(zr.File, filepath.Base(dst))
	if entry == nil {
		return fmt.Errorf("no database entry found in %s", filepath.Base(zipPath))
	}
	rc, err := entry.Open()
	if err != nil {
		return err
	}
	defer rc.Close()

	tmp := dst + ".tmp"
	out, err := os.Create(tmp)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, rc); err != nil {
		out.Close()
		os.Remove(tmp)
		return err
	}
	if err := out.Close(); err != nil {
		os.Remove(tmp)
		return err
	}
	return os.Rename(tmp, dst)
}

// pickDBEntry returns the archive entry matching wantName, else the first file.
func pickDBEntry(files []*zip.File, wantName string) *zip.File {
	for _, f := range files {
		if filepath.Base(f.Name) == wantName {
			return f
		}
	}
	if len(files) > 0 {
		return files[0]
	}
	return nil
}
