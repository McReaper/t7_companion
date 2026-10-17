package cli

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/McReaper/t7_companion/internal/radiant/radianttest"
)

func TestDecodeOpsRefusesWhatNoOpHas(t *testing.T) {
	for js, want := range map[string]string{
		`{"box": {}}`:                        "a JSON array of ops",
		`[{"box": {"mn": [0,0,0]}}]`:         `op 0: json: unknown field "mn"`,
		`[{"delete": ["a"]}, {"paint": {}}]`: `op 1: json: unknown field "paint"`,
		`[{"set": {"id": "a", "keys": 1}}]`:  "op 0:",
	} {
		if _, err := decodeOps([]byte(js)); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%s: err = %v, want %q", js, err, want)
		}
	}
	ops, err := decodeOps([]byte(`[{"set": {"id": "a", "keys": {"k": null}}}]`))
	if err != nil || ops[0].Set.Keys["k"] != nil {
		t.Errorf("a null value: %+v, %v", ops, err)
	}
}

func TestMapEditAnswersEachOpsID(t *testing.T) {
	testRadiant(t)
	ops, err := decodeOps([]byte(`[
		{"entity": {"class": "light", "origin": [1, 2, 3]}},
		{"move": {"ids": ["5ec8de49"], "by": [0, 0, 8]}},
		{"delete": ["76780cea"]}
	]`))
	if err != nil {
		t.Fatal(err)
	}
	out, err := mapEdit(context.Background(), ops, "light and floor", 0)
	if err != nil {
		t.Fatal(err)
	}
	if out.Map != "test_claude.map" || out.Undo != "t7kb: light and floor" || out.Deleted != 2 || len(out.Made) != 2 {
		t.Fatalf("answer = %+v", out)
	}
	light, floor := out.Made[0], out.Made[1]
	if light.Op != 0 || light.Class != "light" || len(light.ID) < 8 || light.Was != "" || light.Bounds != nil || light.Origin[2] != 3 {
		t.Errorf("light = %+v", light)
	}
	if floor.Op != 1 || floor.Was != "5ec8de49" || floor.Class != "" || floor.Origin != nil || floor.Bounds[2] != -56 {
		t.Errorf("floor = %+v", floor)
	}
}

func TestMapEditSaysWhatItCouldNotFindBack(t *testing.T) {
	f := testRadiant(t)
	f.Handle("edit", func(*radianttest.Fake, string, []byte) radianttest.Reply { return radianttest.OK("deleted=0", nil) })
	ops, _ := decodeOps([]byte(`[{"box": {"min": [0,0,0], "max": [8,8,8], "material": "caulk"}}]`))
	out, err := mapEdit(context.Background(), ops, "", 0)
	if err != nil || len(out.Made) != 1 || out.Made[0].ID != "" || !strings.Contains(out.Made[0].Note, "not found in the map read back") {
		t.Errorf("answer %+v, %v", out, err)
	}
	onMachine(t)
	if _, err := mapEdit(context.Background(), ops, "", 0); err == nil {
		t.Error("an edit with no Radiant")
	}
}

func TestMapEditToolAndCommand(t *testing.T) {
	testRadiant(t)
	res, err := mapEditTool().Handler(context.Background(), mcpRequest("map_edit", map[string]any{
		"ops": []any{map[string]any{"entity": map[string]any{"class": "info_null", "origin": []any{0, 0, 0}}}}, "label": "a null",
	}))
	if err != nil || res.IsError || !strings.Contains(resultText(res), `"undo":"t7kb: a null"`) {
		t.Errorf("tool: %v %s", err, resultText(res))
	}
	bad, _ := mapEditTool().Handler(context.Background(), mcpRequest("map_edit", map[string]any{"ops": []any{map[string]any{"nope": 1}}}))
	if !bad.IsError || !strings.Contains(resultText(bad), `unknown field "nope"`) {
		t.Errorf("a bad op: %s", resultText(bad))
	}
	run := func(stdin string, args ...string) (string, error) {
		cmd := newMapCmd()
		var out strings.Builder
		cmd.SetOut(&out)
		cmd.SetErr(&out)
		cmd.SetIn(strings.NewReader(stdin))
		cmd.SetArgs(append([]string{"edit"}, args...))
		err := cmd.Execute()
		return out.String(), err
	}
	out, err := run(`[{"delete": ["f2d51a52"]}]`)
	var answer mapEditOut
	if err != nil || json.Unmarshal([]byte(out), &answer) != nil || answer.Deleted != 1 {
		t.Errorf("stdin: %v\n%s", err, out)
	}
	file := filepath.Join(t.TempDir(), "ops.json")
	if err := os.WriteFile(file, []byte(`[{"delete": ["6e73936d"]}]`), 0o644); err != nil {
		t.Fatal(err)
	}
	if out, err := run("", "--ops", file, "--label", "no sun"); err != nil || !strings.Contains(out, `"undo": "t7kb: no sun"`) {
		t.Errorf("file: %v\n%s", err, out)
	}
	if _, err := run("", "--ops", filepath.Join(t.TempDir(), "missing.json")); err == nil {
		t.Error("a missing ops file")
	}
	if _, err := run(`not json`); err == nil {
		t.Error("ops that are no JSON")
	}
}

// prefabReader reads a prefab from the install's map_source once, and says
// why it can't.
func TestPrefabReader(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "map_source", "_prefabs", "test")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	room := filepath.Join(dir, "room.map")
	if err := os.WriteFile(room, []byte("iwmap 4\n// entity 0\n{\n\"classname\" \"worldspawn\"\n}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "open.map"), []byte("{\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	read := prefabReader(root)
	m, err := read("_prefabs/test/room.map")
	if err != nil || len(m.Entities) != 1 {
		t.Fatalf("room.map: %v, %v", m, err)
	}
	if err := os.Remove(room); err != nil {
		t.Fatal(err)
	}
	if again, err := read("_prefabs/test/room.map"); err != nil || again != m {
		t.Errorf("read again: %v, %v: the first read, kept", again, err)
	}
	for model, want := range map[string]string{"_prefabs/test/none.map": "none.map", "_prefabs/test/open.map": "still open"} {
		if _, err := read(model); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%s: %v, want %q", model, err, want)
		}
	}
	if _, err := prefabReader("")("_prefabs/test/room.map"); err == nil || !strings.Contains(err.Error(), "install isn't known") {
		t.Errorf("no install: %v", err)
	}
}
