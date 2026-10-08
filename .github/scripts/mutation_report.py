#!/usr/bin/env python3
"""The mutation report of a PR, from gremlins' JSON (`gremlins unleash --diff`).

Gremlins mutates only the lines the PR changes. Each LIVED mutant is a change to
the code that no test noticed: the PR adds a test that fails on it, or says why
it changes nothing observable (an equality that adds 0, a sort tie-break, an I/O
error path). No mutant is silenced by a list. Advisory: it never fails.

Usage: python .github/scripts/mutation_report.py gremlins.json [blob-url]
Prints markdown (for $GITHUB_STEP_SUMMARY); blob-url links each line, e.g.
https://github.com/<owner>/<repo>/blob/<sha>.
"""
from __future__ import annotations

import json
import sys
from pathlib import Path

MAX_ROWS = 60


def source_line(path: str, line: int) -> str:
    try:
        lines = Path(path).read_text(encoding="utf-8", errors="replace").splitlines()
        return lines[line - 1].strip() if 0 < line <= len(lines) else ""
    except OSError:
        return ""


def table(rows: list[tuple[str, dict]], blob: str) -> list[str]:
    out = ["| where | mutation | code |", "|---|---|---|"]
    for path, m in rows[:MAX_ROWS]:
        where = f"{path}:{m['line']}"
        if blob:
            where = f"[{where}]({blob}/{path}#L{m['line']})"
        code = source_line(path, m["line"]).replace("|", "\\|").replace("`", "'")
        out.append(f"| {where} | {m['type']} (col {m['column']}) | `{code}` |")
    if len(rows) > MAX_ROWS:
        out.append(f"\n…and {len(rows) - MAX_ROWS} more (the gremlins.json artifact has them all)")
    return out


def report(data: dict, blob: str = "") -> str:
    by_status: dict[str, list[tuple[str, dict]]] = {}
    for f in data.get("files", []):
        for m in f.get("mutations", []):
            by_status.setdefault(m["status"], []).append((f["file_name"], m))
    for rows in by_status.values():
        rows.sort(key=lambda r: (r[0], r[1]["line"], r[1]["column"], r[1]["type"]))
    n = {s: len(by_status.get(s, [])) for s in ("KILLED", "LIVED", "NOT COVERED", "TIMED OUT", "NOT VIABLE")}
    out = ["## Mutation testing of the changed lines", ""]
    if not any(n.values()):
        out.append("No mutable code among the changed Go lines.")
        return "\n".join(out) + "\n"
    tested = n["KILLED"] + n["LIVED"]
    efficacy = f"{100 * n['KILLED'] / tested:.1f}%" if tested else "n/a"
    out.append(f"**{n['KILLED']} killed, {n['LIVED']} lived, {n['NOT COVERED']} on lines no test runs** "
               f"(efficacy {efficacy}; {n['TIMED OUT']} timed out, {n['NOT VIABLE']} didn't compile). Advisory.")
    out.append("")
    out.append("Each lived mutant changes the code without any test failing: add a test that fails on it, "
               "or say in the PR why it changes nothing observable.")
    if n["LIVED"]:
        out += ["", "### Lived", ""] + table(by_status["LIVED"], blob)
    if n["NOT COVERED"]:
        out += ["", "### On lines no test runs", ""] + table(by_status["NOT COVERED"], blob)
    return "\n".join(out) + "\n"


def main(argv: list[str]) -> int:
    if len(argv) < 2:
        print(__doc__)
        return 2
    try:
        data = json.loads(Path(argv[1]).read_text(encoding="utf-8"))
    except (OSError, ValueError) as e:
        print(f"## Mutation testing of the changed lines\n\ngremlins wrote no report ({e}): see the job log.")
        return 0
    sys.stdout.write(report(data, argv[2].rstrip("/") if len(argv) > 2 else ""))
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv))
