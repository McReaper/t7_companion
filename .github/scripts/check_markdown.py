#!/usr/bin/env python3
"""Word budgets for the repo's markdown, and a warning on narrated history.

Docs state rules, not how they were learned: the story of a fix goes in the
commit message. Every markdown file has a word budget by kind (BUDGETS below,
first match wins); going over is an ERROR, so a doc can't regrow by accretion.
Phrases that usually narrate history ("the hard way", "was cut from"…) are a
WARN — some are legitimate inside a trap's description.

A file at 95% of its budget gets a WARN, and an ERROR says what to do: not trim
words elsewhere to make room (that erodes the file at random), but decide where
the new detail's reader is, and if it does belong here, move a whole block to
its reader — the largest sections are listed as candidates.

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

# (glob, word budget, where a block moved out of such a file goes) — first match
# wins; a None budget = not checked.
BUDGETS: list[tuple[str, int | None, str]] = [
    ("plugin/evals/*/prompt.md", None, ""),          # test fixtures
    ("plugin/evals/*/graders/*.md", None, ""),
    ("plugin/evals-holdout/*/prompt.md", None, ""),
    ("plugin/evals-holdout/*/graders/*.md", None, ""),
    ("README.md", 700,                               # the human front door
     "docs/ (detail for humans), a skill (for agents), the tool's own description"),
    (".claude/CLAUDE.md", 3500,                      # loaded into every contributor session
     "the package doc (internal/<pkg>/doc.go) or a comment beside the code it rules, docs/ for reference"),
    ("templates/AGENTS.md", 1200,                    # loaded into every user's agent
     "the skill that owns the craft (AGENTS.md keeps a one-line floor pointing to it)"),
    ("plugin/skills/*/SKILL.md", 3000,               # body, frontmatter excluded
     "the skill's references/ (one level deep, linked from SKILL.md)"),
    ("plugin/skills/*/references/*.md", 2000,        # loaded on demand
     "another reference file, split by what the reader comes for"),
    ("docs/*.md", 1500,                              # reference
     "a new docs/ page, split by topic"),
    (".github/release-notes/*.md", 300,              # the release page
     "the generated changelog: keep only what the user can now do"),
    ("*", 1000, "a file of its own, where its reader looks"),
]

NEAR = 0.95  # a WARN above this share of the budget: the next addition needs a move, not a trim

HISTORY = re.compile(
    r"\b(the hard way|found by|drifted|calibrat\w+ (by|over|against)|was cut from|used to be|"
    r"we tried|turned out|this has bitten|after losing)\b",
    re.IGNORECASE,
)

HEADING = re.compile(r"^(#{2,3})\s+(.*)")

errors = 0


def report(path: str, line: int, level: str, msg: str) -> None:
    global errors
    if level == "ERROR":
        errors += 1
    print(f"{path}:{line}: {level}: {msg}")


def budget_for(rel: str) -> tuple[int | None, str]:
    for pattern, words, home in BUDGETS:
        if fnmatch.fnmatch(rel, pattern):
            return words, home
    return None, ""


def body_of(text: str) -> tuple[str, int]:
    """The text after YAML frontmatter, and the 1-based line it starts on."""
    lines = text.splitlines()
    if lines and lines[0].strip() == "---":
        for i in range(1, len(lines)):
            if lines[i].strip() == "---":
                return "\n".join(lines[i + 1:]), i + 2
    return text, 1


def largest_sections(body: str, first: int, n: int = 3) -> list[tuple[int, int, str]]:
    """The n largest ##/### sections (or, without headings, top-level list items
    and paragraphs) as (words, line, title)."""
    blocks: list[list] = []  # [words, line, title]
    cur = None
    in_code = False
    for i, line in enumerate(body.splitlines()):
        if line.lstrip().startswith("```"):
            in_code = not in_code
        m = None if in_code else HEADING.match(line)
        if m:
            cur = [0, first + i, m.group(2).strip()]
            blocks.append(cur)
            continue
        if cur is None:  # text before the first heading: one block per paragraph or list item
            if line.strip() and not line.startswith((" ", "\t")):
                blocks.append([0, first + i, line.strip()[:60]])
            if blocks:
                blocks[-1][0] += len(line.split())
            continue
        cur[0] += len(line.split())
    return sorted((tuple(b) for b in blocks), reverse=True)[:n]


def check_budget(rel: str, body: str, first: int) -> None:
    budget, home = budget_for(rel)
    if budget is None:
        return
    words = len(body.split())
    if words > budget:
        biggest = "; ".join(f"line {line} \"{title}\" ({w} words)" for w, line, title in largest_sections(body, first))
        report(rel, first, "ERROR",
               f"{words} words, over its budget of {budget}. Don't trim words elsewhere to make room: first decide whether "
               f"the new detail's reader is here at all; if it is, move a whole block to its reader — {home}. "
               f"Raising the budget is a separate, justified decision (its own commit), never a way to fit this change. "
               f"Largest sections: {biggest}")
    elif words >= NEAR * budget:
        report(rel, first, "WARN",
               f"{words} of {budget} words: the next addition will need a whole block moved to its reader ({home}), "
               f"not words trimmed elsewhere")


def check(rel: str) -> None:
    if budget_for(rel)[0] is None:
        return
    text = (ROOT / rel).read_text(encoding="utf-8")
    body, first = body_of(text)
    check_budget(rel, body, first)
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
