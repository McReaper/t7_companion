package cli

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// The README's detailed diagram lists every modding skill, every MCP tool
// with what it acts on, and every mod tool t7kb runs. Each list is checked
// against the code that defines it, both ways, so the diagram can't fall behind
// a new skill, tool or executable, or keep one that's gone; a failure says what
// to add or remove, and in which box.

const readmePath = "../../README.md"

// The detailed diagram's boxes, by subgraph id.
const (
	skillsBox   = "SK"  // a node per domain skill, its id the skill's directory ("-" as "_")
	serverBox   = "SRV" // nodes whose labels name the MCP tools, each linked to what it acts on
	modToolsBox = "MTB" // a node per executable t7kb runs, its id the file's name without .exe
)

func TestREADMEDiagramMatchesTheCode(t *testing.T) {
	d := detailedDiagram(t)
	for _, problem := range d.problems() {
		t.Error(problem)
	}
	checkBox(t, d, skillsBox, "a domain skill (t7kb:knowledge-base routes to every one)", domainSkills(t),
		"add `%s[\"…\"]`, labelled with the skill's craft")
	checkBox(t, d, modToolsBox, "an executable t7kb runs", executables(t),
		"add `%s[\"…\"]`, labelled with the mod tool's name")
	checkServer(t, d)
}

// checkBox compares a box's node ids with want (id -> what in the code it stands for).
func checkBox(t *testing.T, d *diagram, box, what string, want map[string]string, hint string) {
	t.Helper()
	have := map[string]bool{}
	for _, id := range d.children[box] {
		have[id] = true
	}
	for _, id := range sortedKeys(want) {
		if !have[id] {
			t.Errorf("%s: no node %q for %s: %s", d.where(box), id, want[id], fmt.Sprintf(hint, id))
		}
	}
	for _, id := range sortedKeys(have) {
		if _, ok := want[id]; !ok {
			t.Errorf("%s: node %q is not %s: rename it or remove it", d.where(box), id, what)
		}
	}
}

// checkServer: the server box's labels name exactly the registered MCP tools,
// and each of its nodes links to what it acts on.
func checkServer(t *testing.T, d *diagram) {
	t.Helper()
	tools := map[string]string{}
	for _, ts := range toolsets(nil, nil) {
		for _, tool := range ts.tools {
			tools[tool.Tool.Name] = ts.name
		}
	}
	named := map[string]string{} // tool name -> node
	for _, id := range d.children[serverBox] {
		for _, name := range regexp.MustCompile(`<br/>|[,\s]+`).Split(d.label[id], -1) {
			if name != "" {
				named[name] = id
			}
		}
		if len(d.out[id]) == 0 {
			t.Errorf("%s: node %q (%q) has no arrow to what it acts on", d.where(serverBox), id, d.label[id])
		}
	}
	for _, name := range sortedKeys(tools) {
		if _, ok := named[name]; !ok {
			t.Errorf("%s: MCP tool %q (toolset %q) is named in no node: add it to the label of the node for its toolset, "+
				"or a new node linked to what the tool acts on", d.where(serverBox), name, tools[name])
		}
	}
	for _, name := range sortedKeys(named) {
		if _, ok := tools[name]; !ok {
			t.Errorf("%s: node %q names %q, which is no MCP tool (labels here are tool names only): remove it",
				d.where(serverBox), named[name], name)
		}
	}
}

// domainSkills are the skills t7kb:knowledge-base's description routes to:
// check_skills.py makes it name every domain skill.
func domainSkills(t *testing.T) map[string]string {
	t.Helper()
	b, err := os.ReadFile("../../plugin/skills/knowledge-base/SKILL.md")
	if err != nil {
		t.Fatal(err)
	}
	front := strings.SplitN(string(b), "\n---", 2)[0]
	out := map[string]string{}
	for _, m := range regexp.MustCompile(`t7kb:([a-z0-9-]+)`).FindAllStringSubmatch(front, -1) {
		out[strings.ReplaceAll(m[1], "-", "_")] = "the skill plugin/skills/" + m[1]
	}
	if len(out) == 0 {
		t.Fatal("t7kb:knowledge-base's description routes to no skill")
	}
	return out
}

// executables are the mod tools the Go code runs: every "<name>.exe" string
// literal outside tests.
func executables(t *testing.T) map[string]string {
	t.Helper()
	exe := regexp.MustCompile(`"([A-Za-z0-9_]+)\.exe"`)
	out := map[string]string{}
	for _, dir := range []string{"../../internal", "../../cmd"} {
		err := filepath.WalkDir(dir, func(p string, e fs.DirEntry, err error) error {
			if err != nil || e.IsDir() || !strings.HasSuffix(p, ".go") || strings.HasSuffix(p, "_test.go") {
				return err
			}
			b, err := os.ReadFile(p)
			if err != nil {
				return err
			}
			for _, m := range exe.FindAllStringSubmatch(string(b), -1) {
				if _, seen := out[m[1]]; !seen {
					out[m[1]] = m[1] + ".exe, run in " + strings.TrimPrefix(filepath.ToSlash(p), "../../")
				}
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	return out
}

// diagram is a mermaid flowchart, as far as these checks read it: each node's
// label and innermost box, and the arrows.
type diagram struct {
	label    map[string]string   // node or box id -> label
	children map[string][]string // box id -> the nodes labelled in it, in order
	boxes    map[string]bool
	used     map[string]bool     // every id an arrow or a ~~~ chain names
	out      map[string][]string // id -> the ids its arrows go to
	bad      []string            // lines not understood
}

// The node forms the README uses: id, id["…"], id[("…")], id(["…"]).
const nodeRE = `([A-Za-z0-9_]+)(?:\["([^"]*)"\]|\[\("([^"]*)"\)\]|\(\["([^"]*)"\]\))?`

var (
	nodeLine = regexp.MustCompile(`^` + nodeRE + `$`)
	// The arrow kinds the README uses: solid (-->), dotted (-.->), thick (==>), any of them both ways.
	arrowLine = regexp.MustCompile(`^` + nodeRE + `\s*(?:<?-->|<?-\.->|<?==>)\s*(?:\|"[^"]*"\|\s*)?` + nodeRE + `$`)
	boxLine   = regexp.MustCompile(`^subgraph ([A-Za-z0-9_]+)\["([^"]*)"\]$`)
)

// detailedDiagram is the README's mermaid block that holds the server box.
func detailedDiagram(t *testing.T) *diagram {
	t.Helper()
	b, err := os.ReadFile(readmePath)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range quoteBreaks(string(b)) {
		t.Error(p)
	}
	// The diagram sits in a GitHub alert, a blockquote: read it without the "> " markers.
	src := regexp.MustCompile(`(?m)^> ?`).ReplaceAllString(string(b), "")
	for _, m := range regexp.MustCompile("(?s)```mermaid\n(.*?)```").FindAllStringSubmatch(src, -1) {
		if strings.Contains(m[1], "subgraph "+serverBox+"[") {
			return parseDiagram(m[1])
		}
	}
	t.Fatalf("README.md has no mermaid diagram with a %q box", serverBox)
	return nil
}

// quoteBreaks lists the lines of a quoted mermaid block (its opening fence in
// a blockquote, as the diagram is in its alert) that lack the ">" marker: such
// a line ends the blockquote and GitHub draws no diagram, though the block,
// read without its markers, still parses.
func quoteBreaks(src string) []string {
	var out []string
	quoted := false
	for i, line := range strings.Split(src, "\n") {
		bare := strings.TrimSpace(strings.TrimPrefix(line, ">"))
		if !quoted {
			quoted = strings.HasPrefix(line, ">") && bare == "```mermaid"
			continue
		}
		if !strings.HasPrefix(line, ">") {
			out = append(out, fmt.Sprintf("README.md:%d: %q, inside the quoted diagram, has no \">\": it ends the blockquote, "+
				"and GitHub stops drawing the diagram", i+1, line))
		}
		quoted = bare != "```"
	}
	return out
}

func parseDiagram(src string) *diagram {
	d := &diagram{label: map[string]string{}, children: map[string][]string{},
		boxes: map[string]bool{}, used: map[string]bool{}, out: map[string][]string{}}
	var stack []string
	inFront := false
	for _, raw := range strings.Split(src, "\n") {
		line := strings.TrimSpace(raw)
		switch {
		case line == "---":
			inFront = !inFront
		case inFront, line == "", strings.HasPrefix(line, "flowchart "), strings.HasPrefix(line, "direction "):
		case line == "end" && len(stack) > 0:
			stack = stack[:len(stack)-1]
		case boxLine.MatchString(line):
			m := boxLine.FindStringSubmatch(line)
			d.boxes[m[1]], d.label[m[1]] = true, m[2]
			stack = append(stack, m[1])
		case strings.Contains(line, "~~~"): // invisible links only order the nodes
			for _, id := range strings.Split(line, "~~~") {
				d.used[strings.TrimSpace(id)] = true
			}
		case arrowLine.MatchString(line):
			m := arrowLine.FindStringSubmatch(line)
			from, to := m[1], m[5]
			d.define(from, firstNonEmptyOf(m[2:5]...), stack)
			d.define(to, firstNonEmptyOf(m[6:9]...), stack)
			d.used[from], d.used[to] = true, true
			d.out[from] = append(d.out[from], to)
		case nodeLine.MatchString(line):
			m := nodeLine.FindStringSubmatch(line)
			d.define(m[1], firstNonEmptyOf(m[2:5]...), stack)
		default:
			d.bad = append(d.bad, line)
		}
	}
	return d
}

// define records a node where it is first labelled: in the innermost box open
// there. A node only named (in an arrow or a ~~~ chain) is no node of its box.
func (d *diagram) define(id, label string, stack []string) {
	if label == "" || d.boxes[id] || d.label[id] != "" {
		return
	}
	d.label[id] = label
	box := ""
	if len(stack) > 0 {
		box = stack[len(stack)-1]
	}
	d.children[box] = append(d.children[box], id)
}

// problems are what makes the diagram itself unreliable: a line the parser
// doesn't read, or an id linked but never labelled.
func (d *diagram) problems() []string {
	var out []string
	for _, l := range d.bad {
		out = append(out, fmt.Sprintf("README.md's detailed diagram: line %q is in a form this test doesn't read", l))
	}
	for _, id := range sortedKeys(d.used) {
		if !d.boxes[id] && d.label[id] == "" {
			out = append(out, fmt.Sprintf("README.md's detailed diagram: %q is linked (an arrow or a ~~~ chain) but never labelled: "+
				"a typo, or a node removed from its box and not from its links", id))
		}
	}
	return out
}

func (d *diagram) where(box string) string {
	return fmt.Sprintf("README.md's detailed diagram, box %q (subgraph %s)", d.label[box], box)
}

func firstNonEmptyOf(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

func sortedKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// Inside a quoted diagram, each line that drops its ">" is reported, the
// closing fence included, and nothing after the block; a diagram outside a
// blockquote has no markers to drop.
func TestQuoteBreaksFindTheLinesOutOfTheBlockquote(t *testing.T) {
	for src, want := range map[string][]int{
		"> [!NOTE]\n> ```mermaid\n> flowchart TB\n    a[\"A\"]\n> b\n> ```\nafter\n": {4},
		"> ```mermaid\n> a\n```\nafter\n":                                            {3},
		"```mermaid\nflowchart TB\n  a\n```\n":                                       nil,
		"> ```mermaid\n> a\n> ```\n```mermaid\nb\n```\n":                             nil,
	} {
		got := quoteBreaks(src)
		var lines []int
		for _, p := range got {
			var n int
			if _, err := fmt.Sscanf(p, "README.md:%d:", &n); err != nil {
				t.Fatalf("%q names no line: %v", p, err)
			}
			lines = append(lines, n)
		}
		if fmt.Sprint(lines) != fmt.Sprint(want) {
			t.Errorf("%q: lines %v, want %v (%q)", src, lines, want, got)
		}
	}
}
