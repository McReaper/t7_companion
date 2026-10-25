"""Tests for check_pr_body.py: python -m unittest discover -s .github/scripts -p 'test_*.py'"""
from __future__ import annotations

import os
import re
import subprocess
import sys
import unittest
from pathlib import Path

from check_pr_body import COMMENT, TEMPLATE, check

REAL = TEMPLATE.read_text(encoding="utf-8")
SCRIPT = Path(__file__).with_name("check_pr_body.py")

SMALL = """<!-- a note for the author -->

## What failed

<!-- the symptom -->

## Checklist

<!-- tick or n/a -->

- [ ] Tested live
- [ ] No `secrets`
"""


def filled(template: str) -> str:
    """The template answered: each comment replaced by text, each item ticked."""
    return COMMENT.sub("Some text.", template).replace("- [ ]", "- [x]")


class CheckPRBody(unittest.TestCase):
    def test_the_real_template_filled_passes(self):
        self.assertEqual(check(REAL, filled(REAL)), [])

    def test_the_real_template_unfilled_fails_on_every_section_and_item(self):
        errors = check(REAL, REAL)
        sections = re.findall(r"^## ", REAL, re.MULTILINE)
        items = re.findall(r"^- \[ \]", REAL, re.MULTILINE)
        self.assertEqual(len(errors), len(sections) - 1 + len(items))

    def test_an_empty_body_misses_every_section(self):
        self.assertEqual(check(SMALL, ""), [
            "missing the section `## What failed`",
            "missing the section `## Checklist`",
        ])

    def test_a_section_left_to_its_comment_fails(self):
        body = filled(SMALL).replace("## What failed\n\nSome text.", "## What failed\n\n<!-- the symptom -->")
        self.assertEqual(check(SMALL, body), ["`## What failed` has nothing but the template's comment: write it"])

    def test_an_unticked_item_fails(self):
        body = filled(SMALL).replace("- [x] Tested live", "- [ ] Tested live")
        self.assertEqual(check(SMALL, body), ["`## Checklist`: tick `Tested live`, or end it with n/a"])

    def test_an_item_marked_na_passes_unticked(self):
        body = filled(SMALL).replace("- [x] Tested live", "- [ ] Tested live — N/A: docs only")
        self.assertEqual(check(SMALL, body), [])

    def test_na_in_the_items_own_text_does_not_count(self):
        template = SMALL.replace("Tested live", "Tested live, or n/a")
        body = filled(template).replace("- [x] Tested live", "- [ ] Tested live")
        self.assertEqual(check(template, body), ["`## Checklist`: tick `Tested live, or n/a`, or end it with n/a"])

    def test_an_item_with_a_note_after_it_passes(self):
        body = filled(SMALL).replace("- [x] Tested live", "- [X] Tested live on Radiant 1.2.2")
        self.assertEqual(check(SMALL, body), [])

    def test_a_missing_or_reworded_item_fails(self):
        body = filled(SMALL).replace("- [x] No `secrets`\n", "- [x] No secrets\n")
        self.assertEqual(check(SMALL, body), ["`## Checklist`: the item `No `secrets`` is missing: keep its text, ticked or marked n/a"])

    def test_an_item_inside_a_comment_does_not_count(self):
        body = filled(SMALL).replace("- [x] Tested live", "<!--\n- [x] Tested live\n-->")
        self.assertEqual(check(SMALL, body), ["`## Checklist`: the item `Tested live` is missing: keep its text, ticked or marked n/a"])

    def test_crlf_star_bullets_case_and_extra_sections_pass(self):
        body = "## WHAT FAILED\r\n\r\nIt crashed.\r\n\r\n## Notes\r\n\r\nmore\r\n\r\n## Checklist\r\n\r\n* [x] Tested  live\r\n* [x] No `secrets`\r\n- [ ] an item of our own\r\n"
        self.assertEqual(check(SMALL, body), [])

    def test_a_deeper_heading_stays_inside_its_section(self):
        body = filled(SMALL).replace("## What failed\n\nSome text.", "## What failed\n\n### Detail\n")
        self.assertEqual(check(SMALL, body), [])

    def test_a_section_given_twice_is_judged_on_the_first(self):
        body = "## What failed\n\n<!-- -->\n\n" + filled(SMALL).partition("\n\n")[2]
        self.assertEqual(check(SMALL, body), ["`## What failed` has nothing but the template's comment: write it"])


class Main(unittest.TestCase):
    """The script as the pr-body workflow runs it: the body in $PR_BODY, else on stdin."""

    def run_script(self, body: str, env: dict[str, str], via_stdin: bool = False, stdin: str = ""):
        environ = {k: v for k, v in os.environ.items() if k not in ("PR_BODY", "GITHUB_ACTIONS")}
        environ.update(env)
        if not via_stdin:
            environ["PR_BODY"] = body
        return subprocess.run([sys.executable, str(SCRIPT)], input=body if via_stdin else stdin,
                              env=environ, capture_output=True, text=True, encoding="utf-8")

    def test_a_filled_body_exits_0(self):
        done = self.run_script(filled(REAL), {})
        self.assertEqual((done.returncode, done.stdout), (0, ""))

    def test_an_empty_body_exits_1_with_annotations_on_actions(self):
        done = self.run_script("", {"GITHUB_ACTIONS": "true"})
        self.assertEqual(done.returncode, 1)
        lines = done.stdout.splitlines()
        self.assertTrue(lines and all(line.startswith("::error::PR body: ") for line in lines), lines)
        self.assertEqual(lines[-1], "::error::PR body: fill it as .github/pull_request_template.md asks")

    def test_without_pr_body_it_reads_stdin(self):
        done = self.run_script("", {}, via_stdin=True)
        self.assertEqual(done.returncode, 1)
        self.assertTrue(done.stdout.startswith("PR body: missing the section"), done.stdout)
        self.assertEqual(self.run_script(filled(REAL), {}, via_stdin=True).returncode, 0)

    def test_an_empty_pr_body_is_judged_not_replaced_by_stdin(self):
        self.assertEqual(self.run_script("", {}, stdin=filled(REAL)).returncode, 1)


if __name__ == "__main__":
    unittest.main()
