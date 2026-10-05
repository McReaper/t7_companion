package store

import (
	"encoding/binary"
	"math"
	"math/rand/v2"
	"testing"
)

// The vector scan scores from the row's bytes: it must give the very score the
// decode-then-dot reference gives, to the bit, or the ranking could shift.
func TestDotBytesMatchesDot(t *testing.T) {
	r := rand.New(rand.NewPCG(1, 2))
	for range 200 {
		q, v := make([]float32, 384), make([]float32, 384)
		for i := range q {
			q[i], v[i] = r.Float32()*2-1, r.Float32()*2-1
		}
		blob := make([]byte, 0, 4*len(v))
		for _, f := range v {
			blob = binary.LittleEndian.AppendUint32(blob, math.Float32bits(f))
		}
		if got, want := dotBytes(q, blob), dot(q, decodeVec(blob)); got != want {
			t.Fatalf("dotBytes = %v, dot = %v", got, want)
		}
	}
}
