# Releasing

Pushing a `v*` tag runs GoReleaser (`.github/goreleaser.yaml` + `.github/workflows/release.yml`): it cross-builds the Windows binary, runs a before-hook that downloads the embedding model into `models/`, and bundles binary + model + docs into `t7kb_windows_amd64.zip`. The version is injected via `-ldflags` into `internal/cli.version`.

## The release notes are the commit subjects, under hand-written highlights

- Release notes = auto-generated `changelog` + curated `footer` (install steps, in `.github/goreleaser.yaml`), and no blurb about the tool: whoever reads a release page already knows it. The changelog lists only what a user would notice — `feat` (✨ New), `fix` (🐛 Fixed) and `docs(skills)` (📚 Skill knowledge); `ci`/`chore`/`test`/`refactor`/other `docs` are dropped — and each line is the subject with its `type(scope):` prefix turned into a bold scope label, no SHA.
- **Highlights, required:** a hand-written `.github/release-notes/<tag>.md`, committed with the version bump, goes on top of the generated changelog (then headed "All changes"). CI, `pre-push` and the release fail without it. Its reader is a modder deciding whether to update: write what they can now do, or what stops going wrong for them, in their words. How it was built, checked or measured, what an earlier version got wrong, and the repo's own housekeeping stay in the commits.

## `plugin.json`'s version must match the tag

`plugin/.claude-plugin/plugin.json`'s `version` must match the tag before you push it. `release.yml` fails the job if they differ, so the Go release and the plugin version never drift — `t7kb update-check` relies on that to tell a user an "update available" also means a matching plugin/skills update. The release PR bumps it, with the highlights; other PRs leave it alone. The local `pre-push` hook catches a mismatch earlier; CI is the real enforcement.

## Only a rebuilt database needs a manual upload

`t7kb.db.zip` is **not** built by CI (it's built upstream). The release workflow **carries it forward** from the most recent earlier release that has one, and skips if the new release already has it — so only a *rebuilt* DB needs a manual upload: `gh release upload <tag> t7kb.db.zip --clobber`.

## Check a config change before tagging

Validate config changes with `goreleaser check --config .github/goreleaser.yaml`; dry-run with `goreleaser release --snapshot --clean --config .github/goreleaser.yaml`.
