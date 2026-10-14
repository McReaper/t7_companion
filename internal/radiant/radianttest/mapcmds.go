package radianttest

import (
	"strings"

	"github.com/McReaper/t7_companion/internal/format/mapfile"
)

// parsed is the fake's map text, read; Text that doesn't parse is a test's mistake.
func (f *Fake) parsed() *mapfile.Map {
	m, err := mapfile.Parse([]byte(f.Text))
	if err != nil {
		panic("radianttest: the fake's Text: " + err.Error())
	}
	return m
}

// entities gives every entity's text, or the named ones' and which are missing.
func entities(f *Fake, args string, _ []byte) Reply {
	want := strings.Fields(args)
	if len(want) == 0 {
		return OK("", []byte(f.Text))
	}
	byGUID := map[string]*mapfile.Entity{}
	for _, e := range f.parsed().Entities {
		byGUID[strings.ToUpper(e.Guid())] = e
	}
	var out []byte
	var missing []string
	for _, g := range want {
		if e := byGUID[strings.ToUpper(g)]; e != nil {
			out = append(out, e.Bytes()...)
		} else {
			missing = append(missing, g)
		}
	}
	summary := ""
	if len(missing) > 0 {
		summary = "missing=" + strings.Join(missing, ",")
	}
	return OK(summary, out)
}

// selection gives one line per selected brush, with its entity; the bounds
// are left at zero (the plugin gives Radiant's).
func selection(f *Fake, _ string, _ []byte) Reply {
	owner := map[string]string{}
	for _, e := range f.parsed().Entities {
		for _, b := range e.Brushes() {
			owner[strings.ToUpper(b.Guid())] = e.Guid()
		}
	}
	var out strings.Builder
	for _, g := range f.Selection {
		if e, ok := owner[strings.ToUpper(g)]; ok {
			out.WriteString(g + " " + e + " 0 0 0 0 0 0\n")
		}
	}
	return OK("", []byte(out.String()))
}
