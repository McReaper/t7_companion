// Package shader finds Black Ops III's compiled shaders in the mod tools'
// cache, groups a source's permutations by the program they compiled to, and
// decompiles one to HLSL with t7_dxbc. It backs the shader_decompile tool
// (internal/cli/shader_*.go), which names a shader by its cache file, its
// source, or a material type (through the techset the gdt package resolves).
//
// # The cache
//
// The cache is share/assetconvert/shaders/pc/v7, a file per compiled shader
// named <source>_<stage>_main_<hash> (techsetdef_unlit.hlsl_ps_main_CJK6…):
// the HLSL source with its directory's slashes made underscores, the stage
// (ps, vs, gs, cs) and a hash of what was compiled. The linker adds a file
// here for every custom source it compiles.
//
// # Permutations
//
// The hash covers the source's content and its defines (a vertex and a pixel
// shader can share one, and editing a source changes it), and the stock
// sources aren't shipped, so it can't be recomputed: the permutation a
// techset uses is found by grouping a source's files by program instead,
// which permutations differing only by defines the code ignores share. Those
// count as one.
//
// # The decompiler
//
// The DXBC to HLSL work is t7_dxbc's (github.com/McReaper/t7_dxbc), a
// repository of its own with its own oracles: a decompilation bug is fixed
// there, and this package only finds, groups and pages.
package shader
