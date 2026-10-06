package gdt

import (
	"bytes"
	"encoding/gob"
	"fmt"
	"hash/fnv"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/McReaper/t7_companion/internal/format/xmodelbin"
)

// The materials an xmodel uses live in its LOD files (.xmodel_bin), not in its
// GDT entry, and a full install has ~40k xmodels over ~150k such files. Two
// caches, persisted beside the asset index and checked by mtime and size, keep
// a lookup to stat calls once warm: per GDT, its xmodels' LOD files; per LOD
// file, its materials.

const modelMatsVersion = 3 // 3: BulletCollisionFile only for a Custom BulletCollisionLOD

type modelUse struct {
	Asset string
	Line  int
	Field string
	Path  string // the LOD file, relative to the root
}

type gdtModels struct {
	Mod, Size int64
	Uses      []modelUse
}

type fileMats struct {
	Mod, Size int64
	Mats      []string
	Err       string
}

type modelMats struct {
	mu      sync.Mutex
	loaded  bool
	dirty   bool
	Version int
	GDTs    map[string]gdtModels // GDT rel path -> its xmodels' LOD files
	Files   map[string]fileMats  // LOD file rel path -> its materials
}

func modelMatsPath(root string) string {
	dir, err := os.UserCacheDir()
	if err != nil {
		dir = os.TempDir()
	}
	h := fnv.New64a()
	h.Write([]byte(strings.ToLower(filepath.Clean(root))))
	return filepath.Join(dir, "t7kb", fmt.Sprintf("xmodel-materials-%x.gob", h.Sum64()))
}

// load reads the persisted caches once; a missing or older one starts empty.
func (m *modelMats) load(root string) {
	if m.loaded {
		return
	}
	m.loaded = true
	if b, err := os.ReadFile(modelMatsPath(root)); err == nil {
		var disk modelMats
		if gob.NewDecoder(bytes.NewReader(b)).Decode(&disk) == nil && disk.Version == modelMatsVersion {
			m.GDTs, m.Files = disk.GDTs, disk.Files
		}
	}
	if m.GDTs == nil {
		m.GDTs, m.Files = map[string]gdtModels{}, map[string]fileMats{}
	}
}

// save writes the caches when they changed, atomically.
func (m *modelMats) save(root string) {
	if !m.dirty {
		return
	}
	var b bytes.Buffer
	disk := modelMats{Version: modelMatsVersion, GDTs: m.GDTs, Files: m.Files}
	path := modelMatsPath(root)
	if gob.NewEncoder(&b).Encode(&disk) != nil || os.MkdirAll(filepath.Dir(path), 0o755) != nil {
		return
	}
	tmp := path + ".tmp"
	if os.WriteFile(tmp, b.Bytes(), 0o644) == nil && os.Rename(tmp, path) == nil {
		m.dirty = false
	}
}

// ModelUses returns the xmodels, across the indexed GDTs, one of whose LOD files
// uses material (compared case-insensitively). The Field is "<field> file": the
// model file that GDT field names uses the material.
func (w *Workspace) ModelUses(material string) ([]RefHit, error) {
	files, err := w.indexedFiles(nil)
	if err != nil {
		return nil, err
	}
	m := &w.models
	m.mu.Lock()
	defer m.mu.Unlock()
	m.load(w.Root)
	defer m.save(w.Root)

	uses := w.refreshGDTModels(files)
	w.refreshFileMats(uses)

	var hits []RefHit
	for gdt, us := range uses {
		for _, u := range us {
			for _, mat := range m.Files[u.Path].Mats {
				if strings.EqualFold(mat, material) {
					// "<field> file": the file the field names uses it (gdt_get shows the path)
					hits = append(hits, RefHit{File: gdt, Line: u.Line, Asset: u.Asset, Type: "xmodel", Field: u.Field + " file"})
					break
				}
			}
		}
	}
	sort.Slice(hits, func(i, j int) bool {
		if hits[i].File != hits[j].File {
			return hits[i].File < hits[j].File
		}
		return hits[i].Line < hits[j].Line
	})
	return hits, nil
}

// refreshGDTModels returns, per GDT, its xmodels' LOD files, re-parsing only
// the GDTs whose mtime or size changed. Called with m.mu held.
func (w *Workspace) refreshGDTModels(files []string) map[string][]modelUse {
	m := &w.models
	var mu sync.Mutex
	out := map[string][]modelUse{}
	parallel(files, func(rel string) {
		st, err := os.Stat(w.Abs(rel))
		if err != nil {
			return
		}
		mu.Lock()
		c, ok := m.GDTs[rel]
		mu.Unlock()
		if !ok || c.Mod != st.ModTime().UnixNano() || c.Size != st.Size() {
			c = gdtModels{Mod: st.ModTime().UnixNano(), Size: st.Size(), Uses: w.lodFilesIn(rel)}
			mu.Lock()
			m.GDTs[rel], m.dirty = c, true
			mu.Unlock()
		}
		if len(c.Uses) > 0 {
			mu.Lock()
			out[rel] = c.Uses
			mu.Unlock()
		}
	})
	return out
}

// lodFilesIn lists the LOD files of every xmodel (derived ones included) in a GDT.
func (w *Workspace) lodFilesIn(rel string) []modelUse {
	b, err := os.ReadFile(w.Abs(rel))
	if err != nil || !bytes.Contains(b, []byte("xmodel")) {
		return nil
	}
	f, err := Parse(b)
	if err != nil {
		return nil
	}
	var out []modelUse
	for _, a := range f.Assets {
		typ, fields, err := w.Resolved(f, a)
		if err != nil || typ != "xmodel" {
			continue
		}
		custom := false // the custom bullet mesh is read only when BulletCollisionLOD says so
		for _, fl := range fields {
			custom = custom || (fl.Key == "BulletCollisionLOD" && Unquote(fl.Value) == "Custom")
		}
		for _, fl := range fields {
			base, ok := fileFields["xmodel"][fl.Key]
			if fl.Key == "BulletCollisionFile" && custom { // its materials (bullet_collision_*) are packed too
				base, ok = "model_export", true
			}
			v := Unquote(fl.Value)
			if !ok || v == "" {
				continue
			}
			p := filepath.ToSlash(filepath.Join(base, filepath.FromSlash(strings.ReplaceAll(v, `\`, "/"))))
			out = append(out, modelUse{Asset: a.Name, Line: a.Line, Field: fl.Key, Path: p})
		}
	}
	return out
}

// refreshFileMats decodes the LOD files that are new or changed. Called with
// m.mu held.
func (w *Workspace) refreshFileMats(uses map[string][]modelUse) {
	m := &w.models
	seen := map[string]bool{}
	var paths []string
	for _, us := range uses {
		for _, u := range us {
			if !seen[u.Path] {
				seen[u.Path] = true
				paths = append(paths, u.Path)
			}
		}
	}
	var mu sync.Mutex
	parallel(paths, func(rel string) {
		st, err := os.Stat(w.Abs(rel))
		if err != nil {
			return // a missing LOD file is gdt_check's to report
		}
		mu.Lock()
		c, ok := m.Files[rel]
		mu.Unlock()
		if ok && c.Mod == st.ModTime().UnixNano() && c.Size == st.Size() {
			return
		}
		c = fileMats{Mod: st.ModTime().UnixNano(), Size: st.Size()}
		if b, err := os.ReadFile(w.Abs(rel)); err != nil {
			c.Err = err.Error()
		} else if c.Mats, err = xmodelbin.Materials(b); err != nil {
			c.Err = err.Error()
		}
		mu.Lock()
		m.Files[rel], m.dirty = c, true
		mu.Unlock()
	})
}

// WarmModels fills the xmodel material caches in the background, so the first
// ModelUses of a session doesn't pay for reading every LOD file.
func (w *Workspace) WarmModels() {
	go func() { _, _ = w.ModelUses("") }()
}

// ModelMaterials returns the materials an xmodel's LOD files use, deduplicated
// in first-seen order, for the first GDT definition of name.
func (w *Workspace) ModelMaterials(name string) ([]string, error) {
	locs, err := w.FindTyped(name, "xmodel")
	if err != nil || len(locs) == 0 {
		return nil, err
	}
	m := &w.models
	m.mu.Lock()
	defer m.mu.Unlock()
	m.load(w.Root)
	defer m.save(w.Root)
	var uses []modelUse
	for _, u := range w.refreshGDTModels([]string{locs[0].File})[locs[0].File] { // cached per GDT
		if u.Asset == name {
			uses = append(uses, u)
		}
	}
	w.refreshFileMats(map[string][]modelUse{locs[0].File: uses})
	seen := map[string]bool{}
	var out []string
	for _, u := range uses {
		for _, mat := range m.Files[u.Path].Mats {
			if !seen[strings.ToLower(mat)] {
				seen[strings.ToLower(mat)] = true
				out = append(out, mat)
			}
		}
	}
	return out, nil
}
