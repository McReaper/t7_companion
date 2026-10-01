package embed

import (
	"os"
	"testing"
)

// The embedding model's costs: loading it (once per process — it dominates a
// one-shot `t7kb search`) and embedding one query (every search). Skipped
// unless HF_HOME points at a model cache: in a test binary, "beside the binary"
// is a temp dir, and New would download ~87 MB.
//
//	HF_HOME=<cache> go test ./internal/embed -run '^$' -bench . -benchmem

func needModel(b *testing.B) {
	b.Helper()
	if os.Getenv("HF_HOME") == "" {
		b.Skip("set HF_HOME to a cache holding all-MiniLM-L6-v2")
	}
}

func BenchmarkLoad(b *testing.B) {
	needModel(b)
	b.ReportAllocs()
	for b.Loop() {
		if _, err := New(); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkEmbed(b *testing.B) {
	needModel(b)
	e, err := New()
	if err != nil {
		b.Fatal(err)
	}
	for _, q := range []struct{ name, text string }{
		{"short", "zombie spawner not spawning"},
		{"sentence", "how do I register a clientfield on the server and read its value in a Lua widget"},
	} {
		b.Run(q.name, func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				if _, err := e.Embed(q.text); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
