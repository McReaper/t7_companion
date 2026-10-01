package gdt

import (
	"fmt"
	"os"
	"regexp"
	"sort"
	"testing"
)

// TestCalibrate runs gdt_check over every GDT of a real install and tallies the
// issues by kind into a report — how each rule's error/warning level was set:
// what Treyarch's stock GDTs do while still linking is not an error. Diff the
// report before and after changing a rule. Opt-in (several minutes):
//
//	T7KB_CALIBRATE=report.txt TA_TOOLS_PATH=<bo3 root> go test ./internal/gdt -run TestCalibrate -timeout 30m
func TestCalibrate(t *testing.T) {
	out := os.Getenv("T7KB_CALIBRATE")
	root := os.Getenv("TA_TOOLS_PATH")
	if out == "" || root == "" {
		t.Skip()
	}
	w, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	w.scan()
	var files []string
	for f := range w.idx.files {
		files = append(files, f)
	}
	sort.Strings(files)
	quoted := regexp.MustCompile(`"[^"]*"|[-0-9.]+`)
	tally := map[string]int{}
	ex := map[string]string{}
	assets, gdts := 0, 0
	for _, f := range files {
		res, err := w.Check(f, "")
		if err != nil {
			tally["load-error"]++
			continue
		}
		gdts++
		assets += res.Checked
		for _, a := range res.Assets {
			for _, is := range a.Issues {
				k := fmt.Sprintf("%s %s.%s: %s", is.Level, a.Type, is.Field, quoted.ReplaceAllString(is.Msg, "_"))
				if len(k) > 160 {
					k = k[:160]
				}
				tally[k]++
				if ex[k] == "" {
					ex[k] = fmt.Sprintf("%s:%d %s", f, a.Line, is.Msg)
				}
			}
		}
	}
	keys := make([]string, 0, len(tally))
	for k := range tally {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool { // by count, then by kind: a stable report to diff
		if tally[keys[i]] != tally[keys[j]] {
			return tally[keys[i]] > tally[keys[j]]
		}
		return keys[i] < keys[j]
	})
	fh, _ := os.Create(out)
	defer fh.Close()
	errs, warns := 0, 0
	for _, k := range keys {
		if k[0] == 'e' {
			errs += tally[k]
		} else {
			warns += tally[k]
		}
		fmt.Fprintf(fh, "%7d  %s\n         e.g. %.200s\n", tally[k], k, ex[k])
	}
	fmt.Fprintf(fh, "gdts=%d assets=%d errors=%d warnings=%d\n", gdts, assets, errs, warns)
}
