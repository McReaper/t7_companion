package cli

import (
	"archive/zip"
	"fmt"
	"hash/crc32"
	"io"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
)

// The database ships as t7kb.db.zip beside the binary. The installer runs
// `t7kb unpack-db` after each download, which brings t7kb.db in line with the
// zip. A running t7kb server keeps t7kb.db open, and Windows can't replace an
// open file, so an update made while an agent session runs leaves the new
// database staged as t7kb.db.new: the next t7kb to open the database swaps it in.

// rename is os.Rename; tests make it fail as Windows does on a file in use.
var rename = os.Rename

func stagedDB(path string) string { return path + ".new" }

// ensureDB makes sure the resolved t7kb.db exists before it is opened: it swaps
// in a staged t7kb.db.new, else unpacks a sibling t7kb.db.zip when t7kb.db is
// missing (a manual install that only dropped the zip). Every unpack is atomic:
// it writes a .tmp and renames, so a crash mid-unpack never leaves a
// half-written db that looks valid.
func ensureDB(path string, log io.Writer) error {
	swapStaged(path, log)
	if _, err := os.Stat(path); err == nil {
		return nil
	}
	zipPath := path + ".zip"
	if _, err := os.Stat(zipPath); err != nil {
		return fmt.Errorf("database not found: %s (and no %s to unpack beside it)",
			path, filepath.Base(zipPath))
	}
	fmt.Fprintf(log, "unpacking %s (one-time)…\n", filepath.Base(zipPath))
	if err := unpackDB(zipPath, path); err != nil {
		return fmt.Errorf("unpack %s: %w", filepath.Base(zipPath), err)
	}
	return nil
}

// swapStaged moves a staged t7kb.db.new over t7kb.db. When another t7kb still
// has the database open it says so on log and leaves both in place: the old
// database keeps serving until a later start finds it free.
func swapStaged(path string, log io.Writer) {
	staged := stagedDB(path)
	if _, err := os.Stat(staged); err != nil {
		return
	}
	if err := rename(staged, path); err != nil {
		fmt.Fprintf(log, "t7kb: a newer database waits in %s, but %s is still in use (%v); "+
			"serving the old one until t7kb starts while no other t7kb runs\n",
			filepath.Base(staged), filepath.Base(path), err)
	}
}

func newUnpackDBCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "unpack-db",
		Short: "Bring t7kb.db in line with the t7kb.db.zip beside it (the installer runs it)",
		Long: "Compares t7kb.db with the database in t7kb.db.zip and unpacks the zip when they\n" +
			"differ. If a running t7kb holds t7kb.db open (Windows can't replace it then), the\n" +
			"new database waits as t7kb.db.new and the next t7kb to start swaps it in.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return syncDB(resolveDB(), cmd.OutOrStdout())
		},
	}
}

// syncDB makes path hold the database its sibling zip carries: nothing when it
// already does, else an unpack to the staged path, swapped in unless path is in
// use.
func syncDB(path string, out io.Writer) error {
	zipPath := path + ".zip"
	zr, err := zip.OpenReader(zipPath)
	if err != nil {
		return err
	}
	defer zr.Close()
	entry := pickDBEntry(zr.File, filepath.Base(path))
	if entry == nil {
		return fmt.Errorf("no database entry found in %s", filepath.Base(zipPath))
	}

	same, err := holds(path, entry)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	if same {
		os.Remove(stagedDB(path)) // an older staged copy must not replace the current one
		fmt.Fprintf(out, "%s already matches %s.\n", filepath.Base(path), filepath.Base(zipPath))
		return nil
	}
	return stageAndSwap(entry, path, out)
}

// stageAndSwap unpacks entry to the staged path, unless a staged copy of it is
// already there, then moves it over path.
func stageAndSwap(entry *zip.File, path string, out io.Writer) error {
	name, staged := filepath.Base(path), stagedDB(path)
	ready, err := holds(staged, entry)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	if !ready {
		fmt.Fprintf(out, "unpacking %s.zip (%s)…\n", name, gib(entry.UncompressedSize64))
		if err := extract(entry, staged); err != nil {
			return err
		}
	}
	if err := rename(staged, path); err != nil {
		fmt.Fprintf(out, "The new database is unpacked as %s, but %s is in use (%v).\n"+
			"Each open agent session runs a t7kb server that keeps it open: close every session,\n"+
			"and the next t7kb to start swaps the new database in. Until then the old one serves.\n",
			filepath.Base(staged), name, err)
		return nil
	}
	fmt.Fprintf(out, "%s now matches %s.zip.\n", name, name)
	return nil
}

// holds reports whether the file at path is entry's content: same size, then
// same CRC-32, which the zip records for every entry.
func holds(path string, entry *zip.File) (bool, error) {
	f, err := os.Open(path)
	if err != nil {
		return false, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return false, err
	}
	if uint64(info.Size()) != entry.UncompressedSize64 {
		return false, nil
	}
	h := crc32.NewIEEE()
	if _, err := io.Copy(h, f); err != nil {
		return false, err
	}
	return h.Sum32() == entry.CRC32, nil
}

func gib(n uint64) string { return fmt.Sprintf("%.1f GB", float64(n)/1e9) }

// unpackDB extracts the t7kb.db entry from zipPath to dst.
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
	return extract(entry, dst)
}

// extract writes entry to dst through dst+".tmp", so dst only ever appears
// whole; the zip reader checks the entry's CRC-32 as it reaches the end.
func extract(entry *zip.File, dst string) error {
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
