---
name: crossref
description: How to use other Call of Duty titles' GSC/CSC source and script dumps as reference when modding Black Ops 3 — the game/engine decoder ring (t5/t6/t8/t9 Treyarch, iw4–iw8/h1/h2 Infinity Ward, s1/s2/s4 Sledgehammer), which lineages actually transfer, and xensik's gsc-tool for (de)compiling scripts across games. Use when porting from another CoD (source-side naming/structure) or studying how a mechanic is implemented elsewhere to reimplement it in BO3. These are references for structure/approach only — never a source of BO3 token names; verify shipped BO3 tokens against the raw install per t7kb:knowledge-base. Distinct from t7kb:assets (the BO3-side extract/compile/material/anim porting pipeline itself) and t7kb:anim-retarget (the Maya HumanIK/`-mo` cross-generation retarget craft) — this skill only finds and reads the other game's source, it doesn't do the port or the retarget; for writing the BO3 GSC/CSC itself see t7kb:scripting.
---

# Cross-referencing other Call of Duty titles

Other CoD titles' GSC/CSC — decompiled dumps and leaked/reconstructed source — is a useful **reference** for two BO3 tasks: **porting an asset** (the source game's own scripts name and structure what you're ripping) and **reimplementing a mechanic** (see how another title shapes a system, then rebuild it in BO3 idiom). This skill is the decoder ring for which game is which, which lineages transfer, and the tool that produces these dumps. It is **not** part of t7kb — these are external repos you fetch on demand (they carry no reliability score of their own).

## The one rule: reference, not BO3 truth

Cross-game source describes *other games*. **t7kb:knowledge-base**'s governing constraint applies here — "cross-game intuitions (other CoD titles, generic engine terms) are usually wrong here." So:

- **Never paste cross-game GSC into a BO3 script.** Function names, KVPs, asset fields, notetracks, and even calling conventions differ across titles. Read the other game to understand *how a system is shaped* — its state machine, event flow, data layout — then re-implement in BO3 idiom.
- **Verify every shipped token against the raw BO3 install** (the `scripts/` tree and assets under the mod-tools root) before asserting it exists in BO3. A function that's clearly there in BO2 or Cold War may be renamed, absent, or subtly different in BO3.
- **Rank them low.** These are community decompilations/dumps — inherently paraphrased and version-specific, on par with (or below) the low-reliability end of t7kb. Prefer t7kb and the raw install; use a cross-game dump to fill a *structural* gap, not a naming one.

## Engine lineage — what transfers

- **Treyarch line (`t5 t6 t7 t8 t9`) transfers most** — shared zombies architecture, powerup/perk lineage, and stdlib heritage. Mind the *syntax* split: BO3's `#using`/`#namespace`, `&func` pointers, `#insert` macros, and clientfields are the **t7-era** system, shared by **t8/t9** (so those read closest to BO3 line-for-line, though t8 moved the compiled format on again); **t5/t6 predate it** and use path-based `maps\...::func()` includes, so from BO1/BO2 you port the *concept and structure* and translate the syntax. **BO2 (`t6`) zombies is the direct ancestor of BO3 zombies** — the best lineage read for round logic, powerups, and perks.
- **Infinity Ward line (`iw4`–`iw8`, `h1/h2`) and Sledgehammer line (`s1 s2 s4`) are more divergent** — different stdlib, calling conventions, and asset APIs. Reach for them mainly (a) **source-side when porting an asset** from that game — the dump tells you the real function/asset/anim names of what you're extracting (pairs with **t7kb:assets**), and (b) as a **second opinion on a mechanic's general approach** — never for names you'll paste into BO3.

## The catalog: the id is the anchor, not the repo name

Game id → title → lineage → community source. The **id** is the anchor (it's what gsc-tool and rippers use); repo names churn, so treat these as starting points and confirm the current mirror.

**Treyarch (T-line — closest to BO3):**

| id | Title | Community source |
|----|-------|------------------|
| `t5` | Black Ops (2010) | [plutoniummod/t5-scripts](https://github.com/plutoniummod/t5-scripts) |
| `t6` | Black Ops II (2012) | [plutoniummod/t6-scripts](https://github.com/plutoniummod/t6-scripts) |
| **`t7`** | **Black Ops III (2015) — this is you** | mod tools ship the GSC/CSC source under `scripts/` (ground truth) |
| `t8` | Black Ops 4 (2018) | [shiversoftdev/t8-src](https://github.com/shiversoftdev/t8-src) |
| `t9` | Black Ops Cold War (2020) | [shiversoftdev/t9-src](https://github.com/shiversoftdev/t9-src) |

**Infinity Ward (IW-line):**

| id | Title | Community source |
|----|-------|------------------|
| `iw4` | Modern Warfare 2 (2009) | [shit-ware/IW4](https://github.com/shit-ware/IW4) |
| `iw5` | Modern Warfare 3 (2011) | [plutoniummod/iw5-scripts](https://github.com/plutoniummod/iw5-scripts), [Brentdevent/MW3-GSC-Dump](https://github.com/Brentdevent/MW3-GSC-Dump) |
| `iw6` | Ghosts (2013) | [alterware/iw6-scripts](https://git.alterware.dev/alterware/iw6-scripts), [mjkzy/iw6dev-gsc-dump](https://github.com/mjkzy/iw6dev-gsc-dump) |
| `iw7` | Infinite Warfare (2016) | [AuroraDoesCode/CODIW-Source](https://github.com/AuroraDoesCode/CODIW-Source) |
| `iw8` | Modern Warfare (2019) | [Sku-111/mw19-gsc-dump](https://github.com/Sku-111/mw19-gsc-dump) |
| `h1` | MW Remastered (2016) | [mjkzy/h1-gsc-dump](https://github.com/mjkzy/h1-gsc-dump) |
| `h2` | MW2 Campaign Remastered (2020) | [alicealys/h2-dump](https://github.com/alicealys/h2-dump) |
| — | H2M (MWR-era restoration mod) | [S3RAPH-1M/H2M-GSC-Dump](https://github.com/S3RAPH-1M/H2M-GSC-Dump) |

**Sledgehammer (S-line):**

| id | Title | Community source |
|----|-------|------------------|
| `s1` | Advanced Warfare (2014) | [mjkzy/s1-gsc-dump](https://github.com/mjkzy/s1-gsc-dump) |
| `s2` | WWII (2017) | [mjkzy/s2-gsc-dump](https://github.com/mjkzy/s2-gsc-dump) |
| `s4` | Vanguard (2021) | [mjkzy/s4-gsc-dump](https://github.com/mjkzy/s4-gsc-dump) |

## gsc-tool (xensik) — read/write compiled scripts across games

[github.com/xensik/gsc-tool](https://github.com/xensik/gsc-tool) is the compiler/decompiler/(dis)assembler these dumps are produced with. Five modes via `-m`: `comp` (source → `.gscbin`), `decomp` (`.gscbin` → source), `asm` / `disasm` (bytecode ↔ `.gscasm`), and `parse`. Reach for it when the game/version you want **isn't already dumped** — pull the compiled scripts from that install and `decomp` your own readable GSC/CSC.

Two things to get right:

- **Per-game support is uneven — check the current build's target list.** At time of writing it covers `iw5 iw6 iw7 iw8 iw9`, `s1 s2 s4`, `h1 h2`, `t6`, and `t7 t8 t9 t10` / `jup` (MWIII 2023) — but several (t8/t9/t10, jup) are **work-in-progress**, and **`t7` (Black Ops III) is decompile-only** (no recompile). Note `iw9`, `t10` and `jup` have no row in the catalog above — no widely-mirrored dump, so decompiling your own is the only route for those.
- **For BO3 itself you rarely need it.** The mod tools already ship BO3's GSC/CSC as source under `scripts/`, and that raw source *is* the ground truth (see **t7kb:knowledge-base**). Decompiling `t7` is a fallback for a compiled script your install doesn't ship as source; its output is decompiler-reconstructed (paraphrased), so treat it below the shipped source.

For decompiling BO3's own *logic assets* (AI behavior/ASM, weaponfiles, tables, script bundles) into GDTs — a different job from script bytecode — that's **HydraX**, covered in **t7kb:assets**.

## Where this hands off

- **Porting an asset?** Use the source-game dump for the *names and structure* of what you're ripping, then follow **t7kb:assets** for the extract → APE compile → materials → anims pipeline on the BO3 side (and **t7kb:animation** for the anim compile specifics).
- **Porting an animation specifically?** This skill only gets you as far as the source game's rig/anim and its real names — the cross-generation retarget itself (HumanIK characterization, bind-pose traps, `-mo` constraints for a viewhands rig) is **t7kb:anim-retarget**'s craft, not this one's. Hand off there once you have the source asset identified.
- **Reimplementing a mechanic?** Study the source for shape only, then write it per **t7kb:scripting** (header/usings, stdlib, hooks, clientfields) and confirm every BO3 signature in t7kb + the raw install.

## Don't invent

These dumps describe *other games*. A function, KVP, or asset that exists in BO2 / MW3 / Cold War may have no BO3 equivalent, a renamed one, or different semantics. If neither t7kb nor the raw BO3 install supports a token, don't assert BO3 has it — no matter how clearly the other game's source shows it.
