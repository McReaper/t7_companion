---
name: setup-or-update
description: How to install, update, or repair the t7kb Black Ops 3 knowledge-base tool — downloads the binary and database, registers the `t7kb` MCP server via `claude mcp add`, records the BO3 mod-tools root in a short `CLAUDE.md` there (replacing the old `AGENTS.md` primer), and offers to turn on the plugin's auto-update. Run once right after installing the plugin, invoked explicitly as `/t7kb:setup-or-update` — but also use whenever the user asks to install, set up, reinstall, update or upgrade t7kb, when the SessionStart hook reports the t7kb binary and plugin versions differ, when the user wants its MCP server (re)registered, or reports the `search`/`get` tools are missing, unavailable, or won't connect.
---

# Install or update t7kb

Download the t7kb tool + database, register it as an MCP server so it auto-starts in future sessions, record the BO3 root, and keep the plugin current. Do these steps for the user's OS, reporting what you run. On an update, every step still applies: each one checks what is already in place.

## 1. Run the installer — it's idempotent, so force it only to update

Detect the OS and run the matching command. It downloads the binary, the embedding model, and the ~0.9 GB database archive into one folder, and prints the install path. Both installers are idempotent: if a binary + database are already present at the target path, they skip the download — safe to run even after a manual or earlier install.

If a binary already exists at the target path (the case the idempotency check would otherwise just skip), run `<path>/t7kb update-check` (`.exe` on Windows) first — it's a lightweight, on-demand-only network check (no automatic/background calls anywhere else in `t7kb`) — and act on what it reports:

- **Up to date** — proceed to skip the install as usual.
- **"Running a local/dev build"** — the binary at the install path was built from source, not downloaded from a release, so it may lack tools the skills name. Unless the user says they put it there on purpose, treat it like an available update and offer `-Force`.
- **Update available** — tell the user, and offer to re-run with `-Force` below. That refreshes the binary + database only, never the plugin. **Tell the user first that it stops t7kb's MCP server in every open session** (Windows can't replace the binary or the database while one runs): they restart Claude Code afterwards. When the release changed the database, the update deletes the old one, and the first session to start unpacks the new one, about 20 s once.
- **"The t7kb Claude Code plugin (its skills) is behind …"** (with either verdict above) — one or more installs of this plugin, per scope, are on an older version. Relay the listed installs and offer to run the printed `claude plugin marketplace update` / `claude plugin update … --scope …` commands (a `project`/`local` one from the project path it names); after they succeed, tell the user to restart Claude Code, which is what loads the new skills. Step 4 keeps it from falling behind again.
- **Check failed** (e.g. offline) — don't block setup on it; fall back to skipping the install as before, with a one-line note that the check itself couldn't run.

When the SessionStart hook sent the user here (the binary and the plugin versions differ), the update-check above says which side is behind. Otherwise, only pass the force flag below if the user explicitly wants to reinstall/update.

Releases are Windows-only, like the mod tools. In PowerShell:

```powershell
irm https://raw.githubusercontent.com/McReaper/t7_companion/main/install/install.ps1 | iex
```

It installs to `%LOCALAPPDATA%\t7kb`; the binary is `%LOCALAPPDATA%\t7kb\t7kb.exe`. Add `-Force` (run the script directly, not piped, to pass args) to reinstall/update.

**Validate before moving on:** check the command's output for an error (network failure, permission denied, disk space) rather than assuming success. If it failed, stop and report the exact error — don't proceed to step 2 and register an MCP server for a binary that was never installed.

## 2. Register the MCP server — always double-quote the path

Skip this step when `claude mcp list` already shows `t7kb` with the installed binary's full path. Otherwise use the **absolute path** to `t7kb.exe` from step 1, `.exe` included: a path without it fails to spawn. **Always double-quote the path**, even if it looks safe unquoted:

```bash
claude mcp add t7kb -- "C:\Users\<you>\AppData\Local\t7kb\t7kb.exe" mcp
```

This command runs through a POSIX-style shell (the Bash tool is git-bash), which treats an unquoted backslash as an escape character and silently drops it before a non-special letter — `C:\Users\<you>\AppData\Local\t7kb\t7kb.exe` becomes `C:Users<you>AppDataLocalt7kbt7kb.exe`, a path that can't spawn. Double-quoting the argument (as above) prevents this.

**Memory:** the server keeps the knowledge base's vectors in memory, about 570 MB, so a search answers in ~0.1 s instead of ~2 s. On a machine short of memory (8 GB, with the game and the mod tools open), or if the user asks for it, register it with that turned off — searches then read the vectors from the database:

```bash
claude mcp add t7kb -e T7KB_VECTOR_INDEX=0 -- "C:\Users\<you>\AppData\Local\t7kb\t7kb.exe" mcp
```

**Validate the registration before moving on:** run `claude mcp list` and confirm the `t7kb` entry shows the full path with backslashes intact. If the server fails to connect, check this first — re-run the `claude mcp add` command above with the quoted path to fix it.

## 3. Record the BO3 mod-tools root in a `CLAUDE.md` there — t7kb:knowledge-base reads it

Walk up from the current directory to find the **BO3 mod-tools root** — the folder holding `bin/`, `share/` and `map_source/`, with `usermaps/` or `mods/` beside them (a map/mod project usually lives *inside* that tree, e.g. `usermaps/<name>/`, not at the root itself). That root is also where Treyarch's shipped files live: the ground truth `t7kb:knowledge-base`'s "verify against ground truth" section has the agent check. Record it so nobody repeats the search. Claude Code loads every `CLAUDE.md` from that root down to wherever a session starts, so one file there covers the whole install and every `usermaps/<map>`/`mods/<mod>` under it; a per-project `CLAUDE.md` still layers on top. Skip this step entirely if no BO3-root markers are found.

- **No `CLAUDE.md` at the root:** write this stub, `<root>` being the absolute path:
  ```markdown
  # Black Ops 3 mod-tools root

  - Raw mod-tools root: `<root>` (this file's directory) — confirmed present; use it as is.
  - BO3 modding goes through the t7kb skills and MCP tools; if the tools are missing, run `/t7kb:setup-or-update`.
  ```
  Everything else an agent needs comes from the MCP server's instructions and the skills, which update with t7kb; what the stub records never changes, so it never needs updating.
- **A `CLAUDE.md` without the root fact:** offer to append the first bullet (with confirmation). Check first, so a re-run doesn't add it twice.
- **The old primer:** an `AGENTS.md` starting `# AGENTS.md — Black Ops 3 modding workspace`, usually beside a `CLAUDE.md` holding only `@AGENTS.md`, which t7kb 2.x fetched. It digests the skills into always-loaded context, and that copy never updated, so an agent acts on it and never loads the skill. Tell the user it is superseded and offer to replace it with the stub, keeping the root it recorded under `## This install`. Before deleting, compare it with the primer's last version (`https://raw.githubusercontent.com/McReaper/t7_companion/v2.9.0/templates/AGENTS.md`) and show the user anything beyond it — lines of their own go into the new `CLAUDE.md` if they want them. In a `CLAUDE.md` with more than the import, remove only the `@AGENTS.md` line. **Delete either file only on the user's yes.**

### Offer to keep that root in git

Modding edits **shared, install-wide files** — an assetlist line commented out to override a stock asset, a GDT `gdt_edit` rewrites, files a community pack drops into `source_data/` or `deffiles/` — and Steam's "verify files" and APE saves overwrite files silently. A git history of the install root makes each change reviewable and revertible, and lets you read `git log`/`git diff` to see what changed since the last good build. `gdt_edit` keeps only one `.bak`.

- **The root is not a git repository** (`git -C "<root>" rev-parse` fails): explain that, and offer to set it up. **Run `git init` only on the user's yes.** With it: `git init` at the root, then fetch the default ignore and LFS rules beside it (skip a file that already exists), then suggest committing in steps — the install is ~10 GB — starting with `zone_source/all/assetlist/`, `deffiles/`, `source_data/`, `share/raw/`, `usermaps/` and `mods/`.
  ```bash
  curl -fsSL https://raw.githubusercontent.com/McReaper/t7_companion/main/templates/bo3.gitignore -o "<root>/.gitignore"
  curl -fsSL https://raw.githubusercontent.com/McReaper/t7_companion/main/templates/bo3.gitattributes -o "<root>/.gitattributes"
  ```
  ```powershell
  irm https://raw.githubusercontent.com/McReaper/t7_companion/main/templates/bo3.gitignore -OutFile "<root>\.gitignore"
  irm https://raw.githubusercontent.com/McReaper/t7_companion/main/templates/bo3.gitattributes -OutFile "<root>\.gitattributes"
  ```
  The ignore file keeps out what the tools regenerate and large stock art; the user adds their own art with `git add -f`, which the attributes send to Git LFS (`git lfs install` once).
- **It is a repository with no `.gitignore`**: offer the same two files.
- **It already has one**: leave it alone.

## 4. Keep the plugin current — its source, then auto-update

### The marketplace must point at `McReaper/t7_companion`

Installs made before 2026-09-29 added the marketplace from `t7-reapy/t7_companion`. That GitHub organization is deleted: updates still arrive through GitHub's redirect to the repository's new home, but the freed name can be registered by anyone, and the marketplace would then serve their plugins instead (a plugin's hooks run code on the user's machine).

- **Check** `claude plugin marketplace list --json`: the `t7-reapy` entry must have `"source": "github"` and `"repo": "McReaper/t7_companion"`.
- **Any other source:** explain the above in two sentences and, on the user's yes, set `extraKnownMarketplaces["t7-reapy"].source` to `{"source": "github", "repo": "McReaper/t7_companion"}` in the settings file that declares it (`~/.claude/settings.json`, else the project's `.claude/settings.json`), keeping every other key. Then run `claude plugin marketplace add McReaper/t7_companion`, which re-clones the marketplace from there; the installed plugin stays (verified on a test install). The settings edit comes first: Claude Code refuses an `add` whose source differs from the declared one.

### Offer auto-update — the only automatic path to the next release

Nothing else tells the user a release exists: a third-party marketplace such as `t7-reapy` doesn't auto-update by default, the SessionStart hook works offline (it compares the installed binary's version with the plugin's, so it only speaks once one of them has updated), and `t7kb update-check` runs only when asked. With auto-update on, Claude Code updates the plugin in the background and the next session loads it; the hook then sees the binary behind and sends the user back here.

- **Read the setting** from the user's `~/.claude/settings.json`: `extraKnownMarketplaces["t7-reapy"].autoUpdate` (`claude plugin marketplace add` declared the marketplace there). Read only that settings file, never Claude Code's internal `known_marketplaces.json` — its format is Claude Code's own and can change.
- **Not `true`:** explain the above in a sentence and offer to set `"autoUpdate": true` on that entry. Write it only on the user's yes, editing the JSON in place and keeping every other key. If they'd rather do it themselves: `/plugin` → Marketplaces → `t7-reapy` → Enable auto-update.
- **When it can't take effect:** with `DISABLE_UPDATES`, `DISABLE_AUTOUPDATER` or `CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC` set and `FORCE_AUTOUPDATE_PLUGINS=1` not set, Claude Code skips the plugin auto-update pass whatever the setting says. Say so rather than setting it, and leave the choice of environment to the user.

## 5. Confirm what was done, and report a failed step as failed

Tell the user what was done. After a first install, the `t7kb:search` and `t7kb:get` MCP tools become available in the **next session** (or after `/reload-plugins`); after an update, a restart of Claude Code loads the new binary and skills. Either way, the ~3.8 GB database unpacks itself the first time the server starts, about 20 s once.

If the install or registration failed at any step above, report the exact error there instead of reaching this step — don't claim success.
