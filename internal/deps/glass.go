package deps

// Breakable glass: a glass entity's brushes break as the glass asset its type
// key names (GDT type glass), so the map pulls in what that glass names — its
// base, cracked and shard materials, its crack and shatter effects. The glass
// itself is packed inside the compiled map (glasses), not on its own.

// glassChildren adds what the glass asset name names.
func (g *Graph) glassChildren(name string, out *idSet) {
	if _, typ, fields, ok := g.resolve(g.w.FindTyped(name, "glass")); ok {
		g.fieldChildren(typ, fields, out)
	}
}
