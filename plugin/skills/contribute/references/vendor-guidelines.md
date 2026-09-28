# The external checklist the house style sits on top of

Referenced from `plugin/skills/contribute/SKILL.md` — read that file first. **Where anything here conflicts with it, `SKILL.md` wins**: the claim-shaped headings, the jargon-dense descriptions and the grounding order are deliberate departures, because this plugin is a query tool with many competing triggers, not a generic skill library.

Anthropic publishes [skill-authoring best practices](https://platform.claude.com/docs/en/agents-and-tools/agent-skills/best-practices) and the [Claude Code skills reference](https://code.claude.com/docs/en/skills); the open [Agent Skills specification](https://agentskills.io/specification) sets the hard format rules; OpenAI publishes a thinner [Skills guide](https://developers.openai.com/api/docs/guides/tools-skills). Tagged **[A]** Anthropic, **[S]** spec, **[O]** OpenAI.

1. [S] **`name`**: lowercase letters, digits, single hyphens, ≤ 64 characters, **must match the parent directory name**, no `anthropic`/`claude`. Claude Code lists and invokes plugin skills as `<plugin>:<directory>`.
2. [S+A] **`description`**: 1–1,024 characters, **no XML tags** (so no `<name>` placeholders — write `…` or `{name}`), third person, says what the skill does *and* when to use it. Claude Code truncates the listing at 1,536 characters (description + `when_to_use`).
3. [S] **Valid YAML.** An unquoted value can't contain `: ` or ` #`, or strict parsers reject the file.
4. [A] **Assume the model is already smart.** Only add context it lacks; make each paragraph justify its token cost.
5. [O] **State the negative** — `Use when…` plus `Distinct from…`, with the near-misses that must route elsewhere.
6. [A] **Match degrees of freedom to fragility.** Prose where many approaches work; one exact command where the operation is fragile and order-dependent.
7. [A] **Progressive disclosure** — body under 500 lines, detail in bundled files, **one level deep**, a table of contents on any reference over 100 lines.
8. [A] **Evaluations before prose** — baseline without the skill, at least three scenarios, then write only enough to pass them. Here that's `claude plugin eval` over `plugin/evals/`.
9. [A] **Must work on the weakest model you ship to.** What Opus infers, Haiku needs stated.
10. [A+O] **Workflows get numbered steps and a validate → fix → repeat loop**, not a bare imperative.
11. [A+O] **Scripts behave like tiny CLIs** — deterministic output, fail loudly, handle their own errors, no unexplained constants (`scripts/check_skills.py` is the model).
12. [A] **Hygiene** — no time-sensitive claims (use an "old patterns" note), consistent terminology, forward slashes in skill-internal paths, fully-qualified MCP tool names (`t7kb:search`, never bare `search`).
13. [O] **Skills are privileged code** — gate side-effecting actions behind the user's go-ahead, and don't duplicate a skill's content into always-loaded context, or callers act on the copy and never load the skill.

On 13: `templates/AGENTS.md` deliberately restates a floor of craft rules for agents running **without** this plugin — a legitimate exception, but each such bullet has to end by pointing at the skill that owns it. Duplication as a fallback is fine; duplication that reads as sufficient is not.
