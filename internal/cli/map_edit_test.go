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
	out, err := mapEdit(context.Background(), ops, revsOf(t, "5ec8de49", "76780cea"), "light and floor", 0)
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
	out, err := mapEdit(context.Background(), ops, nil, "", 0)
	if err != nil || len(out.Made) != 1 || out.Made[0].ID != "" || !strings.Contains(out.Made[0].Note, "not found in the map read back") {
		t.Errorf("answer %+v, %v", out, err)
	}
	onMachine(t)
	if _, err := mapEdit(context.Background(), ops, nil, "", 0); err == nil {
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
	if _, err := run(`[{"delete": ["f2d51a52"]}]`); err == nil || !strings.Contains(err.Error(), "f2d51a52: named without the rev") {
		t.Errorf("no revs: %v", err)
	}
	if _, err := run(`[{"delete": ["f2d51a52"]}]`, "--revs", `["f2d51a52"]`); err == nil || !strings.Contains(err.Error(), "--revs: an object") {
		t.Errorf("revs no object: %v", err)
	}
	revs, _ := json.Marshal(revsOf(t, "f2d51a52"))
	sunRevs, _ := json.Marshal(revsOf(t, "6e73936d"))
	out, err := run(`[{"delete": ["f2d51a52"]}]`, "--revs", string(revs))
	var answer mapEditOut
	if err != nil || json.Unmarshal([]byte(out), &answer) != nil || answer.Deleted != 1 {
		t.Errorf("stdin: %v\n%s", err, out)
	}
	file := filepath.Join(t.TempDir(), "ops.json")
	if err := os.WriteFile(file, []byte(`[{"delete": ["6e73936d"]}]`), 0o644); err != nil {
		t.Fatal(err)
	}
	if out, err := run("", "--ops", file, "--label", "no sun", "--revs", string(sunRevs)); err != nil || !strings.Contains(out, `"undo": "t7kb: no sun"`) {
		t.Errorf("file: %v\n%s", err, out)
	}
	if _, err := run("", "--ops", filepath.Join(t.TempDir(), "missing.json")); err == nil {
		t.Error("a missing ops file")
	}
	if _, err := run(`not json`); err == nil {
		t.Error("ops that are no JSON")
	}
}

// revsOf reads, through map_find, the revs map_edit wants for these ids.
func revsOf(t *testing.T, ids ...string) map[string]string {
	t.Helper()
	out, err := mapFind(context.Background(), mapFindArgs{IDs: ids, Brushes: true})
	if err != nil {
		t.Fatal(err)
	}
	revs := map[string]string{}
	for _, e := range out.Found {
		revs[e.ID] = e.Rev
		for _, b := range e.List {
			revs[b.ID] = b.Rev
		}
	}
	for _, id := range ids {
		if revs[id] == "" {
			t.Fatalf("no rev for %s in %+v", id, out.Found)
		}
	}
	return revs
}
