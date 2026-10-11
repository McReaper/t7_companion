package build

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestParseStages(t *testing.T) {
	ok := []struct {
		in   string
		want []string
	}{
		{"compile,light,link", []string{"compile", "light", "link"}},
		{"link", []string{"link"}},
		{" Link , RUN ", []string{"link", "run"}},
		{"compile,compile", []string{"compile"}},
	}
	for _, tc := range ok {
		got, err := parseStages(tc.in)
		if err != nil {
			t.Fatalf("parseStages(%q) unexpected error: %v", tc.in, err)
		}
		if len(got) != len(tc.want) {
			t.Fatalf("parseStages(%q) = %v, want keys %v", tc.in, got, tc.want)
		}
		for _, k := range tc.want {
			if !got[k] {
				t.Fatalf("parseStages(%q) missing %q (got %v)", tc.in, k, got)
			}
		}
	}
	for _, in := range []string{"", "  ", "bogus", "compile,bogus", ",,"} {
		if _, err := parseStages(in); err == nil {
			t.Fatalf("parseStages(%q) expected error", in)
		}
	}
}

func TestNormalizeLight(t *testing.T) {
	for _, in := range []string{"low", "medium", "high", "LOW", "High"} {
		got, err := normalizeLight(in)
		if err != nil {
			t.Fatalf("normalizeLight(%q) error: %v", in, err)
		}
		if got != strings.ToLower(in) {
			t.Fatalf("normalizeLight(%q) = %q, want %q", in, got, strings.ToLower(in))
		}
	}
	for _, in := range []string{"ultra", "", "med"} {
		if _, err := normalizeLight(in); err == nil {
			t.Fatalf("normalizeLight(%q) expected error", in)
		}
	}
}

func TestStripColor(t *testing.T) {
	cases := map[string]string{
		"^1red^7":    "red",
		"no codes":   "no codes",
		"caret^ end": "caret^ end", // ^ not followed by a digit stays
		"^0a^1b^2c":  "abc",
	}
	for in, want := range cases {
		if got := stripColor(in); got != want {
			t.Fatalf("stripColor(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestExtractErrors(t *testing.T) {
	in := strings.Join([]string{
		"just noise",
		"^1ERROR: bad thing^7", // color codes stripped
		"UNRECOVERABLE ERROR: boom",
		"unresolved external symbol foo",
		"ERROR: bad thing", // duplicate of the stripped line above — deduped
		"another normal line",
	}, "\n")
	got := extractErrors(in)
	want := []string{"ERROR: bad thing", "UNRECOVERABLE ERROR: boom", "unresolved external symbol foo"}
	if len(got) != len(want) {
		t.Fatalf("extractErrors = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("extractErrors[%d] = %q, want %q", i, got[i], want[i])
		}
	}

	var lines []string
	for i := 0; i < 12; i++ {
		lines = append(lines, fmt.Sprintf("ERROR: e%d", i))
	}
	if n := len(extractErrors(strings.Join(lines, "\n"))); n != 8 {
		t.Fatalf("extractErrors cap = %d, want 8", n)
	}
}

// A failed tool whose output matches no error pattern still says everything:
// its whole console output, colour codes stripped, then the exit status and
// where to teach t7kb the error.
func TestFailureLines(t *testing.T) {
	exit := errors.New("exit status 3")
	if got := failureLines("noise\nERROR: known\n", exit); !reflect.DeepEqual(got, []string{"ERROR: known"}) {
		t.Errorf("a known error line: %q", got)
	}
	got := failureLines("^3first^7\r\n\n  second\nthird", exit)
	if !reflect.DeepEqual(got, []string{"first", "second", "third", "exit status 3", unknownFailure}) {
		t.Errorf("every line, then the exit status and the hint: %q", got)
	}
	if !strings.Contains(unknownFailure, "t7kb:contribute") {
		t.Errorf("the hint names the skill: %q", unknownFailure)
	}
	if got := failureLines("", exit); !reflect.DeepEqual(got, []string{"exit status 3", unknownFailure}) {
		t.Errorf("no output: %q", got)
	}
}

// fakeTools makes a temp dir that passes the linker preflight (bin/linker_modtools.exe
// exists) without any real mod tools, so validation-path tests never execute a binary.
func fakeTools(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	bin := filepath.Join(root, "bin")
	if err := os.MkdirAll(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bin, "linker_modtools.exe"), []byte("stub"), 0o755); err != nil {
		t.Fatal(err)
	}
	return root
}

// runBuildReport must reject bad input during preflight — before any stage runs — and
// carry a non-nil error with an empty report. These paths need no BO3 install.
func TestRunBuildReport_Validation(t *testing.T) {
	t.Setenv("TA_TOOLS_PATH", "")
	t.Setenv("TA_GAME_PATH", "")
	tools := fakeTools(t)

	cases := []struct {
		name    string
		opts    *Options
		target  string
		wantErr string
	}{
		{"name too short", &Options{ToolsPath: tools, Stages: "link", Light: "medium"}, "z", "too short"},
		{"two letters are a name", &Options{ToolsPath: tools, Stages: "bogus", Light: "medium"}, "zm", "unknown stage"},
		{"no tools path", &Options{Stages: "link", Light: "medium"}, "zm_test", "no mod-tools path"},
		{"tools not found", &Options{ToolsPath: t.TempDir(), Stages: "link", Light: "medium"}, "zm_test", "mod tools not found"},
		{"bad stage", &Options{ToolsPath: tools, Stages: "bogus", Light: "medium"}, "zm_test", "unknown stage"},
		{"bad light", &Options{ToolsPath: tools, Stages: "link", Light: "ultra"}, "zm_test", "light quality"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rep, err := Run(tc.opts, tc.target, io.Discard)
			if err == nil {
				t.Fatalf("expected error, got report %+v", rep)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("error %q does not contain %q", err.Error(), tc.wantErr)
			}
			if len(rep.Stages) != 0 {
				t.Fatalf("expected no stages run on validation failure, got %d", len(rep.Stages))
			}
		})
	}
}
