Detail for **t7kb:mapping** (`plugin/skills/mapping/SKILL.md`) — how the map looks, through t7kb's map tools: texturing face by face (the materials are the mapper's choice), the views to check them, and keeping the sun out. Read the parent skill first, and `map-tools.md` for building.

Contents:

- [Texture faces, not brushes: caulk what nobody sees](#texture-faces-not-brushes-caulk-what-nobody-sees)
- [Look from the top first](#look-from-the-top-first)
- [Sun through a wall is the lighting's to fix](#sun-through-a-wall-is-the-lightings-to-fix)

## Texture faces, not brushes: caulk what nobody sees

A new `brush` takes one material on all its faces, at the material's own tile; a `texture` op then lays materials face by face — `faces` `top`, `bottom`, `sides` or directions (`+x,-y`) — in the same batch, naming the brush `@n` (the op that made it). The tile defaults to the size the install's maps use the material at most (the answer's `tiles` says which and from how many faces; a material they hardly use gets its GDT's tiling at 16 texels a unit), so bricks and planks come out at the scale Treyarch's maps have them. Then:

- **Caulk every face nobody sees** — wall tops and bottoms against a floor or ceiling, backs against the sky box: `caulk` draws nothing and costs nothing, a visible material there still draws. Name those faces in a `texture` op, and **never caulk a clip brush's face**: a face's material is also what it collides as (see the parent skill).
- **Keep one `origin` for faces that meet**: tiles start on it (the world's 0 0 0 by default), so walls of one room given the same tile run on without a seam across brushes. Set it to the floor's corner when a texture must start at the floor (a wainscot, a tile row).
- **Trims**: a `tile` as high as the face makes one repeat span it — a baseboard or a beam's band.
- `rotate` turns planks and stripes (90 to run them the other way), `flip` mirrors, `scale` 2 doubles the tile. `material` alone swaps a material and keeps each face's tile, as Radiant's Find/Replace does.

Look at the result with `t7kb:map_view` `render` `fullbright` and `hide` `volumes` from a `camera` inside the room: the template's volumes (`volume_fpstool`, the start zone, sun and umbra volumes) cover ±1536 and draw their tool textures over a shot from inside them too (verified in a test Radiant: half a room's shot was `volume_fpstool`'s face), and `hide` hides every volume and trigger for the shot alone.

## Look from the top first

`t7kb:map_view` with `view` `top` is the layout: outlines, entity names, the ids written on it, and the `scale` (image pixels per unit) to measure with. In the 3D view pick the render by the question (Radiant's F5–F8): `fullbright` for textures, `fake` for shapes, `bake` for light: it runs Radiant's own bake (the lighting toolbar's bolt, at its L/M/H) and shoots under real lighting, where an unbaked map is black and a sun leak or a dark room shows. A volume's brush (the `volume_sun`, the start zone) draws its tool texture over everything behind it, from outside it or inside (verified: sun icons covering the shot), so pass `hide` `volumes`; `hide` `sky` takes the sky box's brushes away for a shot from outside the map. Every view puts the user's camera, render method, 2D axis, zoom and centre back, but they see them move: take the views the work needs.

## Sun through a wall is the lighting's to fix

Sunlight along a wall's foot or through a corner doesn't call for overlapping brushes: walls 8 thick or more, the `volume_sun`'s `shadowSplitDistance`, shadows on the lights, probes in every space and a `high` bake keep it out, all in **t7kb:lighting**. Then `bake` and look from inside each space.
