# Trigger evals

One case per directory: a prompt a real user might type, graded by which skill Claude invokes (`tool_used` on `Skill`). About a third are near-misses that must land on a *neighbouring* skill — that's what tests a boundary. Run from the repo root:

```sh
claude plugin eval plugin --ablation none --runs 1                      # the whole suite, once
claude plugin eval plugin --case scripting-zm-callback-never-fires --runs 1 --ablation none
```

Every run is a real model call billed to your plan. `--ablation none` skips the no-plugin baseline, which can't invoke plugin skills anyway. Results land in `plugin/evals/results/` (gitignored). When you change a skill's `description`, add or adjust a case here — see `plugin/skills/contribute/SKILL.md`.
