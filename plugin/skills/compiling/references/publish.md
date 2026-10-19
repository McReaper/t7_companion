# Publish uploads `zone/` as it is: link fresh, then shrink your images

Referenced from `plugin/skills/compiling/SKILL.md`. Read that file first.

The Launcher's Publish uploads the whole `usermaps/<map>/zone/` folder (verified in the install), and the linker only appends to a `.xpak`: what each link replaces stays in it, read by nothing (verified on a real build: 93 MB of a 325 MB `.xpak`). `t7kb:zone_contents` without a line gives the upload and the dead data its `.xpak` carries; link the build you publish with `t7kb:build` `fresh_xpak` (CLI `--fresh-xpak`). Deleting the `.xpak` files by hand works only for the languages you then relink.

Images are most of the rest, and their data doesn't compress in the `.xpak`. Each `mipBase` step down (APE's *Mipmap Base Size*, `1/1` → `1/2` → `1/4` → `1/8`) halves width and height and leaves a quarter of the streamed bytes (verified on a real build: a 4K colour map 22.4 → 5.6 → 1.4 MB). `t7kb:zone_contents` with `images` ranks your GDTs and images by what a step saves; set it with `t7kb:gdt_edit` (a batch per GDT), relink, and check them in game. `doNotResize` locks an image's size; stock GDTs aren't editable.
