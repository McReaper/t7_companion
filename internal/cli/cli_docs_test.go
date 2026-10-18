package cli

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

// docs/cli.md and the pages it links, docs/cli/*.md, give every command a
// section headed by the command itself (## `t7kb gdt edit`). The pages are
// checked against the cobra tree both ways, so they can't keep a command or an
// option that's gone, or miss a new command; a failure names the page and what
// to add or remove.

const (
	cliIndex = "../../docs/cli.md"
	cliPages = "../../docs/cli"
)

var (
	sectionHeading = regexp.MustCompile(`^## (.+)$`)
	longFlag       = regexp.MustCompile(`(?:^|[^\w-])--([a-z][a-z0-9-]*)`)
	pageLink       = regexp.MustCompile(`\]\(cli/([a-z0-9-]+\.md)\)`)
)

// A docSection is a page's text under one ## heading (or before the first);
// path is the command its heading names, nil for any other heading.
type docSection struct {
	page, heading, text string
	path                []string
}

func TestCLIDocsMatchTheCommands(t *testing.T) {
	root := newRootCmd()
	addDefaultFlags(root)
	pages := cliDocPages(t)
	documented := map[string]string{} // command path -> page
	for _, page := range sortedKeys(pages) {
		for _, s := range docSections(page, pages[page]) {
			checkDocSection(t, root, s, documented)
		}
	}
	for _, c := range documentedCommands(root) {
		if _, ok := documented[c.CommandPath()]; !ok {
			t.Errorf("`%s` has no section in docs/cli.md or docs/cli/*.md: add \"## `%s`\" to its family's page, "+
				"with what it's for, an example and its useful options", c.CommandPath(), strings.TrimSuffix(c.UseLine(), " [flags]"))
		}
	}
	checkCLIIndexLinks(t, pages[cliIndex])
}

// checkDocSection: a command section names a command that runs, once across
// the pages, and only its own options; any other section names options some
// command has.
func checkDocSection(t *testing.T, root *cobra.Command, s docSection, documented map[string]string) {
	t.Helper()
	var cmd *cobra.Command
	if s.path != nil {
		found, rest, err := root.Find(s.path[1:])
		if err != nil || len(rest) != 0 || found.CommandPath() != strings.Join(s.path, " ") || !found.Runnable() {
			t.Errorf("%s: section %q names `%s`, which is no command: rename or remove the section", s.page, s.heading, strings.Join(s.path, " "))
			return
		}
		cmd = found
		if prev, dup := documented[cmd.CommandPath()]; dup {
			t.Errorf("%s: `%s` has a section in %s already: keep one", s.page, cmd.CommandPath(), prev)
		}
		documented[cmd.CommandPath()] = s.page
	}
	for _, m := range longFlag.FindAllStringSubmatch(s.text, -1) {
		name := m[1]
		switch {
		case cmd != nil && cmd.Flag(name) == nil:
			t.Errorf("%s: section %q names --%s, which `%s` doesn't have: fix or remove it", s.page, s.heading, name, cmd.CommandPath())
		case cmd == nil && !anyCommandHasFlag(root, name):
			t.Errorf("%s: section %q names --%s, which no command has: fix or remove it", s.page, s.heading, name)
		}
	}
}

// checkCLIIndexLinks: docs/cli.md links every page of docs/cli/, and only
// pages that exist.
func checkCLIIndexLinks(t *testing.T, index string) {
	t.Helper()
	linked := map[string]bool{}
	for _, m := range pageLink.FindAllStringSubmatch(index, -1) {
		linked[m[1]] = true
		if _, err := os.Stat(filepath.Join(cliPages, m[1])); err != nil {
			t.Errorf("docs/cli.md links cli/%s, which doesn't exist", m[1])
		}
	}
	entries, err := os.ReadDir(cliPages)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".md") && !linked[e.Name()] {
			t.Errorf("docs/cli.md doesn't link cli/%s: add it to the table of pages", e.Name())
		}
	}
}

// cliDocPages reads docs/cli.md and docs/cli/*.md, by path.
func cliDocPages(t *testing.T) map[string]string {
	t.Helper()
	paths, err := filepath.Glob(filepath.Join(cliPages, "*.md"))
	if err != nil {
		t.Fatal(err)
	}
	pages := map[string]string{}
	for _, p := range append(paths, cliIndex) {
		b, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		pages[filepath.ToSlash(p)] = strings.ReplaceAll(string(b), "\r\n", "\n")
	}
	return pages
}

// docSections splits a page at its ## headings. A heading that starts with
// t7kb names a command: its words up to the first argument or option.
func docSections(page, text string) []docSection {
	cur := docSection{page: page, heading: "(top)"}
	var out []docSection
	for _, line := range strings.Split(text, "\n") {
		m := sectionHeading.FindStringSubmatch(line)
		if m == nil {
			cur.text += line + "\n"
			continue
		}
		out = append(out, cur)
		cur = docSection{page: page, heading: m[1]}
		words := strings.Fields(strings.ReplaceAll(m[1], "`", ""))
		if len(words) > 0 && words[0] == "t7kb" {
			for _, w := range words {
				if strings.ContainsAny(w[:1], "<[-") {
					break
				}
				cur.path = append(cur.path, w)
			}
		}
	}
	return append(out, cur)
}

// documentedCommands are the commands a user runs: every runnable one but the
// hidden ones.
func documentedCommands(c *cobra.Command) []*cobra.Command {
	var out []*cobra.Command
	if c.Runnable() && !c.Hidden {
		out = append(out, c)
	}
	for _, sub := range c.Commands() {
		out = append(out, documentedCommands(sub)...)
	}
	return out
}

// addDefaultFlags gives every command the --help (and the root --version)
// cobra adds only when it runs one.
func addDefaultFlags(c *cobra.Command) {
	c.InitDefaultHelpFlag()
	c.InitDefaultVersionFlag()
	for _, sub := range c.Commands() {
		addDefaultFlags(sub)
	}
}

func anyCommandHasFlag(c *cobra.Command, name string) bool {
	if c.Flag(name) != nil {
		return true
	}
	for _, sub := range c.Commands() {
		if anyCommandHasFlag(sub, name) {
			return true
		}
	}
	return false
}
