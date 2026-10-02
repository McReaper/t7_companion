#!/usr/bin/env python3
"""Word budgets for the repo's markdown, and a warning on narrated history.

Docs state rules, not how they were learned: the story of a fix goes in the
commit message. Every markdown file has a word budget by kind (BUDGETS below,
first match wins); going over is an ERROR, so a doc can't regrow by accretion.
Phrases that usually narrate history ("the hard way", "was cut from"…) are a
WARN — some are legitimate inside a trap's description.

Usage: python .github/scripts/check_markdown.py [file.md ...]
With no arguments it checks every markdown file git tracks or would track.
Prints `path:line: ERROR|WARN: message`; exits 1 if any ERROR.
"""
from __future__ import annotations

import fnmatch
import re
import subprocess
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]

# (glob, word budget) — first match wins; None = not checked.
BUDGETS: list[tuple[str, int | None]] = [
    ("plugin/evals/*/prompt.md", None),          # test fixtures
    ("plugin/evals/*/graders/*.md", None),
    ("README.md", 700),                          # the human front door
    (".claude/CLAUDE.md", 3500),                 # loaded into every contributor session
    ("templates/AGENTS.md", 1200),               # loaded into every user's agent
    ("plugin/skills/*/SKILL.md", 3000),          # body, frontmatter excluded
    ("plugin/skills/*/references/*.md", 2000),   # loaded on demand
    ("docs/*.md", 1500),                         # reference
    (".github/release-notes/*.md", 300),         # the release page
    ("*", 1000),                                 # anything else
]

HISTORY = re.compile(
    r"\b(the hard way|found by|drifted|calibrat\w+ (by|over|against)|was cut from|used to be|"
    r"we tried|turned out|this has bitten|after losing)\b",
    re.IGNORECASE,
)

errors = 0


def report(path: str, line: int, level: str, msg: str) -> None:
    global errors
    if level == "ERROR":
        errors += 1
    print(f"{path}:{line}: {level}: {msg}")


def budget_for(rel: str) -> int | None:
    for pattern, words in BUDGETS:
        if fnmatch.fnmatch(rel, pattern):
            return words
    return None


def body_of(text: str) -> tuple[str, int]:
    """The text after YAML frontmatter, and the 1-based line it starts on."""
    lines = text.splitlines()
    if lines and lines[0].strip() == "---":
        for i in range(1, len(lines)):
            if lines[i].strip() == "---":
                return "\n".join(lines[i + 1:]), i + 2
    return text, 1


def check(rel: str) -> None:
    budget = budget_for(rel)
    if budget is None:
        return
    text = (ROOT / rel).read_text(encoding="utf-8")
    body, first = body_of(text)
    words = len(body.split())
    if words > budget:
        report(rel, first, "ERROR", f"{words} words, over its budget of {budget}: cut, or move the detail where its reader is (see CLAUDE.md)")
    if fnmatch.fnmatch(rel, ".github/release-notes/*.md"):
        return  # a release page is about what changed: the history check doesn't apply
    in_code = False
    for n, line in enumerate(text.splitlines(), 1):
        if line.lstrip().startswith("```"):
            in_code = not in_code
            continue
        m = None if in_code else HISTORY.search(line)
        if m:
            report(rel, n, "WARN", f"{m.group(0)!r} reads like history: state the rule; the story goes in the commit message")


def markdown_files() -> list[str]:
    try:
        out = subprocess.run(
            ["git", "ls-files", "--cached", "--others", "--exclude-standard", "*.md"],
            cwd=ROOT, capture_output=True, text=True, check=True,
        ).stdout
        files = [f for f in out.splitlines() if f]
    except (OSError, subprocess.CalledProcessError):
        files = [p.relative_to(ROOT).as_posix() for p in ROOT.rglob("*.md") if ".git" not in p.parts]
    return sorted(f for f in files if (ROOT / f).is_file())


def main(argv: list[str]) -> int:
    files = [Path(a).resolve().relative_to(ROOT).as_posix() for a in argv] if argv else markdown_files()
    for rel in files:
        if rel.endswith(".md"):
            check(rel)
    return 1 if errors else 0


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
