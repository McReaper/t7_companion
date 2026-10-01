package cli

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

// runGDT runs `t7kb gdt <args>` against root and returns stdout and the error.
func runGDT(t *testing.T, root, stdin string, args ...string) (map[string]any, error) {
	t.Helper()
	cmd := newGDTCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetIn(strings.NewReader(stdin))
	cmd.SetArgs(append(args, "--tools-path", root))
	if err := cmd.Execute(); err != nil {
		return nil, err
	}
	var m map[string]any
	if err := json.Unmarshal(out.Bytes(), &m); err != nil {
		t.Fatalf("gdt %v: output is not JSON: %v\n%s", args, err, out.String())
	}
	return m, nil
}

func TestGDTCommands(t *testing.T) {
	root := fakeToolsRoot(t)
	t.Setenv("TA_TOOLS_PATH", "")

	t.Run("read-only subcommands answer in JSON", func(t *testing.T) {
		for _, args := range [][]string{
			{"find", "clean"},
			{"get", "clean", "--all", "--filter", "gloss"},
			{"schema", "material", "--filter", "gloss"},
			{"schema"},
			{"check", "source_data/test.gdt", "--asset", "clean"},
			{"refs", "shared_img"},
		} {
			m, err := runGDT(t, root, "", args...)
			if err != nil || len(m) == 0 {
				t.Errorf("gdt %v: %v %v", args, m, err)
			}
		}
		m, _ := runGDT(t, root, "", "get", "clean", "--all", "--filter", "gloss")
		if own := m["own_fields"].(map[string]any); len(own) != 1 || own["gloss"] != "1" {
			t.Errorf("--all --filter: %v", own)
		}
	})

	t.Run("edit takes --set, --unset and stays a dry run without --write", func(t *testing.T) {
		m, err := runGDT(t, root, "", "edit", "--file", "source_data/test.gdt", "--asset", "clean", "--set", "gloss=1.5", "--set", "noCastShadow=1", "--unset", "colorMap")
		if err != nil {
			t.Fatal(err)
		}
		if m["written"] != false || len(m["changes"].([]any)) != 3 {
			t.Fatalf("%v", m)
		}
	})

	t.Run("edit writes with --write", func(t *testing.T) {
		m, err := runGDT(t, root, "", "edit", "--file", "source_data/new.gdt", "--asset", "fresh", "--type", "material", "--set", "materialType=lit", "--write")
		if err != nil || m["written"] != true {
			t.Fatalf("%v %v", m, err)
		}
		if g, err := runGDT(t, root, "", "get", "fresh"); err != nil || g["type"] != "material" {
			t.Fatalf("the written asset reads back: %v %v", g, err)
		}
	})

	t.Run("edit --batch from stdin", func(t *testing.T) {
		batch := `[{"asset":"b1","type":"material","set":{"materialType":"lit"}},{"asset":"b2","parent":"b1","set":{"gloss":"2"}}]`
		m, err := runGDT(t, root, batch, "edit", "--file", "source_data/batch.gdt", "--batch", "-")
		if err != nil || len(m["results"].([]any)) != 2 || m["written"] != false {
			t.Fatalf("%v %v", m, err)
		}
	})

	t.Run("edit --image-* creates an image", func(t *testing.T) {
		_, err := runGDT(t, root, "", "edit", "--file", "source_data/img.gdt", "--asset", "i1", "--image-texture", "texture_assets/nope.tif", "--image-semantic", "diffuseMap")
		// the flags reach the image path, which needs stock images to take settings from (the fixture has none)
		if err == nil || !strings.Contains(err.Error(), "to take diffuseMap image settings from") {
			t.Fatalf("%v", err)
		}
	})

	t.Run("edit argument errors", func(t *testing.T) {
		for _, tc := range []struct {
			args []string
			want string
		}{
			{[]string{"edit", "--file", "source_data/test.gdt", "--asset", "clean", "--set", "novalue"}, "key=value"},
			{[]string{"edit", "--file", "source_data/test.gdt"}, "--asset is required"},
			{[]string{"edit", "--asset", "clean"}, "file"},
		} {
			if _, err := runGDT(t, root, "", tc.args...); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("gdt %v: want an error containing %q, got %v", tc.args, tc.want, err)
			}
		}
		if _, err := runGDT(t, root, "{not json", "edit", "--file", "source_data/test.gdt", "--batch", "-"); err == nil || !strings.Contains(err.Error(), "--batch") {
			t.Errorf("a bad batch: %v", err)
		}
	})
}
