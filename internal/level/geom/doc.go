// Package geom is the geometry the level tools compute with: boxes, a brush's
// planes and the vertices where they meet, a vertical line's stretch through a
// brush or a patch's triangle, an entity's origin and bounds, and 2D points,
// hulls, outlines cut into convex pieces and the coverage of one hull by
// others. It knows the .map format's numbers (mapfile) and nothing of Radiant,
// a batch or a map's ops: those live in package level, which builds on it.
package geom
