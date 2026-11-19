package embed

import (
	"errors"
	"os"
	"sync"
	"sync/atomic"
	"testing"
)

// fakeModel stands in for go-sentex's model.
type fakeModel struct{}

func (fakeModel) Embed(text string) ([]float32, error) { return []float32{float32(len(text))}, nil }
func (fakeModel) Dimensions() int                      { return 384 }

// withLoader replaces loadModel for one test, counting its loads.
func withLoader(t *testing.T, err error) *atomic.Int32 {
	t.Helper()
	var loads atomic.Int32
	old := loadModel
	loadModel = func() (model, error) {
		loads.Add(1)
		if err != nil {
			return nil, err
		}
		return fakeModel{}, nil
	}
	t.Cleanup(func() { loadModel = old })
	t.Setenv("HF_HOME", t.TempDir())
	return &loads
}

// A lazy Embedder loads nothing until it embeds, loads once however many
// embed at the same time, and loads again after Release.
func TestLazyEmbedderLoadsOnFirstUseAndAgainAfterRelease(t *testing.T) {
	loads := withLoader(t, nil)
	e := Lazy()
	if loads.Load() != 0 {
		t.Fatal("Lazy loaded the model")
	}
	if e.Release() {
		t.Error("Release said it dropped a model never loaded")
	}

	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			if v, err := e.Embed("zombie"); err != nil || len(v) != 1 || v[0] != 6 {
				t.Errorf("Embed = %v, %v", v, err)
			}
		})
	}
	wg.Wait()
	if n := loads.Load(); n != 1 {
		t.Fatalf("8 concurrent Embeds loaded the model %d times, want 1", n)
	}
	if e.Dim() != 384 || loads.Load() != 1 {
		t.Error("Dim reloaded the model, or gave the wrong dimension")
	}

	if !e.Release() {
		t.Fatal("Release said nothing was loaded")
	}
	if _, err := e.Embed("again"); err != nil {
		t.Fatal(err)
	}
	if n := loads.Load(); n != 2 {
		t.Errorf("Embed after Release: %d loads, want 2", n)
	}
}

// New loads at once, so a missing model fails the command that asked for it;
// a lazy Embedder fails its Embed instead, and Dim is 0.
func TestNewFailsAtOnceWhenTheModelCantLoad(t *testing.T) {
	broken := errors.New("no model")
	withLoader(t, broken)
	if _, err := New(); !errors.Is(err, broken) {
		t.Fatalf("New: %v, want the load's error", err)
	}
	e := Lazy()
	if _, err := e.Embed("x"); !errors.Is(err, broken) {
		t.Errorf("Embed: %v, want the load's error", err)
	}
	if e.Dim() != 0 {
		t.Error("Dim without a model, want 0")
	}
}

// New with a model that loads hands back a loaded Embedder.
func TestNewLoadsTheModel(t *testing.T) {
	loads := withLoader(t, nil)
	e, err := New()
	if err != nil {
		t.Fatal(err)
	}
	if loads.Load() != 1 || !e.Release() {
		t.Error("New didn't load the model")
	}
}

// Without HF_HOME, the model is looked for beside the binary.
func TestModelHomeDefaultsBesideTheBinary(t *testing.T) {
	withLoader(t, nil)
	t.Setenv("HF_HOME", "")
	if _, err := Lazy().Embed("x"); err != nil {
		t.Fatal(err)
	}
	if os.Getenv("HF_HOME") == "" {
		t.Error("HF_HOME left empty, want the binary's models/ directory")
	}
}
