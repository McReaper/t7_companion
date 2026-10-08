#!/usr/bin/env python3
"""Coverage of the Go lines a PR changes, against a minimum.

A changed line is one the diff from the base adds or edits in a non-test .go
file; it counts when a statement of a cover profile's block sits on it, and is
covered when any such block ran. The profile comes from
`go test -coverpkg=./... -coverprofile=cover.out ./...`, so a line another
package's tests reach counts as covered. Tests that need the BO3 install or
the real database don't run in CI: a line only they reach is uncovered here,
and wants a test on a fixture.

Usage: python .github/scripts/patch_coverage.py cover.out BASE MIN [blob-url]
Prints markdown (for $GITHUB_STEP_SUMMARY); exits 1 when under MIN percent.
"""
from __future__ import annotations

import re
import subprocess
import sys
from pathlib import Path

MAX_ROWS = 40
HUNK = re.compile(r"^@@ -\d+(?:,\d+)? \+(\d+)(?:,(\d+))? @@")
BLOCK = re.compile(r"^(.+?):(\d+)\.\d+,(\d+)\.\d+ (\d+) (\d+)$")


def changed_lines(base: str) -> dict[str, set[int]]:
    """Lines the diff from the merge base adds or edits, per non-test .go file."""
    diff = subprocess.run(["git", "diff", "-U0", "--no-color", f"{base}...HEAD", "--", "*.go"],
                          capture_output=True, text=True, check=True).stdout
    out: dict[str, set[int]] = {}
    path = ""
    for line in diff.splitlines():
        if line.startswith("+++ "):
            path = line[6:] if line.startswith("+++ b/") else ""
        elif (m := HUNK.match(line)) and path.endswith(".go") and not path.endswith("_test.go"):
            start, count = int(m.group(1)), int(m.group(2) or 1)
            out.setdefault(path, set()).update(range(start, start + count))
    return out


def covered_lines(profile: str, module: str) -> dict[str, dict[int, bool]]:
    """Per file, each line a block's statements sit on, and whether one ran."""
    out: dict[str, dict[int, bool]] = {}
    for line in Path(profile).read_text(encoding="utf-8").splitlines()[1:]:
        m = BLOCK.match(line.strip())
        if not m or int(m.group(4)) == 0:
            continue
        path = m.group(1).removeprefix(module + "/")
        ran = int(m.group(5)) > 0
        lines = out.setdefault(path, {})
        for n in range(int(m.group(2)), int(m.group(3)) + 1):
            lines[n] = lines.get(n, False) or ran
    return out


def module_path() -> str:
    for line in Path("go.mod").read_text(encoding="utf-8").splitlines():
        if line.startswith("module "):
            return line.split()[1]
    raise SystemExit("go.mod has no module line")


def check(changed: dict[str, set[int]], cover: dict[str, dict[int, bool]], minimum: float,
          blob: str = "") -> tuple[bool, str]:
    missed: list[tuple[str, int]] = []
    total = 0
    for path in sorted(changed):
        lines = cover.get(path, {})
        for n in sorted(changed[path]):
            if n in lines:
                total += 1
                if not lines[n]:
                    missed.append((path, n))
    out = ["## Coverage of the changed lines", ""]
    if not total:
        out.append("No changed Go line holds a statement.")
        return True, "\n".join(out) + "\n"
    pct = 100 * (total - len(missed)) / total
    ok = pct >= minimum
    out.append(f"**{total - len(missed)} of {total} changed lines covered ({pct:.1f}%), "
               f"{'at least' if ok else 'under'} the {minimum:g}% minimum.**")
    if missed:
        out += ["", "| uncovered line | code |", "|---|---|"]
        for path, n in missed[:MAX_ROWS]:
            where = f"[{path}:{n}]({blob}/{path}#L{n})" if blob else f"{path}:{n}"
            try:
                code = Path(path).read_text(encoding="utf-8", errors="replace").splitlines()[n - 1].strip()
            except (OSError, IndexError):
                code = ""
            out.append(f"| {where} | `{code.replace('|', chr(92) + '|').replace('`', chr(39))}` |")
        if len(missed) > MAX_ROWS:
            out.append(f"\n…and {len(missed) - MAX_ROWS} more")
    return ok, "\n".join(out) + "\n"


def main(argv: list[str]) -> int:
    if len(argv) < 4:
        print(__doc__)
        return 2
    ok, text = check(changed_lines(argv[2]), covered_lines(argv[1], module_path()), float(argv[3]),
                     argv[4].rstrip("/") if len(argv) > 4 else "")
    sys.stdout.write(text)
    return 0 if ok else 1


if __name__ == "__main__":
    sys.exit(main(sys.argv))
