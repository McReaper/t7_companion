package cli

import (
	"fmt"
	"os"
	"strings"

	"github.com/McReaper/t7_companion/internal/scaffold"
)

// createMaxFiles caps the file list in the answer: the ZM Advanced template has ~60.
const createMaxFiles = 40

type createResult struct {
	*scaffold.Plan
	Written   bool     `json:"written"`
	More      int      `json:"more_files,omitempty"`
	Templates []string `json:"templates"`
	Next      []string `json:"next_steps"`
}

// createOp plans a new map or mod from the install's templates and, with
// write, creates it. A dry run (the default) lists the files and any that
// already exist; a write refuses when one does.
func createOp(toolsPath, name, template string, zones []string, write bool) (*createResult, error) {
	root := strings.TrimRight(firstNonEmpty(toolsPath, os.Getenv("TA_TOOLS_PATH")), `\/`)
	if root == "" {
		return nil, fmt.Errorf("no mod-tools path: pass tools_path / --tools-path or set TA_TOOLS_PATH")
	}
	p, err := scaffold.NewPlan(root, name, template, zones)
	if err != nil {
		return nil, err
	}
	res := &createResult{Plan: p}
	res.Templates, _ = scaffold.Templates(root)
	if write {
		if err := p.Write(); err != nil {
			return nil, err
		}
		res.Written = true
	}
	res.Next = createNextSteps(p, write)
	if len(p.Files) > createMaxFiles {
		res.More = len(p.Files) - createMaxFiles
		p.Files = p.Files[:createMaxFiles]
	}
	return res, nil
}

func createNextSteps(p *scaffold.Plan, written bool) []string {
	var next []string
	if !written {
		if len(p.Conflicts) > 0 {
			return []string{"some files already exist: pick another name (nothing is ever overwritten)"}
		}
		next = append(next, "nothing written yet: run again with write=true to create these files")
	}
	if p.Kind == "mod" {
		return append(next,
			"list the mod's assets in mods/"+p.Name+"/zone_source/<zone>_mod.zone",
			"build it with t7kb:build name="+p.Name+" mod=true")
	}
	mode := p.Name[:2]
	return append(next,
		"open map_source/"+mode+"/"+p.Name+".map in Radiant to build the level",
		"build it with t7kb:build name="+p.Name+" (compile, light, link), then stages=run to play it")
}
