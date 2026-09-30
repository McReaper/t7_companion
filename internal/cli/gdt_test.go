package cli

import (
	"slices"
	"strings"
	"testing"
)

func TestEditItemConvertsJSONValues(t *testing.T) {
	var it gdtEditItem
	if err := decodeStrict([]byte(`{"asset":"m","set":{"a":"x","b":true,"c":false,"d":0.5,"e":1e21,"f":null},"image":{}}`), &it); err != nil {
		t.Fatal(err)
	}
	r, err := it.request("source_data/x.gdt")
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{"a": "x", "b": "1", "c": "0", "d": "0.5", "e": "1000000000000000000000"}
	for k, v := range want {
		if r.Set[k] != v {
			t.Errorf("set.%s = %q, want %q", k, r.Set[k], v)
		}
	}
	if _, ok := r.Set["f"]; ok || !slices.Contains(r.Unset, "f") {
		t.Errorf("null must unset, not write \"<nil>\": set=%v unset=%v", r.Set, r.Unset)
	}
	if r.Image != nil {
		t.Error("an empty image {} means no image (as on the CLI)")
	}
	if err := decodeStrict([]byte(`{"asset":"m","set":{"a":[1]}}`), &it); err != nil {
		t.Fatal(err)
	}
	if _, err := it.request("x.gdt"); err == nil {
		t.Error("a list value must be rejected")
	}
	var args gdtEditArgs
	if err := decodeStrict([]byte(`{"file":"x.gdt","asset":"m","copyFrom":"y"}`), &args); err == nil || !strings.Contains(err.Error(), "copyFrom") {
		t.Errorf("a misspelt key must be an error, not ignored: %v", err)
	}
}
