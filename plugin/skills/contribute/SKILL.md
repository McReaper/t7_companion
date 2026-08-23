---
name: contribute
description: How to author or revise a skill in this repo (the t7kb Claude Code plugin) — when a new `bo3-*` skill is warranted versus extending an existing one, the frontmatter description craft that drives triggering, the house body style (claim-shaped headings, method-and-traps rather than reference dumps), the grounding rules every BO3 claim has to pass (t7kb reliability → raw mod-tools install), the follow-ups that are mandatory when you add/rename/remove a skill, and how to test triggering against sibling skills. Use whenever you're writing, editing, splitting, or reviewing a `plugin/skills/*/SKILL.md` here, or when the user says a skill is missing, stale, wrong, or should cover something new. This is the t7kb-specific version of Anthropic's `skill-creator`. Not for answering BO3 modding questions themselves — that's bo3-knowledge and the bo3-* craft skills.
---

# Writing a t7kb skill

A skill here is **not documentation** — the corpus already is the documentation, and an agent can `search` it. A skill is the thing retrieval can't give you: the **method** for a craft, plus the **silent-failure traps** that nobody wrote down cleanly because they were only ever discovered by losing an afternoon to them. Every editorial rule below follows from that one distinction.

Read two or three existing skills before writing (`plugin/skills/mapping/`, `zombies-ai/`, `debugging/` are the clearest short examples) — matching the house voice matters more than following this file to the letter.

## First: does this actually need a new skill?

There are already 14 domain skills (plus two meta, `setup` and `contribute`) over one fairly narrow domain, so **the marginal cost of a new one is trigger competition**, not disk space. A skill that overlaps a sibling makes *both* fire less reliably, and the model picks between them from descriptions alone. Default to **extending an existing skill**.

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
- **Single-file until the body earns a split.** Skills load in three levels — frontmatter always in context, body when triggered, bundled files only on demand — so `references/` earns its keep once the body gets long, or splits cleanly per-variant (one file per tool/platform the agent picks between). Below that, an extra hop just costs a read.
- **Judge the split on words, not lines.** `bo3-moving-platforms` was only 172 lines but ~3,900 words of dense prose; `bo3-anim-retarget` was 350 lines and 6,582 words. Both needed splitting. **Keep the body under 500 lines (hard) and aim under 3,000 words**; a line count alone will tell you a dense skill is fine when it isn't.
- **References go one level deep, and no deeper.** Every `references/*.md` must be linked directly from `SKILL.md` — a reference that links to another reference gets partially read (the agent previews it with `head` instead of reading it whole) and you get silently incomplete information. Give any reference file over 100 lines a table of contents at the top, and a one-line backlink telling the reader to load `SKILL.md` first. Working examples: `anim-retarget/references/` (four files, split per path) and `moving-platforms/references/` (one file of secondary detail).

## The description is the entire trigger

The body only ever gets read if the description won. Claude also **undertriggers** by default, so a shy, tasteful description silently loses. The house pattern:

```
How to <do the craft> — <specific sub-topics, in real BO3 jargon>. Use when <symptom-side situations, including literal error strings>. Distinct from <sibling> (<what that one owns>) — this is <the niche>.
```

**The 1,024-character limit is hard, and it is validated — not a guideline.** Measure before you commit, counting characters and not bytes (these descriptions are full of em-dashes and `→`, which are three bytes each in UTF-8, so `wc -c` overstates by 3-8):

```sh
python -c "import io,re;print(len(re.sub(r'^description: ','',[l for l in io.open('SKILL.md',encoding='utf-8') if l.startswith('description:')][0]).strip()))"
```

Four skills here silently shipped over that ceiling (`bo3-moving-platforms` at 1,529, `bo3-animation` at 1,407, `bo3-fx` at 1,397, `bo3-anim-retarget` at 1,354) — nothing warns you, so it has to be checked. **Trimming is a real editorial job:** cut connectives, hedges and repeated jargon, never a symptom trigger, a literal error string, or the `Distinct from` clause. And treat a description that lands at 1,005-1,015 as a smell — if the ceiling is what stopped you writing, the description is doing too many jobs; several here cluster just under it because they were optimised to the limit rather than to the reader.

What makes these work:

- **Jargon density is a feature.** `iwfx`, `angleVelRoll`, `scriptparsetree`, `LEDs`, `clientfield` are the words a user actually types. Generic phrasing ("effects and particles") matches nothing specific.
- **Trigger on symptoms, not topics.** The strongest lines in this repo are failure descriptions: "when a ported anim looks bound but limbs are mis-rotated/exploding", "when a spawned model won't spin or spins jerky". A user with a problem describes the problem, not its taxonomy — and paste an exact error string when the craft has one (`unable to find animation '<name>' in tree 'all_player'`).
- **Name the negative.** `bo3-zombies-ai` says "This is in-game AI … not LLM/agent AI, don't conflate the two", because that collision is real. Ruling something out is as useful as ruling it in.
- **The "Distinct from" clause is not optional** on anything adjacent to a sibling.

## Body style

**Open with the thesis, not a definition.** One sentence naming what the craft *actually is*, phrased as the correction to the wrong mental model — "Radiant is brush/patch geometry, not code", "Shipping a map/mod is a pipeline of separate stages". Then the two standing pointers, in this order: look exact tokens up in **t7kb** (`t7kb:search` then `t7kb:get`), and hand off to the sibling skills that own the neighbouring stages.

**Make every `##` heading the conclusion, not the topic.** This is the single most recognisable thing about these files, and it's what lets an agent skim-and-act:

| Write this | Not this |
|---|---|
| `## Prefabs: rotate the prefab, not the brush` | `## Prefabs` |
| `## Spawning zombies: it's mostly Radiant KVPs, not a GSC call` | `## Spawners` |
| `## Custom traversals need a behavior tree entry — the animation existing isn't enough` | `## Traversals` |
| `## Getting real line numbers (the usermap trap)` | `## Line numbers` |

**Each trap states symptom → cause → fix → why.** The "why" is what lets the model generalise to the variant of the bug you didn't write about; a bare imperative only covers the case in front of you. Explain the mechanism instead of shouting — if you're reaching for ALL-CAPS `MUST`/`NEVER`, you're usually compensating for an unexplained reason.

**Mark how a fact was learned.** Where a claim came from a real run rather than the corpus, say so ("verified on a real headless build", "verified, recurring report"). It tells a future editor what they're allowed to rewrite.

**Mechanics:** backtick every shipped token (`clip_nosight`, `-onlyents`, `PlayFXOnTag`), bold the load-bearing claim, bold sibling skill names (**bo3-debugging**) so handoffs are visible while skimming, and close with a `## Don't invent` section — a domain-specific restatement that unsupported tokens don't get asserted. **Name MCP tools fully** — `t7kb:search`, `t7kb:get`, `t7kb:build`, never a bare `search`/`get`; an unqualified name is one the agent may fail to resolve when several MCP servers are registered. **Never hard-wrap** (repo-wide rule): one line per paragraph or list item.

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
5. **The skill that owns a workflow, whenever the tool surface changes.** This is the one that has actually bitten: `03ab81c` added the `build` MCP tool and updated six skills — but not `bo3-compiling`, the single skill that governs building. So `bo3-compiling` went on naming only the `t7kb build` shell form, agents dutifully shelled out (some straight past it to the raw mod-tools binaries), and `CLAUDE.md` meanwhile still claimed `build` was "deliberately **not** an MCP tool". Adding or changing a tool means updating the skill that routes agents to it **in the same commit**, and checking `CLAUDE.md` doesn't now contradict the code.
6. **Commit** as `feat(skills):` for a new skill, `docs(skills):` for editing one. `plugin/.claude-plugin/plugin.json`'s `version` gets bumped for the release that ships it (CI gates version-vs-tag) — not once per edit.

## The external checklist this house style sits on top of

Anthropic publishes [skill-authoring best practices](https://platform.claude.com/docs/en/agents-and-tools/agent-skills/best-practices); OpenAI publishes a thinner [Skills guide](https://developers.openai.com/api/docs/guides/tools-skills) plus a [cookbook](https://developers.openai.com/cookbook/examples/skills_in_api). Neither ships a numbered list — this is the distillation, tagged **[A]** Anthropic, **[O]** OpenAI, **[A+O]** both. **Where any of it conflicts with the sections above, this file wins** — the claim-shaped headings, the jargon-dense descriptions and the grounding order are deliberate departures, and they exist because this corpus is a query tool with 14 competing triggers, not a generic skill library.

1. [A] **Assume the model is already smart.** Only add context it lacks; make each paragraph justify its token cost.
2. [A+O] **The description is the entire trigger.** Third person, specific, says what it does *and* when to use it.
3. [O] **State the negative** — `Use when…` plus `Distinct from…`, with the near-misses that must route elsewhere.
4. [A] **Match degrees of freedom to fragility.** Prose where many approaches work; one exact command where the operation is fragile and order-dependent.
5. [A] **Progressive disclosure** — body under 500 lines, detail in bundled files.
6. [A] **References one level deep**, TOC on anything over 100 lines.
7. [A] **Evaluations before prose.** Baseline the model *without* the skill, write at least three scenarios, then write only enough to pass them.
8. [A] **Must work on the weakest model you ship to.** What Opus infers, Haiku needs stated.
9. [A+O] **Workflows get numbered steps and a validate→fix→repeat loop**, not a bare imperative.
10. [A+O] **Scripts behave like tiny CLIs:** deterministic stdout, fail loudly, handle their own errors, no unexplained constants.
11. [A] **Hygiene:** no time-sensitive claims (use an "old patterns" section), consistent terminology, forward slashes, fully-qualified MCP tool names.
12. [O] **Skills are privileged code.** Review before installing, pin versions, gate side-effecting actions — and don'''t duplicate a skill'''s content into always-loaded context, or callers act on the copy and never load the skill.

On 12: `templates/AGENTS.md` deliberately restates a floor of craft rules for agents running **without** this plugin, which is a legitimate exception — but each such bullet has to end by pointing at the skill that owns it and telling the agent to load that instead. Duplication as a fallback is fine; duplication that reads as sufficient is not.

## Test the trigger, not just the prose

Prose you can review by reading; triggering you can't. The failure mode here is never "the body was bad", it's "the wrong skill fired" — so test against the **siblings**, not against nothing:

- Write 6–10 prompts a real user would type — messy, lowercase, with file paths and half-remembered error text. Half should land on the new skill; half should be **near-misses that must land on its neighbour** (that's what makes the boundary real; an obviously-unrelated prompt tests nothing).
- Run them in a fresh session and check which skill actually fires, then which fires when the skill is absent — the delta is the skill's whole value.
- Anthropic's `skill-creator` plugin (official marketplace) automates the heavier version of this, including a description-optimisation loop that scores trigger rates across iterations. Worth it for a skill you expect to be hit constantly; overkill for a one-section edit.

Then reread your draft cold, and cut whatever isn't pulling its weight. A tight 50-line skill that fires reliably beats a thorough 200-line one that doesn't.

## Don't invent

The conventions above are read off the existing skills and `CLAUDE.md` — when in doubt, check what the siblings actually do rather than inferring a rule, and if you're deliberately breaking one, say why in the commit. And apply this skill's own standard to itself: don't put a BO3 claim in a skill you couldn't ground in t7kb or the raw install, no matter how confident it sounds in a forum post.
