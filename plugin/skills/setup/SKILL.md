---
name: setup
description: How to install, update, or repair the t7kb Black Ops 3 knowledge-base tool — downloads the binary and database, registers the `t7kb` MCP server via `claude mcp add`, and offers the AGENTS.md/CLAUDE.md primer at the BO3 mod-tools root. Typically run once right after installing the plugin, invoked explicitly as `/t7kb:setup` — but also use whenever the user asks to install, set up, reinstall, or update t7kb, wants its MCP server (re)registered, or reports the `search`/`get` tools are missing, unavailable, or won't connect.
---

# Install t7kb

One-time setup: download the t7kb tool + database, then register it as an MCP server so it auto-starts in future sessions. Do these steps for the user's OS, reporting what you run.

## 1. Run the installer — it's idempotent, don't force a redownload

Detect the OS and run the matching command. It downloads the binary, the embedding model, and the ~0.9 GB database archive into one folder, and prints the install path. Both installers are idempotent: if a binary + database are already present at the target path, they skip the download instead of re-fetching ~0.9 GB — so it's safe to run this even if the user already installed manually or in a prior session.

If a binary already exists at the target path (the case the idempotency check would otherwise just skip), run `<path>/t7kb update-check` (`.exe` on Windows) first — it's a lightweight, on-demand-only network check (no automatic/background calls anywhere else in `t7kb`), and reports one of three things:

- **Up to date** — proceed to skip the install as usual.
- **Update available** — tell the user, and offer to re-run with `-Force`/`--force` below. Also say that this only refreshes the binary + database: the release also moves the plugin manifest version in lockstep (enforced at release time), so a matching Claude Code plugin/skills update exists too, but getting it is a separate step through Claude Code's own plugin update flow — mention it, don't attempt to check or trigger it yourself.
- **Check failed** (e.g. offline) — don't block setup on it; fall back to skipping the install as before, with a one-line note that the check itself couldn't run.

Only pass the force flag below if the user explicitly wants to reinstall/update.

- **Linux / macOS / WSL:**
  ```bash
  curl -fsSL https://raw.githubusercontent.com/t7-reapy/t7_companion/main/install/install.sh | bash
  ```
  Installs to `~/.t7kb`; binary at `~/.t7kb/t7kb`. Add `--force` (piped: `... | bash -s -- --force`) to reinstall/update.

- **Windows (PowerShell):**
  ```powershell
  irm https://raw.githubusercontent.com/t7-reapy/t7_companion/main/install/install.ps1 | iex
  ```
  Installs to `%LOCALAPPDATA%\t7kb`; binary at `%LOCALAPPDATA%\t7kb\t7kb.exe`. Add `-Force` (run the script directly, not piped, to pass args) to reinstall/update.

**Validate before moving on:** check the command's output for an error (network failure, permission denied, disk space) rather than assuming success. If it failed, stop and report the exact error — don't proceed to step 2 and register an MCP server for a binary that was never installed.

## 2. Register the MCP server — always double-quote the path

Use the **absolute path** to the installed binary from step 1 (include `.exe` on Windows — a path without the extension fails to spawn on Windows). **Always double-quote the path**, even if it looks safe unquoted:

```bash
claude mcp add t7kb -- "/absolute/path/to/t7kb" mcp
```

Example paths: `~/.t7kb/t7kb` (Linux/macOS, expand `~` to the real home) or `C:\Users\<you>\AppData\Local\t7kb\t7kb.exe` (Windows).

On Windows this command still runs through a POSIX-style shell (the Bash tool is git-bash), which treats an unquoted backslash as an escape character and silently drops it before a non-special letter — `C:\Users\<you>\AppData\Local\t7kb\t7kb.exe` becomes `C:Users<you>AppDataLocalt7kbt7kb.exe`, a path that can't spawn. Double-quoting the argument (as above) prevents this.

**Validate the registration before moving on:** run `claude mcp list` and confirm the `t7kb` entry shows the full path with backslashes intact (Windows) or the full absolute path (Linux/macOS). If a path was registered before this fix, or the server ever fails to connect, this is the first thing to check — re-run the `claude mcp add` command above with the quoted path to fix it.

## 3. Offer the workspace primer — and record the root t7kb:knowledge-base depends on

Walk up from the current directory to find the **BO3 mod-tools root** — the folder containing `raw/`, `share_raw/`, `usermaps/`, or `mods/` as siblings (a map/mod project usually lives *inside* that tree, e.g. `usermaps/<name>/`, not at the root itself). That root is also where Treyarch's shipped files live, so it's the same tree the "verify against ground truth" guidance in `t7kb:knowledge-base` points at.

If you find that root and it has no `AGENTS.md` yet, offer to drop the vendor-neutral primer there. Fetch it rather than hand-copy — it's not bundled with the plugin, and this is the single source of truth:

```bash
curl -fsSL https://raw.githubusercontent.com/t7-reapy/t7_companion/main/templates/AGENTS.md -o "<root>/AGENTS.md"
```
```powershell
irm https://raw.githubusercontent.com/t7-reapy/t7_companion/main/templates/AGENTS.md -OutFile "<root>\AGENTS.md"
```

You just walked up to `<root>` *because* it's the raw mod-tools install — the same ground truth `t7kb:knowledge-base`'s "verify against ground truth" section tells the agent to search for on every question. Save that discovery so nobody has to repeat it: append a short, project-specific section to the fetched `AGENTS.md` (this is appending a fact after the canonical fetch, not hand-copying the primer itself, so it doesn't fight the single-source-of-truth rule):

```markdown

## This install

- Raw mod-tools root: `<root>` (this file's directory) — already confirmed present, no need to search for it again.
```

Do this whether `AGENTS.md` was just fetched or already existed — if it already existed but is missing this section, append it (check first so you don't duplicate the section on a re-run).

Claude Code reads `CLAUDE.md`, not `AGENTS.md` — but it walks the directory tree the same way (any `CLAUDE.md` from the root down to wherever a session is launched gets loaded). So if there's no `CLAUDE.md` at that root either, also create a one-line one that imports the file instead of duplicating it:

```
@AGENTS.md
```

Dropping both once at the BO3 root means every session opened anywhere under it — the whole install, or a specific `usermaps/<map>`/`mods/<mod>` subfolder — picks the guidance (and the recorded raw-install path) up automatically. A per-map/mod `AGENTS.md`/`CLAUDE.md` still works on top for that project's own conventions: both Claude Code and AGENTS.md-aware tools accumulate ancestor files rather than let a closer one replace them.

If `CLAUDE.md` already exists at the root, don't overwrite it — offer to add the `@AGENTS.md` import line to it instead (with confirmation), or just tell the user they can add it themselves. Skip the primer/path-recording entirely if no BO3-root markers are found — don't write these into unrelated repos.

## 4. Confirm — never claim success if a step failed

Tell the user setup is done and that the `t7kb:search` and `t7kb:get` MCP tools become available in the **next session** (or after `/reload-plugins`). The 3.5 GB database unpacks itself automatically the first time the server runs.

If the install or registration failed at any step above, report the exact error there instead of reaching this step — don't claim success.
