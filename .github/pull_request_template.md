## What failed, or what was missing

<!-- The symptom a modder hit, in their words — the exact error string if there was one. -->

## The change

<!-- Which skill(s), and the trap/correction in one or two sentences (symptom → cause → fix → why). -->

## How it was verified

<!-- For each BO3 claim: a real build/run, a file in the raw mod-tools install (path + line), or t7kb doc_ids with their reliability. Mark anything community-only as such. -->

## Checklist

- [ ] `python plugin/skills/contribute/scripts/check_skills.py` reports 0 errors
- [ ] No personal paths, private map content, or credentials in the diff
- [ ] Description still under 1,024 characters, no `<tags>`, third person — and a trigger case in `plugin/evals/` added or adjusted if it changed
- [ ] New/renamed/removed skill: `knowledge-base`'s description and `.claude-plugin/marketplace.json` updated
- [ ] `plugin/.claude-plugin/plugin.json` version left alone (bumped at release)
