package cli

import "testing"

// The in-memory vector index is on by default and off for 0/false/off/no.
func TestVectorIndexWanted(t *testing.T) {
	for v, want := range map[string]bool{"": true, "1": true, "yes": true, "0": false, "false": false, " OFF ": false, "no": false} {
		t.Setenv("T7KB_VECTOR_INDEX", v)
		if got := vectorIndexWanted(); got != want {
			t.Errorf("T7KB_VECTOR_INDEX=%q: %v, want %v", v, got, want)
		}
	}
}
