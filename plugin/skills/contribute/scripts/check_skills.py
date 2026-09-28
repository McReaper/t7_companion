#!/usr/bin/env python3
"""Validate the t7kb plugin's skills against the Agent Skills spec and the house rules.

Usage:
    python check_skills.py [SKILLS_DIR]

SKILLS_DIR defaults to the skills directory this script ships in (two levels up:
<skills>/contribute/scripts/check_skills.py), so it works both from a repo clone
(plugin/skills/) and from an installed plugin copy. Standard library only.

Prints one line per finding as `path:line: ERROR|WARN: message` and exits 1 if
any ERROR was found, 0 otherwise. Warnings never fail the run.
"""
from __future__ import annotations

import json
import re
import sys
from pathlib import Path

# Hard limits from the Agent Skills spec / Anthropic's authoring guidance.
NAME_MAX = 64
DESC_MAX = 1024          # characters, not bytes: descriptions are full of em-dashes
BODY_MAX_LINES = 500
# House aims (contribute/SKILL.md): past these the skill wants a references/ split.
BODY_AIM_WORDS = 3000
DESC_SMELL = 1005        # optimised to the ceiling rather than to the reader
REF_TOC_LINES = 100      # reference files longer than this need a table of contents

NAME_RE = re.compile(r"^[a-z0-9]+(-[a-z0-9]+)*$")
XML_TAG_RE = re.compile(r"<[A-Za-z/!?][^<>]*>")
SECOND_PERSON_RE = re.compile(r"\b(you|your|you're|I|I'm|we)\b")
REF_LINK_RE = re.compile(r"references/[\w.-]+\.md")
PERSONAL_PATH_RE = re.compile(r"(?i)(?:[A-Z]:\\+Users\\+|/Users/|/home/)(?!<)[A-Za-z0-9._-]+")
CROSS_REF_RE = re.compile(r"\bt7kb:([a-z0-9-]+)")
TOOL_NAMES = {"search", "get", "build", "setup"}   # t7kb:search etc. are MCP tools / the setup skill

findings: list[tuple[str, int, str, str]] = []


def report(path: Path, line: int, level: str, msg: str) -> None:
    findings.append((str(path), line, level, msg))


def parse_frontmatter(text: str):
    """Return (fields, end_line_index, raw_lines) for simple one-line `key: value` frontmatter."""
    lines = text.splitlines()
    if not lines or lines[0].strip() != "---":
        return None, 0, lines
    fields: dict[str, tuple[str, int]] = {}
    for i in range(1, len(lines)):
        if lines[i].strip() == "---":
            return fields, i, lines
        m = re.match(r"^([A-Za-z_-]+):\s*(.*)$", lines[i])
        if m:
            fields[m.group(1)] = (m.group(2), i + 1)
    return None, 0, lines


def check_yaml_plain(path: Path, key: str, value: str, line: int) -> None:
    """Strict YAML parsers reject these in an unquoted (plain) scalar."""
    if value[:1] in "\"'":
        return
    if ": " in value:
        report(path, line, "ERROR", f"`{key}` contains ': ' -- invalid in an unquoted YAML value; rephrase or quote it")
    if " #" in value:
        report(path, line, "ERROR", f"`{key}` contains ' #' -- starts a YAML comment in an unquoted value")
    if value[:1] in "[]{}>|*&!%@`,?-":
        report(path, line, "ERROR", f"`{key}` starts with a YAML indicator character {value[:1]!r}")


def check_skill(skill_md: Path, known: set[str]) -> None:
    text = skill_md.read_text(encoding="utf-8")
    fields, end, lines = parse_frontmatter(text)
    if fields is None:
        report(skill_md, 1, "ERROR", "missing or unterminated `---` frontmatter")
        return
    dirname = skill_md.parent.name

    name, nline = fields.get("name", ("", 1))
    if not name:
        report(skill_md, 1, "ERROR", "frontmatter has no `name`")
    else:
        if not NAME_RE.match(name) or len(name) > NAME_MAX:
            report(skill_md, nline, "ERROR", f"name {name!r} must be lowercase letters/digits/single hyphens, <= {NAME_MAX} chars")
        if re.search(r"anthropic|claude", name):
            report(skill_md, nline, "ERROR", "name contains a reserved word (anthropic/claude)")
        if name != dirname:
            report(skill_md, nline, "ERROR", f"name {name!r} must match its directory {dirname!r} (Agent Skills spec; Claude Code lists plugin skills by directory)")
        check_yaml_plain(skill_md, "name", name, nline)

    desc, dline = fields.get("description", ("", 1))
    if not desc.strip():
        report(skill_md, 1, "ERROR", "frontmatter has no `description`")
    else:
        n = len(desc.strip())
        if n > DESC_MAX:
            report(skill_md, dline, "ERROR", f"description is {n} characters (limit {DESC_MAX})")
        elif n > DESC_SMELL:
            report(skill_md, dline, "WARN", f"description is {n} characters -- right at the ceiling; trim connectives, keep triggers")
        tags = XML_TAG_RE.findall(desc)
        if tags:
            report(skill_md, dline, "ERROR", f"description contains XML-like tags {tags[:4]} -- use {{name}} or ... for placeholders")
        pov = SECOND_PERSON_RE.findall(desc)
        if pov:
            report(skill_md, dline, "WARN", f"description should be third person (found {sorted(set(pov))})")
        check_yaml_plain(skill_md, "description", desc, dline)

    body_lines = lines[end + 1:]
    body = "\n".join(body_lines)
    if len(body_lines) > BODY_MAX_LINES:
        report(skill_md, end + 2, "ERROR", f"body is {len(body_lines)} lines (limit {BODY_MAX_LINES}) -- split into references/")
    words = len(body.split())
    if words > BODY_AIM_WORDS:
        report(skill_md, end + 2, "WARN", f"body is {words} words (aim < {BODY_AIM_WORDS}) -- move secondary detail into references/")
    if dirname.startswith("bo3-") or name not in {"setup", "contribute"}:
        if "## Don't invent" not in body:
            report(skill_md, end + 2, "WARN", "domain skill has no closing `## Don't invent` section")

    # references: every file linked from SKILL.md, none linking onward, TOC past 100 lines
    ref_dir = skill_md.parent / "references"
    linked = set(REF_LINK_RE.findall(body))
    if ref_dir.is_dir():
        for ref in sorted(ref_dir.glob("*.md")):
            rel = f"references/{ref.name}"
            if rel not in linked:
                report(ref, 1, "ERROR", f"{rel} is not linked from SKILL.md (references must be one level deep from it)")
            rtext = ref.read_text(encoding="utf-8")
            rlines = rtext.splitlines()
            onward = set(REF_LINK_RE.findall(rtext)) - {rel}
            if onward:
                report(ref, 1, "ERROR", f"links to other reference files {sorted(onward)} -- keep references one level deep")
            head = "\n".join(rlines[:40])
            if len(rlines) > REF_TOC_LINES and not re.search(r"(?im)^(## )?contents", head):
                report(ref, 1, "ERROR", f"{len(rlines)} lines but no table of contents near the top")
            if "SKILL.md" not in head:
                report(ref, 1, "WARN", "no backlink telling the reader to load SKILL.md first")
            check_common(ref, rtext, known)
    for rel in sorted(linked):
        if not (skill_md.parent / rel).is_file():
            report(skill_md, 1, "ERROR", f"links {rel}, which does not exist")
    check_common(skill_md, text, known)


def check_common(path: Path, text: str, known: set[str]) -> None:
    in_code = False
    prev_prose = False
    lines = text.splitlines()
    start = 0
    if lines and lines[0].strip() == "---":   # skip frontmatter
        for j in range(1, len(lines)):
            if lines[j].strip() == "---":
                start = j + 1
                break
    for i, line in enumerate(lines, 1):
        if i <= start:
            continue
        if line.lstrip().startswith("```"):
            in_code = not in_code
            prev_prose = False
            continue
        if in_code:
            continue
        for m in PERSONAL_PATH_RE.finditer(line):
            report(path, i, "WARN", f"looks like a personal path ({m.group(0)}) -- use <you> / <bo3_root>")
        for ref in CROSS_REF_RE.findall(line):
            if ref not in known and ref not in TOOL_NAMES:
                report(path, i, "ERROR", f"t7kb:{ref} names no skill in this plugin")
        # hard-wrap heuristic: a prose line continued by a line starting lowercase
        stripped = line.strip()
        is_prose = bool(stripped) and not re.match(r"^([#>|*\-+]|\d+\.|<|---)", stripped)
        if prev_prose and is_prose and stripped[:1].islower():
            report(path, i, "WARN", "looks hard-wrapped -- one line per paragraph (repo rule)")
        prev_prose = is_prose


def check_router(skills_dir: Path, names: set[str]) -> None:
    """knowledge-base must name every other domain skill (the mandatory follow-up)."""
    router = skills_dir / "knowledge-base" / "SKILL.md"
    if not router.is_file():
        return
    fields, _, _ = parse_frontmatter(router.read_text(encoding="utf-8"))
    desc = (fields or {}).get("description", ("", 1))[0]
    for n in sorted(names - {"knowledge-base", "setup", "contribute"}):
        if f"t7kb:{n}" not in desc:
            report(router, 3, "ERROR", f"knowledge-base's description doesn't route to t7kb:{n}")


def check_marketplace(skills_dir: Path) -> None:
    mp = skills_dir.parent.parent / ".claude-plugin" / "marketplace.json"
    if not mp.is_file():
        return
    try:
        json.loads(mp.read_text(encoding="utf-8"))
    except json.JSONDecodeError as e:
        report(mp, e.lineno, "ERROR", f"invalid JSON: {e.msg}")


def main() -> int:
    try:  # a Windows console defaults to cp1252; never crash on an em-dash or arrow
        sys.stdout.reconfigure(encoding="utf-8", errors="replace")
    except (AttributeError, ValueError):
        pass
    skills_dir = Path(sys.argv[1]) if len(sys.argv) > 1 else Path(__file__).resolve().parents[2]
    skill_files = sorted(skills_dir.glob("*/SKILL.md"))
    if not skill_files:
        print(f"no */SKILL.md under {skills_dir}", file=sys.stderr)
        return 1
    names = {p.parent.name for p in skill_files}
    for p in skill_files:
        check_skill(p, names)
    check_router(skills_dir, names)
    check_marketplace(skills_dir)
    for path, line, level, msg in findings:
        print(f"{path}:{line}: {level}: {msg}")
    errors = sum(1 for f in findings if f[2] == "ERROR")
    warns = len(findings) - errors
    print(f"{len(skill_files)} skills checked: {errors} error(s), {warns} warning(s)")
    return 1 if errors else 0


if __name__ == "__main__":
    sys.exit(main())
