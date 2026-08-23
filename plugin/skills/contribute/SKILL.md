---
name: contribute
description: How to author or revise a skill in this repo (the t7kb Claude Code plugin) — when a new `bo3-*` skill is warranted versus extending an existing one, the frontmatter description craft that drives triggering, the house body style (claim-shaped headings, method-and-traps rather than reference dumps), the grounding rules every BO3 claim has to pass (t7kb reliability → raw mod-tools install), the follow-ups that are mandatory when you add/rename/remove a skill, and how to test triggering against sibling skills. Use whenever you're writing, editing, splitting, or reviewing a `plugin/skills/*/SKILL.md` here, or when the user says a skill is missing, stale, wrong, or should cover something new. This is the t7kb-specific version of Anthropic's `skill-creator`. Not for answering BO3 modding questions themselves — that's bo3-knowledge and the bo3-* craft skills.
---

# Writing a t7kb skill

A skill here is **not documentation** — the corpus already is the documentation, and an agent can `search` it. A skill is the thing retrieval can't give you: the **method** for a craft, plus the **silent-failure traps** that nobody wrote down cleanly because they were only ever discovered by losing an afternoon to them. Every editorial rule below follows from that one distinction.

Read two or three existing skills before writing (`plugin/skills/mapping/`, `zombies-ai/`, `debugging/` are the clearest short examples) — matching the house voice matters more than following this file to the letter.

## First: does this actually need a new skill?

There are already ~13 domain skills over one fairly narrow domain, so **the marginal cost of a new one is trigger competition**, not disk space. A skill that overlaps a sibling makes *both* fire less reliably, and the model picks between them from descriptions alone. Default to **extending an existing skill**.

A new skill earns its place when all three hold:

- **Its own toolchain or workflow.** `bo3-anim-retarget` split from `bo3-animation` because Maya/HumanIK retargeting is a different tool and a different failure mode than the export→APE→linker pipeline — not because there was a lot to say about animation.
- **Silent-failure traps of its own.** If the answers are all "look up the right name", that's a `search`, not a skill.
- **A description a sibling's description wouldn't already catch.** Write the description *first*, as a test: if you can't state the boundary in one "Distinct from …" clause, it belongs inside the sibling.

When you do split, both sides get updated — the parent skill's description gains its own "Distinct from" clause pointing at the new one. Trigger boundaries are always stated twice, from both directions.

## Where the file goes

```
plugin/skills/<dir>/SKILL.md      ← one file, that's the whole skill
```

- **Domain craft → `name: bo3-<thing>`. Meta/tooling → bare name** (`setup`, `contribute`). The `bo3-` prefix means "this answers a Black Ops 3 question".
- **The directory name drops the prefix** (`scripting/` holds `name: bo3-scripting`). `bo3-knowledge/` is the one legacy exception — don't copy it, don't rename it either.
- **Single-file is the convention.** The largest skill here (`anim-retarget`, ~300 lines) is still one file. Skills load in three levels — frontmatter always in context, body when triggered, bundled files only on demand — so `references/` earns its keep once a body passes roughly 300 lines or splits cleanly per-variant (one file per tool/platform the agent picks between). Below that, an extra hop just costs a read.

## The description is the entire trigger

The body only ever gets read if the description won. Claude also **undertriggers** by default, so a shy, tasteful description silently loses. The house pattern:

```
How to <do the craft> — <specific sub-topics, in real BO3 jargon>. Use when <symptom-side situations, including literal error strings>. Distinct from <sibling> (<what that one owns>) — this is <the niche>.
```

What makes these work:

- **Jargon density is a feature.** `iwfx`, `angleVelRoll`, `scriptparsetree`, `LEDs`, `clientfield` are the words a user actually types. Generic phrasing ("effects and particles") matches nothing specific.
- **Trigger on symptoms, not topics.** The strongest lines in this repo are failure descriptions: "when a ported anim looks bound but limbs are mis-rotated/exploding", "when a spawned model won't spin or spins jerky". A user with a problem describes the problem, not its taxonomy — and paste an exact error string when the craft has one (`unable to find animation '<name>' in tree 'all_player'`).
- **Name the negative.** `bo3-zombies-ai` says "This is in-game AI … not LLM/agent AI, don't conflate the two", because that collision is real. Ruling something out is as useful as ruling it in.
- **The "Distinct from" clause is not optional** on anything adjacent to a sibling.

## Body style

**Open with the thesis, not a definition.** One sentence naming what the craft *actually is*, phrased as the correction to the wrong mental model — "Radiant is brush/patch geometry, not code", "Shipping a map/mod is a pipeline of separate stages". Then the two standing pointers, in this order: look exact tokens up in **t7kb** (`search` then `get`), and hand off to the sibling skills that own the neighbouring stages.

**Make every `##` heading the conclusion, not the topic.** This is the single most recognisable thing about these files, and it's what lets an agent skim-and-act:

| Write this | Not this |
|---|---|
| `## Prefabs: rotate the prefab, not the brush` | `## Prefabs` |
| `## Spawning zombies: it's mostly Radiant KVPs, not a GSC call` | `## Spawners` |
| `## Custom traversals need a behavior tree entry — the animation existing isn't enough` | `## Traversals` |
| `## Getting real line numbers (the usermap trap)` | `## Line numbers` |

**Each trap states symptom → cause → fix → why.** The "why" is what lets the model generalise to the variant of the bug you didn't write about; a bare imperative only covers the case in front of you. Explain the mechanism instead of shouting — if you're reaching for ALL-CAPS `MUST`/`NEVER`, you're usually compensating for an unexplained reason.

**Mark how a fact was learned.** Where a claim came from a real run rather than the corpus, say so ("verified on a real headless build", "verified, recurring report"). It tells a future editor what they're allowed to rewrite.

**Mechanics:** backtick every shipped token (`clip_nosight`, `-onlyents`, `PlayFXOnTag`), bold the load-bearing claim, bold sibling skill names (**bo3-debugging**) so handoffs are visible while skimming, and close with a `## Don't invent` section — a domain-specific restatement that unsupported tokens don't get asserted. **Never hard-wrap** (repo-wide rule): one line per paragraph or list item.

## Ground every BO3 claim

A wrong skill is worse than a missing one — it's confidently wrong at the top of the agent's context, above the corpus. So the order is always **t7kb (weigh `reliability`) → the raw mod-tools install (ground truth) → the web (last resort, disclosed)**. Anything Treyarch shipped — function names, KVPs, asset fields, error strings, paths — gets confirmed against the install before it goes in.

Some consequences worth internalising:

- **If one `search` answers it, it isn't skill material.** Reference tables of shipped tokens go stale and duplicate the corpus. Point at t7kb and spend the lines on the trap instead.
- **The best skill content is what the corpus can't say**: that something fails *with no error at all*. "`vectorRank` silently skips mismatched dimensions", "cod2map silently skips navmesh from the wrong cwd", "the rotGraph does nothing unless its curve ramps 0→1". Mine your own debugging sessions for these.
- **Negative results are first-class content.** Ruling out a plausible-but-wrong approach saves as much of a reader's afternoon as documenting the right one — that a function is a dead stub, that an API can't do the thing its name suggests, that the obvious way to close a loop is fatal. Write down what you eliminated and how you know.
- **"Not everything under the install root is ground truth."** Community prefab packs and GDTs live *inside* the mod-tools tree (`map_source/_prefabs/<pack>/`, `source_data/<pack>/`), and they routinely disagree with Treyarch's own code — following one instead of the shipped scripts is a classic way to spend hours fixing symptoms. Ground on `share/raw/scripts/`, `docs_modtools/`, and Treyarch's own `map_source/` and prefabs; treat a third-party prefab as community-reliability even though it's on disk.

**A "zero occurrences" finding is only as good as the search that produced it.** Before you assert something doesn't exist, re-run case-insensitively and across singular/plural — BO3 ships KVPs like `DYNAMICPATH` (uppercase, singular), so a search for `dynamicpaths` "proves" absence. Watch the tooling too: ripgrep-backed search respects `.gitignore`, so parts of an install can be silently invisible to it while a plain recursive `grep` sees them. Both mistakes produce a confident, wrong claim.

If you couldn't ground a claim, either cut it or label it as community-corroborated-only in the text.

## Required follow-ups when you add, rename, or remove a skill

Skipping these is how the plugin goes stale — CLAUDE.md treats them as part of the change, not cleanup:

1. **`bo3-knowledge`'s frontmatter description** — it names every other `bo3-*` skill explicitly to route around trigger overlap, so a new domain skill must be listed there (both in the topic list and the skill-name list). Meta skills (`setup`, `contribute`) stay out of it: it's a BO3-question router, not an index.
2. **`.claude-plugin/marketplace.json`** — the plugin's `description` enumerates what ships; add the domain there.
3. **Leave `README.md` alone.** Its mermaid diagram deliberately doesn't enumerate skill names so it can't go stale. Don't reintroduce a hardcoded list.
4. **`CLAUDE.md`** — add a clause only if the skill changes how a contributor works on the repo (a meta skill does; one more craft skill doesn't).
5. **Commit** as `feat(skills):` for a new skill, `docs(skills):` for editing one. `plugin/.claude-plugin/plugin.json`'s `version` gets bumped for the release that ships it (CI gates version-vs-tag) — not once per edit.

## Test the trigger, not just the prose

Prose you can review by reading; triggering you can't. The failure mode here is never "the body was bad", it's "the wrong skill fired" — so test against the **siblings**, not against nothing:

- Write 6–10 prompts a real user would type — messy, lowercase, with file paths and half-remembered error text. Half should land on the new skill; half should be **near-misses that must land on its neighbour** (that's what makes the boundary real; an obviously-unrelated prompt tests nothing).
- Run them in a fresh session and check which skill actually fires, then which fires when the skill is absent — the delta is the skill's whole value.
- Anthropic's `skill-creator` plugin (official marketplace) automates the heavier version of this, including a description-optimisation loop that scores trigger rates across iterations. Worth it for a skill you expect to be hit constantly; overkill for a one-section edit.

Then reread your draft cold, and cut whatever isn't pulling its weight. A tight 50-line skill that fires reliably beats a thorough 200-line one that doesn't.

## Don't invent

The conventions above are read off the existing skills and `CLAUDE.md` — when in doubt, check what the siblings actually do rather than inferring a rule, and if you're deliberately breaking one, say why in the commit. And apply this skill's own standard to itself: don't put a BO3 claim in a skill you couldn't ground in t7kb or the raw install, no matter how confident it sounds in a forum post.
