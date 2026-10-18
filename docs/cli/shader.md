# Shaders: start a custom one from a stock one

[All commands](../cli.md).

## `t7kb shader decompile <name>`

Decompiles one of the game's compiled shaders, from the mod tools' shader cache, to HLSL that compiles back to the same shader: entry point `ps_main` or `vs_main`, the material's parameters as globals, and the names a techset binds its textures and constants by.

The name can be a cache file (`techsetdef_unlit.hlsl_ps_main_…`), a shader source (`techsetdef_unlit.hlsl`) or a material type (`emissive_add`, whose techset gives the source). When a source compiled to several programs for the stage, the answer lists them with what each reads: decompile one by its cache file name.

```
t7kb shader decompile emissive_add --stage ps
t7kb shader decompile techsetdef_unlit.hlsl
```

| Option | What it does |
| --- | --- |
| `--stage` | `ps`, `vs`, `gs` or `cs`: the stage to decompile (default every stage the source has) |
| `--json` | the answer as JSON, as the agent gets it |
