Detail for **t7kb:mapping** (`plugin/skills/mapping/SKILL.md`) — materials and views: texturing face by face when the mapper asks for it, and looking at the result. Read the parent skill first, and `map-tools.md` for building.

Contents:

- [Texture faces, not brushes: caulk what nobody sees](#texture-faces-not-brushes-caulk-what-nobody-sees)
- [Look from the top first, then pick the render by the question](#look-from-the-top-first-then-pick-the-render-by-the-question)
- [Light through a wall is the lighting's to fix](#light-through-a-wall-is-the-lightings-to-fix)

## Texture faces, not brushes: caulk what nobody sees

Materials are the mapper's: the agent lays the ones it is asked for, face by face.

- **Caulk every face nobody sees** — wall tops and bottoms against a floor or ceiling, backs against the sky box: `caulk` draws nothing and costs nothing, a visible material there still draws. **Never caulk a clip brush's face**: a face's material is also what it collides as (parent skill).
- **Faces that meet share one texture origin**, so the tiles of one room's walls run on without a seam across brushes; put it at the floor's corner when a texture must start at the floor (a wainscot, a tile row).
- **A trim takes a tile as high as its face**, so one repeat spans it — a baseboard or a beam's band.
- **Swap a material where it is**, each face keeping its tile, as Radiant's Find/Replace does, rather than retexturing from scratch.

## Look from the top first, then pick the render by the question

The top view is the layout: outlines, entity names and the ids, to measure against the plan. In 3D, pick the render by the question: `fullbright` for materials, `fake` for shapes, a bake for light, where an unbaked map is black and a sun leak or a dark room shows. The template's volumes (the start zone, sun, umbra and fpstool volumes) cover the start room and draw their tool textures over a shot from inside them as well as from outside (verified in a test Radiant): hide them for the shot, and the sky box for a shot from outside the map. Every view moves the mapper's camera before putting it back, and they see it move: take the views the work needs.

## Light through a wall is the lighting's to fix

Sunlight along a wall's foot or through a corner comes from the lighting's setup: **t7kb:lighting** has what keeps it out. Then bake and look from inside each space.
