package gdt

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"image"
	_ "image/png" // register the PNG decoder for image.DecodeConfig
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
)

var donorCache sync.Map // root + "|" + semantic -> donor

// imageFields builds an image asset's fields from a texture: the semantic comes
// from the spec (or the techset slot it fills), everything else from the stock
// images of the same semantic — the settings Treyarch actually ships for it.
func (w *Workspace) imageFields(spec *ImageSpec) ([]Field, []Issue, string, error) {
	var issues []Issue
	sem := spec.Semantic
	if sem == "" {
		if spec.MaterialType == "" || spec.Field == "" {
			return nil, nil, "", fmt.Errorf("image: give semantic, or material_type + field to derive it from the techset")
		}
		ts, err := w.Techsets.Resolve(spec.MaterialType)
		if err != nil {
			return nil, nil, "", err
		}
		for _, s := range ts.Textures {
			if s.Field == spec.Field {
				sem = s.Semantic
			}
		}
		if sem == "" {
			var fields []string
			for _, s := range ts.Textures {
				fields = append(fields, s.Field)
			}
			return nil, nil, "", fmt.Errorf("image: material type %q has no texture slot %q (it reads: %s)", spec.MaterialType, spec.Field, strings.Join(fields, ", "))
		}
	}
	if spec.Texture == "" {
		return nil, nil, "", fmt.Errorf("image: texture path is required")
	}
	base, n, err := w.imageDonor(sem)
	if err != nil {
		return nil, nil, "", err
	}
	a := &Asset{Fields: append([]Field(nil), base...)}
	a.Set("baseImage", Quote(strings.ReplaceAll(spec.Texture, "/", `\`)))
	a.Set("semantic", Quote(sem))

	p := filepath.Join(w.Root, filepath.FromSlash(strings.ReplaceAll(spec.Texture, `\`, "/")))
	switch wd, ht, err := imageSize(p); {
	case os.IsNotExist(err):
		issues = append(issues, Issue{"error", "baseImage", fmt.Sprintf("texture %s does not exist (path is relative to the mod-tools root)", spec.Texture), ""})
	case err != nil:
		// unknown format: nothing to check
	case !pow2(wd) || !pow2(ht):
		issues = append(issues, Issue{"error", "baseImage", fmt.Sprintf("texture is %dx%d — BO3 needs power-of-two dimensions", wd, ht), ""})
	}
	cm, _ := a.Get("compressionMethod")
	note := fmt.Sprintf("settings = each field's most common value across %d stock %s images (compressionMethod %q, override with set)", n, sem, Unquote(cm))
	return a.Fields, issues, note, nil
}

// imageDonor returns, for each field stock images of that semantic carry, the
// value most of them use — Treyarch's settings for that kind of texture (e.g.
// 10,612 of 10,636 normalMaps are "compressed"), with no path left over from a
// single donor. It also returns how many stock images that was built from.
func (w *Workspace) imageDonor(sem string) ([]Field, int, error) {
	key := w.Root + "|" + sem
	if v, ok := donorCache.Load(key); ok {
		d := v.(donor)
		return d.fields, d.n, nil
	}
	w.refresh()
	if w.scErr != nil {
		return nil, 0, w.scErr
	}
	var stock []string
	w.idx.mu.RLock()
	for rel := range w.idx.files {
		if w.stock[strings.ToLower(rel)] {
			stock = append(stock, rel)
		}
	}
	w.idx.mu.RUnlock()

	needle := []byte(`"semantic" "` + sem + `"`)
	var mu sync.Mutex
	counts := map[string]map[string]int{} // key -> value -> images
	var order []string                    // keys in first-seen order
	n := 0
	ch := make(chan string)
	var wg sync.WaitGroup
	for i := 0; i < runtime.NumCPU(); i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for rel := range ch {
				b, err := os.ReadFile(filepath.Join(w.Root, filepath.FromSlash(rel)))
				if err != nil || !bytes.Contains(b, needle) {
					continue
				}
				f, err := Parse(b)
				if err != nil {
					continue
				}
				mu.Lock()
				for _, a := range f.Assets {
					if v, _ := a.Get("semantic"); a.Type != "image" || v != sem {
						continue
					}
					n++
					for _, fl := range a.Fields {
						c := counts[fl.Key]
						if c == nil {
							c = map[string]int{}
							counts[fl.Key] = c
							order = append(order, fl.Key)
						}
						c[fl.Value]++
					}
				}
				mu.Unlock()
			}
		}()
	}
	for _, rel := range stock {
		ch <- rel
	}
	close(ch)
	wg.Wait()
	if n == 0 {
		if len(stock) == 0 {
			return nil, 0, fmt.Errorf("no stock GDTs known (stock.gdtdef missing or empty) to take %s image settings from", sem)
		}
		return nil, 0, fmt.Errorf("no stock image with semantic %q to take settings from", sem)
	}
	sort.Strings(order) // goroutines saw files in any order
	fields := make([]Field, 0, len(order))
	for _, k := range order {
		best, bestN, total := "", -1, 0
		for v, c := range counts[k] {
			total += c
			if c > bestN || (c == bestN && v < best) {
				best, bestN = v, c
			}
		}
		if total*2 <= n {
			continue // a key only a minority of images carry is one of their quirks, not a setting
		}
		fields = append(fields, Field{Key: k, Value: best})
	}
	donorCache.Store(key, donor{fields, n})
	return fields, n, nil
}

type donor struct {
	fields []Field
	n      int
}

func pow2(n int) bool { return n > 0 && n&(n-1) == 0 }

// imageSize reads a texture's dimensions: PNG through the standard library, TIFF
// from its first IFD (width tag 256, height tag 257).
func imageSize(path string) (int, int, error) {
	fh, err := os.Open(path)
	if err != nil {
		return 0, 0, err
	}
	defer fh.Close()
	head := make([]byte, 8)
	if _, err := fh.ReadAt(head, 0); err != nil {
		return 0, 0, err
	}
	if string(head[:4]) == "\x89PNG" {
		cfg, _, err := image.DecodeConfig(fh)
		return cfg.Width, cfg.Height, err
	}
	var bo binary.ByteOrder
	switch string(head[:2]) {
	case "II":
		bo = binary.LittleEndian
	case "MM":
		bo = binary.BigEndian
	default:
		return 0, 0, fmt.Errorf("unsupported image format")
	}
	ifd := int64(bo.Uint32(head[4:8]))
	cnt := make([]byte, 2)
	if _, err := fh.ReadAt(cnt, ifd); err != nil {
		return 0, 0, err
	}
	n := int(bo.Uint16(cnt))
	ents := make([]byte, 12*n)
	if _, err := fh.ReadAt(ents, ifd+2); err != nil {
		return 0, 0, err
	}
	var wd, ht int
	for i := 0; i < n; i++ {
		e := ents[i*12 : i*12+12]
		tag, typ := bo.Uint16(e[0:2]), bo.Uint16(e[2:4])
		var v int
		if typ == 3 { // SHORT
			v = int(bo.Uint16(e[8:10]))
		} else {
			v = int(bo.Uint32(e[8:12]))
		}
		switch tag {
		case 256:
			wd = v
		case 257:
			ht = v
		}
	}
	if wd == 0 || ht == 0 {
		return 0, 0, fmt.Errorf("no dimensions in TIFF header")
	}
	return wd, ht, nil
}
