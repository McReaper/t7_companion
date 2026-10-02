---
name: hud-lui
description: How to work with LUI (Black Ops 3's HUD/menu system) and its embedded Lua — the L3akMod prerequisite, the Engine/element/stock-widget API surface, layout via anchors and margins, events and function overrides, zoning Lua files, overriding vs hooking, and common UI-error causes. Use for HUD elements, custom menus/widgets, perk icons, loading/preview screens, hintstring color codes, and any Lua-in-BO3 task. The text inside a string is t7kb:localization's (`.str` files, reference naming, per-language linking) — this skill owns the widget drawing it. Owns only the Lua half of the GSC/CSC <-> Lua clientfield bridge — subscribing to a model and reading its value in a widget; registering the clientfield and calling `set` on it is GSC/CSC and belongs to t7kb:scripting, which also owns GSC/CSC generally.
---

# Working with LUI/HUD and Lua in Black Ops 3

**Lua is where most of BO3's UI lives — LUI (the menu/HUD system) — and it is a separate language and separate craft from GSC/CSC.** Gameplay logic is GSC (server) / CSC (client); most menus, widgets, and HUD overlays are LUI, written in Lua (technically HavokScript) — though not everything on-screen is: simple always-visible HUD text/timers can be built in pure GSC via `NewHudElem()` (stock helpers in `hud_util_shared.gsc`), no Lua involved. Look up exact API names, `Enum.*` values, and specific widget classes in **t7kb** (`t7kb:search` then `t7kb:get`) — this skill is the craft and the gotchas around it. Sourcing here mixes raw Discord (~0.25 reliability) with curated wiki writeups (~0.70-0.85) — prefer the latter when they disagree.

## Prerequisite: L3akMod

**Stock BO3 mod tools cannot compile or load custom LUI at all** — this support comes from a community patch, **L3akMod**. Nothing else here works without it. Install: requires the VC++ 2013 *and* 2015 x64 redistributables, then it's a single-file swap — replace `<bo3_root>/bin/libtiff64r.dll` with L3akMod's version. Credit "The D3V Team" (DTZxPorter, SE2Dev, Nukem) if you redistribute it. Check it first for any "my custom Lua doesn't load".

## Lua-in-BO3 basics (differ from vanilla Lua)

- **Requiring another file** uses dots, not paths: `require("a.b")` loads `a/b.lua` (drop the folder slash and the `.lua` extension) — not standard Lua's `require("a/b.lua")`.
- **Declare locals, not globals.** Unless a name is meant to be a shared/overridable entry point, use `local function`/`local` — undisciplined globals are a real out-of-memory risk here, not just a style nit.
- **Wrap risky calls in `pcall`.** A **runtime** Lua error can break other open menus, not just yours — only *syntax* errors are caught at compile time. If a widget does anything that can fail (parsing external data, indexing something that might be nil), `pcall` it.

## Creating and registering a menu/HUD: worked example

A menu is a function registered under `LUI.createMenu.<Name>`; `CoD.Menu.NewForUIEditor("<Name>")` allocates the base element you attach controls to. This is the entry point for a new HUD; every custom HUD has this shape (register → allocate → anchor fullscreen → add a control → clean up on close):

```lua
function LUI.createMenu.T7Hud_zm_factory(Instance)
    local Hud = CoD.Menu.NewForUIEditor("T7Hud_zm_factory")

    Hud.soundSet = "HUD"                  -- sound profile
    Hud:setOwner(Instance)                -- owner = the root instance
    Hud:setLeftRight(true, true, 0, 0)    -- stretch horizontally, 0 margin
    Hud:setTopBottom(true, true, 0, 0)    -- stretch vertically, 0 margin
    Hud:playSound("menu_open", Instance)

    Hud.TestText = LUI.UIText.new(Hud, Instance)
    Hud.TestText:setLeftRight(true, false, 20, 250)
    Hud.TestText:setTopBottom(true, false, 20, 50)
    Hud.TestText:setText("Hello World!")
    Hud:addElement(Hud.TestText)

    -- every element you add must be closed when the HUD closes, or it leaks
    local function OnHudClose(Sender)
        Sender.TestText:close()
    end
    LUI.OverrideFunction_CallOriginalSecond(Hud, "close", OnHudClose)

    return Hud
end
```

Naming the file `t7hud_zm_factory.lua` and zoning it as `rawfile,ui/uieditor/menus/hud/t7hud_zm_factory.lua` overrides the stock Zombies HUD **for a mod**. A map's zone won't allow reusing the stock filename; see the rename + `LuiLoad` technique below.

## Overriding a stock HUD: the map/mod split matters

**For a map, you cannot override a LUI file at the zone level** — two zoned files can't share the same path, unlike GSC overrides. The real technique: keep the root function named `LUI.createMenu.T7Hud_zm_factory`, but save the file under a *different* name (e.g. `t7hud_zm_custom.lua`), zone that name, then call `LuiLoad("ui.uieditor.menus.hud.t7hud_zm_custom")` from CSC's `main()` **before** `zm_usermap::main()` — by the time `main()` runs the stock HUD is already loaded, so `LuiLoad`-ing your file re-defines the function and Lua's last-loaded-wins semantics make yours win. **Mods don't have this restriction** — a mod can override the stock file directly at the same path, as in the worked example above.

**Overriding is last-loaded-wins, not a hook system**: unlike GSC (see t7kb:scripting), Lua globals are whichever definition loaded last.

**Overriding `t7hud_zm_factory` doesn't reach every stock map** — each map opens its own HUD menu (`t7hud_zm` for Shadows of Evil, `t7hud_zm_dlc5` for Kino/Moon, `t7hud_zm_tomb` for Origins, …), so a mod's factory override does nothing there (community list, t7kb 0.70). Look up the map's own HUD name first.

**A stock DLC HUD loaded into a usermap draws blank widgets.** `LuiLoad("ui.uieditor.menus.hud.t7hud_zm_castle")` + `CreateLUIMenu`/`OpenLUIMenu` loads the menu, but its `uie_t7_*_dlc` images and its strings aren't in a usermap's fastfile: zone each image with `image,<name>` and its strings with `localize,` (**t7kb:localization**). It draws on top of the factory HUD, so overlapping factory images must be overridden with transparent ones (community, t7kb 0.70).

**Overriding a stock *widget* (not a menu) has extra traps.** To replace e.g. `CoD.cursorhint_image`, `LuiLoad` a renamed file that (1) `require`s the stock widget **with the exact case its parent uses** (`require("ui.uieditor.widgets.MPHudWidgets.cursorhint_image")`, capital `MPHudWidgets`) so both share one module-cache entry — a case mismatch makes the parent `require` reload the stock and clobber your global — then (2) redefines `CoD.<widget> = InheritFrom(...)`. Two more traps: a **parent widget reaches into its child element's fields by name** — ZMCursorHint's `cursorHintImage` subscriptions call `cursorhintimage0.c1x1 / .x1x4 / .c1x2 :setImage(...)`, so a rewrite missing those exact child names throws *"Failed to notify model subscription: attempt to index a nil value"* every notify. And **`setImage` on the stock element never sticks** — its own model subscription overwrites it next frame; to force a custom image, recreate the expected children and override *their* `setImage` to ignore the pushed value (`icon.setImage = function(e) LUI.UIImage.setImage(e, RegisterImage("my_img")) end`). Decompile the stock widget first: reproduce the exact child names and `clipsPerState` the parent expects.

## The `Engine` namespace: LUI's bridge to the game

`Engine.<Name>(...)` calls reach outside the UI tree; this is only the commonly useful part — `t7kb:search` the rest.

| Category | Functions |
|---|---|
| Gamemode/level | `IsZombiesGame()`, `IsMultiplayerGame()`, `IsCampaignGame()`, `IsMenuLevel()`, `GetCurrentMap()`, `GetGametypeName()` |
| Models (data bridge) | `CreateModel(controller, name)`, `SetModelValue(model, value)`, `GetModelValue(model)`, `UnsubscribeAndFreeModel()` — the values are typically fed from GSC/CSC via clientfields, see below |
| Client/player | `GetLocalClientNum()`, `IsControllerUsed()`, `GetFullPlayerName(controller \| clientnum)`, `GetUserSafeAreaForController(controller)` |
| Dvars, console, sound | `SetDvar(name, value)`, `Exec(command)`, `PlaySound(name)`, `ForceHUDRefresh()` |
| Strings/time | `ToUpper(s)`, `ToLower(s)`, `Localize(value)`, `CurrentTime()` |

## Element API: every control shares this

Every LUI control extends `UIElement`, so this API (`Elem:funcName(...)`) works on stock controls and your own widgets alike:

| Category | Functions |
|---|---|
| Child management | `addElement(child)`, `addElementBefore(child, before)`, `addElementAfter(child, after)`, `setParent(elem)`, `getRoot()` |
| Models/subscriptions | `setModel(model)`, `subscribeToModel(model, callback)`, `subscribeToGlobalModel(instance, scope, item, callback)`, `subscribeToElementModel(element, item, callback)` |
| Lifecycle | `close()`, `isClosed()`, `closeAndRefocus(elem)`, `setClass(name)`, `getFullID()` |
| Layout | `setLeftRight(...)`, `setTopBottom(...)` (see below), `setActive`/`setInactive(bubbleToChildren)`, `hide()`/`show()` |
| Events | `registerEventHandler(name, fn)`, `processEvent(eventObject)`, `dispatchEventToParent*`/`dispatchEventToChildren` (see below) |
| Sound | `playSound(alias)`, `playActionSFX()`, `findSoundAlias(action)` |

## Stock elements you compose from

All constructed with `.new(parent, instance)` — the parent is what its margins (below) are measured from:

- **`UIElement`** — the root/base; also usable directly as a grid-like container, and what a custom widget inherits from.
- **`UIText`** — text. Font size is tied to the element's **height** and scales with it; width doesn't affect drawing, so text overflows a too-narrow box. Set the font with `setTTF("fonts/<name>.ttf")`.
- **`UIImage`** — a 2D image or solid color (`setRGB`); register first with `setImage(RegisterImage("<name>"))`. Stretches to the element's width/height.
- **`UIStreamedImage`** — like `UIImage` but waits for a streamed image, showing a spinner meanwhile; fires `streamed_image_ready`.
- **`UIVerticalList`** / **`UIHorizontalList`** — stack children vertically/horizontally; `addSpacer` adds a divider; don't scroll.
- **`UIList`** — like `UIVerticalList` but scrollable.
- **`UIButton`** — the base clickable control; exposes hover/leave/click/mousedown/mouseup events, skinnable by nesting other controls inside it.

## Layout: anchors and margins, not coordinates

Every element positions itself with `setLeftRight(isLeft, isRight, marginLeft, marginRight)` and `setTopBottom(isTop, isBottom, marginTop, marginBottom)` per axis — **both anchors true** stretches the element to fill its parent with the given margins (`true, true, 0, 0` = fullscreen, no margin, the standard HUD container); **one anchor true** fixes the element relative to that single edge, and the second number becomes a size rather than an opposite-edge margin (e.g. `setLeftRight(true, false, 20, 250)` places an element 20px from the left with size 250). This is what keeps a HUD responsive across resolutions instead of using fixed coordinates.

## Events: `registerEventHandler` and function overrides

Two distinct patterns:
- **Element events** (button clicks, image-stream-ready, menu-loaded): `Elem:registerEventHandler("<event_name>", handlerFn)`. Each element type raises its own events — a `UIButton` raises `hover`/`leave`/`click`/`mousedown`/`mouseup`; check the stock element you're extending for which ones it actually fires before assuming a generic one exists.
- **Hooking a stock function** (most commonly `close`, for teardown): `LUI.OverrideFunction_CallOriginalSecond(target, "funcName", yourFn)` runs your function *after* the original; `OverrideFunction_CallOriginalFirst` runs it *before*. The worked example uses it to clean up `Hud.TestText`; skipping it leaks elements across open/close.

## Building a reusable widget

Package child elements behind one constructor (stock HUDs do: ammo, score and perks are each a widget):

```lua
CoD.TestControl = InheritFrom(LUI.UIElement)

function CoD.TestControl.new(HudRef, InstanceRef)
    local Elem = LUI.UIElement.new()
    Elem:setClass(CoD.TestControl)
    Elem.id = "TestControl"

    local TextChild = LUI.UIText.new(Elem, InstanceRef)
    TextChild:setLeftRight(true, true, 0, 0)
    TextChild:setTopBottom(true, true, 0, 0)
    TextChild:setText("Hello World!")
    Elem:addElement(TextChild)

    return Elem
end
```

Zone the widget's file as its own `rawfile`, then `require` it (dotted path, no `.lua`) from wherever you instantiate it — `local Control = CoD.TestControl.new(HudRef, InstanceRef)` — and add it to a HUD/menu with `addElement` like any other control.

## Zoning Lua into the game

Lua files ship as raw source or precompiled:
- `rawfile,<path>.lua` — same as any other zoned raw file.
- `*.luac` — precompiled Lua; the linker can include these directly, useful for pulling in stock/dumped widgets without a `.lua` source.
- `#precache("lui_menu", "<name>")` in GSC/CSC to register a custom menu; `#precache("lui_menu_data", "<property>")` for a menu property name; `#precache("eventstring", "<name>")` for a LUI event name.
- For a **map**, a Lua file usually needs `LuiLoad` called from GSC/CSC (see above); for a **mod**, it typically doesn't. Each widget you force-load also needs its child widgets zoned — a missing child is a common "why is nothing showing" cause.

## The GSC/CSC ↔ Lua bridge: this skill owns only the Lua half

Two distinct channels:
- **Server/client state → Lua (clientfields).** `clientfield::register` both sides in `init`, then `set` server-side — that GSC/CSC half is **t7kb:scripting**'s, not covered again here. On the Lua side, a widget subscribes (`subscribeToGlobalModel`) and reads the value (`Engine.GetModelValue`) — the half this skill actually owns. This is how HUD elements reflect server-driven state (health, notifications, custom UI models via `clientfield::set_player_uimodel`).
- **Lua button press → your Lua handler.** Register directly on the button's own `click` event: `MyButton:registerEventHandler("click", OnClick)` (see Events above). There's no separate GSC-side "menu response" channel to wire up for a plain button press — the handler runs in Lua; call back into GSC/CSC only if the click needs to change gameplay state, via whatever mechanism that system already exposes (a dvar, an `Engine.Exec`'d command, etc.).
- **A custom perk never shows a HUD icon even when its GSC and CSC are right.** The stock perk container iterates a **hard-coded** key→image table and subscribes only to `hudItems.perks.<key>` for those keys, so a perk that isn't in it never draws and nothing errors. Override the container with the table extended — the factory and Shadows of Evil bases use different file, table and image names (community, t7kb 0.70; the stock container ships compiled).
- **Some stock UI models are written but never registered in a usermap.** `_zm_craft_shield.gsc` writes `zmInventory.widget_shield_parts`, and nothing in the stock usermap framework registers it, so shield-part icons never appear. Register it as a `clientuimodel` (1 bit, `int`) in both the map's `.gsc` and `.csc` (**t7kb:scripting** for the GSC half). (**Verified in the install**: the write, and the absent registration.)
- **Menus freeze after prolonged use** when a widget is missing its **back-button** and/or `lose_focus` callback — every focusable widget needs both `gain_focus`/`lose_focus` handlers and a way to go back (`GoBack(menu, controller)` on the back button / Escape), or focus gets stuck.

## Images, icons, fonts

- Import via **APE**, untick **Streamable** (UI assets shouldn't stream), zone it with `image,<name>`, then reference it in Lua with `RegisterImage("<name>")` and `element:setImage(...)`. Changing the image's `semantic` re-ticks Streamable, in APE and in `t7kb:gdt_edit` alike (`image.awi` sets it for every semantic but `effectMap`), so set `streamable` to `0` after the semantic, or in the same `gdt_edit` request.
- **Loading/preview images** for a map are just files, not APE assets: drop `loadingscreen.png` and `previewimage.png` directly into `usermaps/<mapname>/zone/`.
- **A load/intro video fails with no error unless every rule holds**: named `zm_<map>_load`, in `usermaps/<map>/zone/video/`, an MKV with **no audio track** (any audio stream and it won't play), and it plays in **solo only**. Newer HandBrake builds produce files that crash BO3 (1.0.3 is the reported safe one). Sound comes only from a cinematic alias `bik_zm_<map>_load` (`CIN_C_MOD` template) in a companion mod. (Two community guides, t7kb 0.70.)
- **Custom fonts** override named stock TTFs — `default.ttf` (general menu text), `escom.ttf`/`FoundryGridnik-Medium.ttf`/`FoundryGridnik-Bold.ttf` (menu text), `RefrigeratorDeluxe-Regular.ttf` (scoreboard, weapon name, hintstrings), `wearetrippinshort.ttf` (score points, ammo) — per three community wikis that disagree slightly (check in game) — by dropping your replacement into a `fonts` folder and zoning `ttf,fonts/<name>.ttf`. **Must be a real TTF, not OTF** — an OTF causes heavy in-game lag rather than an obvious error. A **map**'s font override never reaches the main menu; only a mod's does.
- Font/UI errors or "no UI at all" are usually a **path or linking problem**: wrong font path relative to the raw/usermap root, a widget created but never added/linked to its parent, or a Lua file that's zoned but never actually `require`d/loaded (or `LuiLoad`ed, for a map). Bisect by commenting out custom Lua files and re-enabling one at a time rather than guessing.

## Hintstring color: `^1`–`^9` inline, Lua only past the palette

- Basic color uses `^1`–`^9` (and `^0`) inline in the string, e.g. `"^3some text^7 more text"` (`^7` resets to white) — no Lua needed for this. Custom colors beyond that palette *do* require Lua.
- The **text itself** should be a localized string, not a raw literal — and the whole `.str` craft (the `<STRFILE>_<REFERENCE>` naming rule, the `triggerstring` precache, per-language files and link passes, `linkerflag,noloc`) belongs to **t7kb:localization**. Go there for a prompt that prints its raw reference name instead of the words; it isn't a LUI problem.

## Tools & examples

- **T7Overcharged** turns opaque LUI errors into readable **stack traces** (and stops them freezing the game) — near-essential for debugging a widget rewrite, where the vanilla console only shows an error code. Use the **`Scroptss/T7Overcharged`** fork: it updates the memory offsets for the current BO3 exe; the 2022 original (JariKCoding) and 2023 (JxstNoTex) builds **crash instantly** on a patched game. Install: DLL → `usermaps/<map>/zone/` (runtime file, re-copy if the linker cleans `zone/`), its two lua → `ui/util/` zoned as `rawfile`, `dvar_hash_list.txt` → BO3 root, then `require("ui.util.T7Overcharged")` + `InitializeT7Overcharged({mapname=..., filespath=[[.\usermaps\<map>\]], workshopid="0"})` in a LuiLoad-ed file. It only reports errors as `<filehash>:<line>: (*stripped)` (shipped LUI has debug names stripped), so you still map the hash back to a file yourself.
- **Zorteok** (also D3V Team) is a Lua disassembler — reads compiled `.luac` back to readable Lua, drag-and-drop, BO2/3/4 support. Use it to study a stock or dumped widget before overriding it.
- Two full open-source HUD bases exist as learning references: `t7hud_zm_factory` and `t7hud_mp`, released by the D3V Team, plus the `T7Hud_template.lua` in the ZM Basic/Advanced Level Radiant templates — those two templates are **community** (MidgetBlaster's T7 asset pack), not Treyarch's; the stock ZM Mod Level template has no Lua — retrieve one from t7kb rather than starting a HUD from a blank file.
- Anything pulled via a decompiler (Zorteok or otherwise) is decompiled source like any other in this corpus: **paraphrase, never quote verbatim**, and don't ship someone else's undisclosed HUD/menu work without permission.

## Don't invent

LUI/Lua API names (`Engine.*`, `CoD.Menu.*`, stock widget classes) are shipped tokens — confirm them against the raw mod-tools install. If neither t7kb nor the install supports an API call or zoning directive, don't assert it exists.
