package mapfile

import "strings"

// KeyValue is one "key" "value" line.
type KeyValue struct{ Key, Value string }

// Layer is one layer line of a map's header: "<name>" flags <flag>….
type Layer struct {
	Name  string
	Flags []string // expanded, active, hidden, prefab, ignore
}

// Version is the header's "iwmap <n>", 0 for entity text, which has none.
func (m *Map) Version() int {
	for _, l := range m.Header {
		var v int
		if n, _ := scanf(trim(l), "iwmap %d", &v); n == 1 {
			return v
		}
	}
	return 0
}

// Keys returns the header's "key" value lines that are not layers
// ("script_startingnumber" 0).
func (m *Map) Keys() []KeyValue {
	var out []KeyValue
	for _, l := range m.Header {
		if k, rest, ok := quoted(trim(l)); ok && !strings.HasPrefix(rest, "flags") {
			out = append(out, KeyValue{k, rest})
		}
	}
	return out
}

// Layers returns the header's layer lines, in order.
func (m *Map) Layers() []Layer {
	var out []Layer
	for _, l := range m.Header {
		k, rest, ok := quoted(trim(l))
		if !ok || (rest != "flags" && !strings.HasPrefix(rest, "flags ")) {
			continue
		}
		out = append(out, Layer{Name: k, Flags: strings.Fields(strings.TrimPrefix(rest, "flags"))})
	}
	return out
}

// Guid is the entity's guid, braces included, "" when it has none.
func (e *Entity) Guid() string { return field(e.lines(), "guid") }

// Layer is the entity's layer, "" when it is on the default one.
func (e *Entity) Layer() string { return field(e.lines(), "layer") }

// Keys returns the entity's "key" "value" lines, in text order.
func (e *Entity) Keys() []KeyValue {
	var out []KeyValue
	for _, l := range e.lines() {
		if kv, ok := keyLine(l); ok {
			out = append(out, kv)
		}
	}
	return out
}

// Key returns the value of the entity's first key named k.
func (e *Entity) Key(k string) (string, bool) {
	for _, kv := range e.Keys() {
		if kv.Key == k {
			return kv.Value, true
		}
	}
	return "", false
}

// Class is the entity's "classname".
func (e *Entity) Class() string {
	v, _ := e.Key("classname")
	return v
}

// Brushes returns the entity's brushes and patches, in text order.
func (e *Entity) Brushes() []*Brush {
	var out []*Brush
	for _, it := range e.Items {
		if it.Brush != nil {
			out = append(out, it.Brush)
		}
	}
	return out
}

// lines returns the entity's own lines, trimmed.
func (e *Entity) lines() []string {
	var out []string
	for _, it := range e.Items {
		if it.Brush == nil {
			out = append(out, trim(it.Line))
		}
	}
	return out
}

// keyLine reads `"key" "value"`. A line that does not open with a quote
// leaves rest empty, which the second read refuses.
func keyLine(l string) (KeyValue, bool) {
	k, rest, _ := quoted(l)
	v, tail, ok := quoted(rest)
	if !ok || tail != "" {
		return KeyValue{}, false
	}
	return KeyValue{k, v}, true
}

// quoted reads a leading "…" from s and returns it and what follows, trimmed.
func quoted(s string) (string, string, bool) {
	if !strings.HasPrefix(s, `"`) {
		return "", "", false
	}
	end := strings.IndexByte(s[1:], '"')
	if end < 0 {
		return "", "", false
	}
	return s[1 : 1+end], strings.TrimSpace(s[2+end:]), true
}

// field returns the value of the first `<name> "value"` or `<name> value` line.
func field(lines []string, name string) string {
	for _, l := range lines {
		rest, ok := strings.CutPrefix(l, name+" ")
		if !ok {
			continue
		}
		rest = strings.TrimSpace(rest)
		if v, _, ok := quoted(rest); ok {
			return v
		}
		return rest
	}
	return ""
}
