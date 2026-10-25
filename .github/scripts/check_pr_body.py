#!/usr/bin/env python3
"""A pull request's body fills .github/pull_request_template.md.

Every `## ` section of the template is in the body: a checklist section with
each of the template's items either ticked or marked "n/a" (its text kept, a
note after it allowed), any other section with text of its own, not only the
template's comment. The body may add sections and items of its own.

Usage: python .github/scripts/check_pr_body.py [template]
The body comes from $PR_BODY (the pr-body workflow), else from stdin:
    gh pr view 123 --json body --jq .body | python .github/scripts/check_pr_body.py
Prints one line per finding; exits 1 if any.
"""
from __future__ import annotations

import os
import re
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]
TEMPLATE = ROOT / ".github" / "pull_request_template.md"

COMMENT = re.compile(r"<!--.*?-->", re.DOTALL)
HEADING = re.compile(r"^##\s+(.+?)\s*#*\s*$")
ITEM = re.compile(r"^\s*[-*]\s+\[([ xX])\]\s+(.*\S)\s*$")
NOT_APPLICABLE = re.compile(r"\bn/a\b", re.IGNORECASE)


def sections(text: str) -> list[tuple[str, str]]:
    """The `## ` sections of a markdown text, in order: (heading, its text)."""
    out: list[tuple[str, str]] = []
    for line in text.split("\n"):  # a CRLF body's "\r" goes in the patterns' trailing \s*
        m = HEADING.match(line)
        if m:
            out.append((m.group(1), ""))
        elif out:
            out[-1] = (out[-1][0], out[-1][1] + line + "\n")
    return out


def items(text: str) -> list[tuple[bool, str]]:
    """The checklist items of a section's text: (ticked, their text)."""
    out = []
    for line in COMMENT.sub("", text).split("\n"):
        m = ITEM.match(line)
        if m:
            out.append((m.group(1) != " ", " ".join(m.group(2).split())))
    return out


def check(template: str, body: str) -> list[str]:
    """What the body leaves unanswered of the template, one finding a line."""
    found = {}
    for heading, text in sections(body):
        found.setdefault(heading.casefold(), text)
    errors = []
    for heading, text in sections(template):
        if heading.casefold() not in found:
            errors.append(f"missing the section `## {heading}`")
            continue
        answer = found[heading.casefold()]
        wanted = items(text)
        if not wanted:
            if not COMMENT.sub("", answer).strip():
                errors.append(f"`## {heading}` has nothing but the template's comment: write it")
            continue
        given = items(answer)
        for _, item in wanted:
            errors.extend(answered(heading, item, given))
    return errors


def answered(heading: str, item: str, given: list[tuple[bool, str]]) -> list[str]:
    """A finding unless the body ticks the template's item, or marks it n/a."""
    for ticked, text in given:
        if text.startswith(item):
            if ticked or NOT_APPLICABLE.search(text[len(item):]):
                return []
            return [f"`## {heading}`: tick `{item}`, or end it with n/a"]
    return [f"`## {heading}`: the item `{item}` is missing: keep its text, ticked or marked n/a"]


def main(argv: list[str]) -> int:
    template = Path(argv[1]) if len(argv) > 1 else TEMPLATE
    body = os.environ.get("PR_BODY")
    if body is None:
        body = sys.stdin.read()
    errors = check(template.read_text(encoding="utf-8"), body)
    prefix = "::error::" if os.environ.get("GITHUB_ACTIONS") else ""
    for e in errors:
        print(f"{prefix}PR body: {e}")
    if errors:
        print(f"{prefix}PR body: fill it as {TEMPLATE.relative_to(ROOT).as_posix()} asks")
    return 1 if errors else 0


if __name__ == "__main__":
    sys.exit(main(sys.argv))
