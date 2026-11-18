package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf16"
)

// utf16Bytes encodes s as UTF-16 code units, little- or big-endian.
func utf16Bytes(s string, littleEndian bool) string {
	var b []byte
	for _, u := range utf16.Encode([]rune(s)) {
		if littleEndian {
			b = append(b, byte(u), byte(u>>8))
		} else {
			b = append(b, byte(u>>8), byte(u))
		}
	}
	return string(b)
}

func TestReadInputTakesWhatWindowsPowerShellWrites(t *testing.T) {
	const body = `[{"a": "é"}]`
	for _, tc := range []struct {
		name, in, want string
	}{
		{"no BOM", body, body},
		{"a UTF-8 BOM", "\xEF\xBB\xBF" + body, body},
		{"UTF-16LE with its BOM", "\xFF\xFE" + utf16Bytes(body, true), body},
		{"UTF-16BE with its BOM", "\xFE\xFF" + utf16Bytes(body, false), body},
		{"a BOM only at the head", body + "\xEF\xBB\xBF", body + "\xEF\xBB\xBF"},
	} {
		got, err := readInput("-", strings.NewReader(tc.in))
		if err != nil || string(got) != tc.want {
			t.Errorf("stdin, %s: got %q, %v; want %q", tc.name, got, err, tc.want)
		}
		file := filepath.Join(t.TempDir(), "in.json")
		if err := os.WriteFile(file, []byte(tc.in), 0o644); err != nil {
			t.Fatal(err)
		}
		got, err = readInput(file, nil)
		if err != nil || string(got) != tc.want {
			t.Errorf("file, %s: got %q, %v; want %q", tc.name, got, err, tc.want)
		}
	}
	if _, err := readInput("-", strings.NewReader("\xFF\xFE[\x00{")); err == nil || !strings.Contains(err.Error(), "odd number of bytes") {
		t.Errorf("a truncated UTF-16 input: %v", err)
	}
	if _, err := readInput(filepath.Join(t.TempDir(), "missing.json"), nil); err == nil {
		t.Error("a missing file must be an error")
	}
}
