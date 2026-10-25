<!-- CI checks this body: every section written, every checklist item ticked or ended with n/a. -->

## What failed, or what was missing

<!-- What a modder or an agent hit, in their words: the exact error string if there was one, or what they couldn't do. -->

## The change

<!-- What changes for them, in one or two sentences; for a skill, the trap or correction (symptom → cause → fix → why). -->

## How it was verified

<!-- The tests that fail without the change, a live run on Radiant, the game or the mod tools, a file of the raw mod-tools install (path + line), or t7kb doc_ids with their reliability. Mark anything community-only as such. -->

## Checklist

<!-- Tick each item, or end it with n/a when it doesn't apply. -->

- [ ] Tested live on what it drives (Radiant, the game, the mod tools), not only on fakes
- [ ] Each `feat`, `fix` and `docs(skills)` commit subject reads as a release-note line for a modder
- [ ] A changed tool: its MCP description, the skill that owns its workflow and its `docs/cli/` page updated here
- [ ] A changed skill description: a trigger case in `plugin/evals/` added or adjusted
- [ ] A skill added, renamed or removed: `.claude-plugin/marketplace.json`'s description updated
- [ ] No personal paths, private map content or credentials in the diff
- [ ] `plugin/.claude-plugin/plugin.json`'s version left alone, unless this is the release PR
