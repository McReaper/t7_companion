---
name: shaders
description: How to make a custom Black Ops 3 shader — a custom techset (a stock techsetdef copied under a new name) whose technique points at custom HLSL in share/raw/shaders_stable, started from the stock shader decompiled by t7kb:shader_decompile, and the materials that use it. Covers technique sources and defines, keeping the stock vertex shader, ps_main/vs_main entry points, material constants and textures bound by name ($Globals), the engine's PerSceneConsts (gameTime), SV_Position.w as distance, and what the linker compiles and caches in shaders/pc/v7. Use when someone wants a glowing, pulsing, scrolling, distance-fading or hologram surface no stock material type gives, asks for a stock shader's HLSL, or a custom material type or shader edit doesn't show. Distinct from t7kb:assets (materials of stock types, GDT authoring, the stock-variant cache miss `failed to open source file`) and t7kb:fx-editing (particle effects) — this is writing the shader itself.
---

# Writing a custom shader for Black Ops 3

A custom shader isn't written from scratch: it is **a stock technique with one source swapped for your HLSL**. Copy a stock techsetdef under a new name, point a technique's pixel (or vertex) shader at a file in `share/raw/shaders_stable`, and start that file from the stock shader decompiled with `t7kb:shader_decompile` — so every name the techset and the engine bind by is already right. Look up techset syntax and stock material types in **t7kb** (`t7kb:search`, `t7kb:gdt_schema` with a `material_type`); the material's GDT side is **t7kb:assets**, building is **t7kb:compiling**, particle effects are **t7kb:fx-editing**.

## The linker compiles your source and keeps the stock shaders from its cache

(Verified on a real build.) The mod tools ship the stock shaders compiled, not their HLSL: `share/assetconvert/shaders/pc/v7` holds ~26,000 files named `<source>_<stage>_main_<hash>` (`techsetdef_unlit.hlsl_ps_main_CJK6…`). For a technique whose source is stock, the linker looks the compiled shader up there by source and `defines`; for a source the cache doesn't have, it compiles the file from `share/raw/shaders_stable` and **adds the result to `v7`**, under a hash of the source's content.

So, in a copied technique:

- **Keep the stock vertex shader as it is, `defines` included.** The cache has exactly that variant; change its source or its defines and the linker looks for one nothing ships, and fails like **t7kb:assets**' `failed to open source file`.
- **Every edit of your source adds a file to `v7`** (`my_shader.hlsl_ps_main_<new hash>`). They are harmless, but when you remove a custom shader, delete its `my_shader.hlsl_*` files there too.
- **A shader-only edit needs a relink, not a compile**: `t7kb:build` with `stages="link"`.

## Start from the stock shader: t7kb:shader_decompile

Call `t7kb:shader_decompile` with the material type you're copying and the stage (`name="emissive_add", stage="ps"`). One stock source compiles to many permutations, most of them the same program:

- **One program** → its HLSL, in one call (`alike` counts the permutations that share it).
- **Several** → each with what it reads: `globals` (material constants), `resources` (textures, samplers, buffers), `inputs` (what it takes from the vertex shader), next to `material_params`, the constants the material type's techset gives. Pick the one whose globals the techset gives (for `emissive_add`: the program reading `colorTint`), then call again with its `shader` as `name`.

The HLSL compiles back to the same shader — t7_dxbc is checked against the game's own shaders by recompiling and running its output — with the names a techset binds by: entry point `ps_main`/`vs_main`, material constants as global variables, the reflected texture and sampler names and registers. It reads at the register level (`r0`, `i0` banks, `asuint`): **leave the decompiled block as it is and add your code after it**, working on its output (`out_SV_TARGET0`). That keeps a known-good baseline: comment your block out and you're back to the stock look.

## Copy the techset, change only the source

1. **Copy** `share/raw/techsetdefs_stable/<category>/<stock>.techsetdef` to `<category>/my_material_type.techsetdef`. The file name **is** the material type.
2. **In each in-game technique** (the `#else` side of `#if TOOLSGFX == "1"`), change only the pixel shader's `source`: `ps = PixelShader() { source = "my_shader.hlsl" … }`. Keep its `defines` and its bindings (`colorMapSampler = "colorMap"`), and the vertex shader's stock source.
3. **Put the same file in `share/raw/techsetdefs_stable_toolsgfx/`** at the same path: the install keeps a twin of every techsetdef there (verified in the install: 1,153 of 1,154 identical).
4. **Write `share/raw/shaders_stable/my_shader.hlsl`**: the decompiled HLSL, then your code.
5. **The material**: `materialCategory` = the techset's `Globals()` category, `materialType` = the file name. `t7kb:gdt_edit` and `t7kb:gdt_check` validate both.
6. **Use it** on brushes (built as `wc/<material>`) or a model (`mc/<material>`), never as a bare `material,` zone line (**t7kb:assets**), and build.

APE and Radiant preview the material with the `#if TOOLSGFX` branch, which keeps a stock tools shader (`ToolsGfx/…`): **your HLSL only shows in game** (read in the install's techsetdefs).

## What your HLSL can read, and how it binds

(Verified in game.)

- **Material constants** — a techset's `Color( "colorTint" )`, `float1( "hdrScale" )` … fill the HLSL **global variable of the same name** (`float3 colorTint;`). Declare them as globals, not in your own cbuffer: the compiler gathers globals into `$Globals`, which the techset fills by name. A constant the techset declares but no global reads is simply unused.
- **Textures and samplers** — the technique line `colorMapSampler = "colorMap"` binds the HLSL resource `colorMapSampler` to the techset's `Texture( "colorMap" )`. Keep the decompiled names and registers.
- **The engine's per-scene constants** — declare only what you read, at its offset:
  ```hlsl
  cbuffer PerSceneConsts : register(b1)
  {
    float4 gameTime : packoffset(c69); // w: seconds — stock shaders scroll with gameTime.w
  }
  ```
  For another constant, decompile a stock shader that reads it: its `PerSceneConsts` declaration gives every name and offset.
- **Distance** — in a pixel shader, `SV_Position.w` is the pixel's view depth: how far in front of the camera it is, in world units. `.xy` is its position on screen.
- **What the vertex shader passes** — the decompiled shader's input parameters match, register for register, what the technique's stock vertex shader writes. Keep them. To read something it doesn't pass (vertex color, normal), pick a stock program whose `inputs` include it.

## Effects that work this way

(Verified in game: a material that is a cyan, scanlined hologram from afar, turns real cell by cell as the player walks up, and shows an energy wave sweeping it up close.)

- **Pulse**: `color * (1 + 0.25 * sin(gameTime.w * 2))`.
- **Scroll**: sample at `uv + gameTime.w * speed`.
- **Fade or reveal with distance**: `saturate((far - SV_Position.w) / (far - near))` drives a `lerp`.
- **Glitch, dissolve**: a hash of `floor(uv * cells)` against a threshold.

On an additive material (`emissive_add`'s `"add + depth"` state), what you output is added to what's behind it: black is invisible, and brighter adds more light.

## Removing a custom shader cleanly

Delete the techsetdef (both copies), the HLSL, its `v7` files and the materials. Then run `gdtdb /rebuild` (the build tool's `gdt_rebuild=true`), not `/update`: `/update` keeps a deleted GDT's assets in its database (verified on the install).

## Don't invent

The stock HLSL sources aren't shipped. Header trees from community packs (LG-RZ's `BlackOps3Shaders`: `code/`, `gfxcore/`, `lib/`) are reconstructions, useful for names but not ground truth. A decompiled stock shader is. Ground a constant's name and offset in a decompiled shader that reads it, and a techset's syntax in the install's own techsetdefs, before writing either.
