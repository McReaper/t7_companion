---
name: contribute
description: How to improve the t7kb plugin's skills and send the change upstream as a pull request to McReaper/t7_companion — from a plugin install (fork, branch, edit, validate, `gh pr create`, only with the user's go-ahead) or from a clone of the repo. Covers when a finding deserves a new skill versus a line in an existing one, the description/trigger craft, the house body style (claim-shaped headings, symptom → cause → fix → why), the grounding bar every BO3 claim must clear (t7kb reliability → raw mod-tools install), the validator, and the mandatory follow-ups. Use when a t7kb skill's advice proved wrong, stale, or missing a step during real modding work; when a session has just verified a silent-failure trap or fix no skill mentions; when the user wants to report, fix or extend a skill, contribute, or open a PR or issue on t7kb; or when editing any `plugin/skills/*/SKILL.md`. Not for answering BO3 questions — that's t7kb:knowledge-base and the craft skills.
---

# Contributing to the t7kb skills

A skill here is **not documentation** — the corpus already is the documentation, and an agent can `t7kb:search` it. A skill is what retrieval can't give you: the **method** for a craft, plus the **silent-failure traps** that nothing in the corpus flags. So the most valuable contributions come from real sessions — a skill's advice that failed, or a fix that took the wrong theory to find. Every rule below follows from that.

## When to offer a contribution — and when not to

Offer **once**, at a natural stopping point, after the user's own problem is solved — never mid-debug. Offer when the session produced something a future session would want and no skill says:

- a t7kb skill's instruction was **wrong or stale**, and you verified the correction (a real build, the install's own files);
- a **silent failure** was diagnosed — it failed with no error, or with an error pointing elsewhere;
- a plausible approach was **ruled out** with evidence (a negative result saves as much time as a fix).

Don't offer for an ordinary answer, a lookup one `t7kb:search` returns, a community pack's own rules, or anything you couldn't verify. One sentence is enough: *"This trap isn't in the `t7kb:scripting` skill — want me to open a PR on t7kb with it?"* If they decline, drop it.

## The contribution workflow

Opening a PR publishes to a public repo, so **every outward step waits for the user's explicit yes**. Copy this checklist and work it in order:

```
Contribution:
- [ ] 1. Name the change: correction / new trap / new skill, and its target skill
- [ ] 2. Ground it: t7kb doc_ids + reliability, install paths/lines, or "verified on a real run"
- [ ] 3. Get a working copy (clone or fork) — not the installed plugin cache
- [ ] 4. Branch, edit in house style
- [ ] 5. Validate: check_skills.py until 0 errors
- [ ] 6. Scrub personal and private content
- [ ] 7. Show the diff + PR text; wait for yes
- [ ] 8. Commit, push, open the PR (or fall back to an issue/patch)
```

1. **Name the change.** Correction, new trap, or new skill (see the next section — almost always *not* a new skill). Find the owning skill from the plugin's skill list.
2. **Ground it** per *Ground every BO3 claim* below, and write down how each claim was verified — it goes in the PR.
3. **Get a working copy.** If the current directory is already a clone (it has `plugin/.claude-plugin/plugin.json` naming `t7kb`), use it. Otherwise ask where to put one — outside the user's BO3 install and map folders — then `gh repo fork McReaper/t7_companion --clone` there (or `git clone https://github.com/McReaper/t7_companion`). **Never edit the installed copy** under `~/.claude/plugins/cache/…`: it's overwritten on update and it isn't a PR.
4. **Branch and edit**: `git switch -c docs/skills-<short-slug>` from an up-to-date `main`; edit `plugin/skills/<dir>/SKILL.md` in house style (sections below). Keep the change focused — one finding, one PR.
5. **Validate**, from the working copy's root: `python plugin/skills/contribute/scripts/check_skills.py`. Fix every `ERROR` and rerun until it reports 0; read the `WARN`s. CI runs the same script on every PR, so a contributor without Python still gets the result there.
6. **Scrub.** No personal paths (`C:\Users\<name>\…` → `<you>` / `<bo3_root>`), no private map names, scripts or assets the user didn't agree to publish, no credentials, no third-party script bodies beyond the tokens the point needs.
7. **Show, then wait.** Show the user the diff and the PR title/body — title as a conventional commit (`docs(skills): …` for an edit, `feat(skills): …` for a new skill); body = what failed, what fixed it, how it was verified (the PR template's sections). Proceed only on an explicit yes.
8. **Ship it**: commit, `git push -u origin HEAD`, then `gh pr create --repo McReaper/t7_companion --base main --fill` or with `--title`/`--body-file`. Don't bump `plugin/.claude-plugin/plugin.json`'s version — the maintainer does that at release. **Fallbacks**: no `gh` or not signed in (`gh auth status`) → offer an issue instead, with the drafted text and the link `https://github.com/McReaper/t7_companion/issues/new`, or hand over `git diff` as a patch. Report the PR/issue URL back.

## First: does this actually need a new skill?

There are already 14 domain skills (plus `setup` and `contribute`) over one fairly narrow domain, so **the marginal cost of a new one is trigger competition**: a skill that overlaps a sibling makes *both* fire less reliably. Default to **extending an existing skill**. A new one earns its place only when all three hold:

- **Its own toolchain or workflow.** `anim-retarget` split from `animation` because Maya/HumanIK retargeting is a different tool and failure mode than the export→APE→linker pipeline — not because there was a lot to say.
- **Silent-failure traps of its own.** If the answers are all "look up the right name", that's a `t7kb:search`, not a skill.
- **A description no sibling already catches.** Write the description *first*, as a test: if you can't state the boundary in one "Distinct from …" clause, it belongs inside the sibling.

When you do split, both sides get a "Distinct from" clause — boundaries are stated from both directions.

## Where the file goes

```
plugin/skills/<name>/SKILL.md          ← the skill; <name> is also its frontmatter name
plugin/skills/<name>/references/*.md   ← optional detail, one level deep
```

- **`name` must equal the directory name** — the Agent Skills spec requires it, and Claude Code lists and invokes plugin skills as `t7kb:<directory>`. Refer to siblings by that listed name, `t7kb:<name>` (**t7kb:scripting**), everywhere: descriptions, bodies, `templates/AGENTS.md`.
- **Single-file until the body earns a split.** Frontmatter is always in context, the body loads when triggered, `references/` only on demand — so a split earns its keep once the body gets long or splits cleanly per variant.
- **Judge the split on words, not lines.** Body under 500 lines (hard) and **under ~3,000 words** (aim) — a 170-line skill can be 3,900 words of dense prose.
- **References one level deep.** Every `references/*.md` is linked directly from `SKILL.md`, never from another reference (a nested one gets previewed with `head` and silently half-read); a table of contents on anything over 100 lines; a backlink telling the reader to load `SKILL.md` first. Examples: `anim-retarget/references/`, `scripting/references/`.

## The description is the entire trigger

The body only ever gets read if the description won, and Claude **undertriggers** by default — a shy, tasteful description silently loses. The house pattern:

```
How to <do the craft> — <sub-topics in real BO3 jargon>. Use when <symptom-side situations, literal error strings>. Distinct from <sibling> (<what it owns>) — this is <the niche>.
```

- **Hard format rules** (the validator enforces them): ≤ **1,024 characters** — counted as characters, not bytes; **no XML-like tags**, so write placeholders as `…` or `{name}`, never `<name>`; **no `: ` or ` #`** inside the unquoted value (strict YAML rejects it — rephrase a literal error string's colon away); **third person**, no "you". Treat a description that lands at 1,005+ as a smell: it was optimised to the ceiling rather than to the reader.
- **Jargon density is a feature.** `iwfx`, `angleVelRoll`, `scriptparsetree`, `clientfield` are what users actually type; "effects and particles" matches nothing.
- **Trigger on symptoms, not topics** — "a wallbuy shows Cost 0", "zombies blind to a player in plain sight" — and paste the exact error string when the craft has one.
- **Name the negative** and keep the **"Distinct from" clause** on anything adjacent to a sibling. When trimming, cut connectives and hedges — never a symptom, an error string, or the boundary.

## Body style

- **Open with the thesis, not a definition** — one sentence naming what the craft *actually is*, phrased as the correction to the wrong mental model. Then the standing pointers: exact tokens in **t7kb** (`t7kb:search` then `t7kb:get`), and the siblings that own the neighbouring stages.
- **Make every `##` heading the conclusion, not the topic**: `## Prefabs: rotate the prefab, not the brush`, not `## Prefabs`. It's what lets an agent skim-and-act.
- **Each trap states symptom → cause → fix → why.** The *why* lets the model generalise to the variant you didn't write about. Explain the mechanism instead of shouting — reaching for ALL-CAPS `MUST` usually means a missing reason.
- **Tag a fact's provenance in a few words** — "verified on a real build", "verified in the install", "community, t7kb 0.25" — but never narrate how it was found; the discovery story goes in the commit message / PR body. The tag tells the next editor what they may rewrite.
- **Mechanics:** backtick every shipped token; bold the load-bearing claim and sibling names (**t7kb:debugging**); fully-qualified MCP tools (`t7kb:search`, `t7kb:get`, `t7kb:build`); close a domain skill with `## Don't invent`; **never hard-wrap** — one line per paragraph or list item.

## Ground every BO3 claim

A wrong skill is worse than a missing one — it's confidently wrong at the top of the agent's context. The order is always **t7kb (weigh `reliability`) → the raw mod-tools install (ground truth) → the web (last resort, disclosed)**, and anything Treyarch shipped — function names, KVPs, asset fields, error strings, paths — gets confirmed against the install before it goes in.

- **If one `t7kb:search` answers it, it isn't skill material.** Reference tables go stale and duplicate the corpus; spend the lines on the trap.
- **The best content is what the corpus can't say**: that something fails *with no error*, or that a plausible approach is a dead end.
- **Not everything under the install root is ground truth.** Community prefab packs, GDTs and script trees live inside it (`map_source/_prefabs/<pack>/`, `source_data/<pack>/`, `share/raw/scripts/Sphynx/`…), some Radiant templates and sound CSVs come from packs (the ZM Basic/Advanced Level templates under `rex/templates/` are MidgetBlaster's T7 asset pack — only ZM Mod Level is stock), and a modder's own maps sit in `usermaps/` — sometimes with the whole install under git. Ground on files dated with the install: Treyarch's `share/raw/scripts/{shared,zm,mp,core}`, `bin/t7.def.json`, `deffiles/*.awi`, `docs_modtools/`, stock prefabs and GDTs. Check a file's date before calling it stock.
- **A "zero occurrences" finding is only as good as the search.** Re-run case-insensitively and across singular/plural (BO3 ships `DYNAMICPATH`, uppercase and singular), and remember ripgrep-backed search respects `.gitignore` — use a plain recursive `grep` on the install.
- If you couldn't ground a claim, cut it or label it community-corroborated in the text.

## Required follow-ups when you add, rename, or remove a skill

1. **`knowledge-base`'s description** names every other domain skill to route around trigger overlap — add the new one (the validator checks this).
2. **`.claude-plugin/marketplace.json`** — the plugin description enumerates the domains; add it there.
3. **Leave `README.md` alone** — its diagram deliberately lists no skill names.
4. **`CLAUDE.md`** — only if the change alters how a contributor works on the repo.
5. **The skill that owns a workflow, whenever the tool surface changes** — in the same commit, or agents keep using the old route (a new MCP tool the owning skill doesn't mention goes unused).
6. **Commit** as `feat(skills):` for a new skill, `docs(skills):` for an edit. The manifest version is bumped at release, not per edit.

## Test the trigger, not just the prose

The failure mode is never "the body was bad" — it's "the wrong skill fired". So test against the **siblings**:

- `plugin/evals/` holds trigger cases for `claude plugin eval`: one prompt per case, graded by whether the right skill was invoked (`tool_used` on `Skill`). Half are **near-misses that must land on a neighbour** — that's what makes a boundary real. Add a case for any description you change, phrased the way a user types: messy, lowercase, half-remembered error text.
- Run one cheaply while iterating: `claude plugin eval plugin --case <name> --runs 1 --ablation none`; the whole suite before a PR that changes descriptions. Every run is a real model call on the user's plan — **ask before running the full suite**.

Then reread the draft cold and cut whatever isn't pulling its weight. A tight 50-line skill that fires reliably beats a thorough 200-line one that doesn't. The vendor guidance this style sits on — and where it deliberately departs — is in **`references/vendor-guidelines.md`**.

## Don't invent

The conventions above are read off the existing skills, `CLAUDE.md` and the validator — when in doubt, check what the siblings actually do rather than inferring a rule, and if you deliberately break one, say why in the PR. And apply this skill's own standard to itself: no BO3 claim goes into a skill that you couldn't ground in t7kb or the raw install, however confident it sounds in a forum post.
