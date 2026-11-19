// Package embed wraps go-sentex (pure-Go all-MiniLM-L6-v2) so the corpus build
// and the live query share ONE embedding implementation — exact-parity vectors.
package embed

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"

	sentex "github.com/edgetools/go-sentex"
)

// Embedder produces 384-dim, L2-normalized sentence embeddings. Its model
// (~200 MB in memory) loads on first use, and Release drops it for a
// long-lived process gone idle; the next Embed loads it again.
type Embedder struct {
	mu sync.Mutex // guards m: one load at a time
	m  model
}

// model is what an Embedder needs of go-sentex's model.
type model interface {
	Embed(text string) ([]float32, error)
	Dimensions() int
}

// loadModel loads go-sentex's model; a variable so tests stand in for it.
var loadModel = func() (model, error) {
	m, err := sentex.LoadModel()
	if err != nil {
		return nil, err
	}
	return m, nil
}

// New loads the model now, so a missing one fails at once: for a command that
// embeds right away. By default it looks in a `models/` directory beside the
// binary (where the release bundles it, so it loads offline); set HF_HOME to
// override. With neither present, go-sentex downloads ~87 MB on first use.
func New() (*Embedder, error) {
	e := Lazy()
	if _, err := e.loaded(); err != nil {
		return nil, err
	}
	return e, nil
}

// Lazy returns an Embedder that loads the model on its first Embed, for a
// process that may never embed (the MCP server).
func Lazy() *Embedder { return &Embedder{} }

// loaded returns the model, loading it unless it is.
func (e *Embedder) loaded() (model, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.m != nil {
		return e.m, nil
	}
	if os.Getenv("HF_HOME") == "" {
		if exe, err := os.Executable(); err == nil {
			_ = os.Setenv("HF_HOME", filepath.Join(filepath.Dir(exe), "models"))
		}
	}
	m, err := loadModel()
	if err != nil {
		return nil, fmt.Errorf("load embedding model: %w", err)
	}
	e.m = m
	return m, nil
}

// Release drops the model, and says whether it was loaded. An Embed already
// running keeps the model it holds.
func (e *Embedder) Release() bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	had := e.m != nil
	e.m = nil
	return had
}

// Dim is the embedding dimension (384 for all-MiniLM-L6-v2), 0 if the model
// can't load.
func (e *Embedder) Dim() int {
	m, err := e.loaded()
	if err != nil {
		return 0
	}
	return m.Dimensions()
}

// Embed returns the L2-normalized vector for text.
func (e *Embedder) Embed(text string) ([]float32, error) {
	m, err := e.loaded()
	if err != nil {
		return nil, err
	}
	return m.Embed(text)
}
