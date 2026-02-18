#!/usr/bin/env python3
"""Parse JUnit XML and report test failures.

Outputs:
  1. Formatted table to stdout (visible in the step's log)
  2. ::error annotations (visible in the check run annotations)
  3. Markdown table to $GITHUB_STEP_SUMMARY
  4. PR comment via gh CLI (best-effort, skips on permission errors)

Usage: python3 junit-report.py <junit-xml-path> <heading>
"""
import os
import subprocess
import sys
import xml.etree.ElementTree as ET

def main():
    # Windows terminals default to cp1252 which can't encode characters like ⚠.
    # Force UTF-8 output so test failure details with emoji/symbols render correctly.
    if hasattr(sys.stdout, "reconfigure"):
        sys.stdout.reconfigure(encoding="utf-8", errors="replace")

    if len(sys.argv) < 2:
        print("Usage: junit-report.py <junit.xml> [heading]", file=sys.stderr)
        sys.exit(1)

    xml_path = sys.argv[1]
    heading = sys.argv[2] if len(sys.argv) > 2 else "Test Failures"

    if not os.path.exists(xml_path):
        return

    tree = ET.parse(xml_path)
    failures = []
    for tc in tree.iter("testcase"):
        f = tc.find("failure")
        if f is not None:
            pkg = tc.get("classname", "")
            name = tc.get("name", "")
            # Prefer the failure body for detail, fall back to message attr
            detail = (f.text or f.get("message") or "").strip()
            # Find the first meaningful line (skip === RUN, blank lines, timestamps)
            short = ""
            for line in detail.split("\n"):
                line = line.strip()
                if line and not line.startswith("=== RUN") and not line.startswith("--- FAIL"):
                    short = line[:200]
                    break
            if not short:
                short = f.get("message", "failed")
            failures.append((pkg, name, short, detail))

    if not failures:
        return

    # 1. Formatted output to stdout (visible in step log)
    print(f"\n{'=' * 60}")
    print(f"  {heading}: {len(failures)} failed")
    print(f"{'=' * 60}\n")
    for pkg, name, short, detail in failures:
        print(f"  FAIL  {pkg}.{name}")
        # Indent the detail for readability
        for line in detail.split("\n")[:15]:
            print(f"        {line}")
        print()
    print(f"{'=' * 60}\n")

    # 2. ::error annotations
    for pkg, name, short, _ in failures:
        print(f"::error title=FAIL {pkg}.{name}::{short}")

    # 3. $GITHUB_STEP_SUMMARY
    summary_path = os.environ.get("GITHUB_STEP_SUMMARY", "")
    if summary_path:
        with open(summary_path, "a", encoding="utf-8", errors="replace") as out:
            out.write(f"## {heading}\n\n")
            out.write("| Package | Test | Error |\n")
            out.write("|---------|------|-------|\n")
            for pkg, name, short, _ in failures:
                # Escape pipes in the message for markdown tables
                safe = short.replace("|", "\\|")
                out.write(f"| `{pkg}` | `{name}` | {safe} |\n")

    # 4. PR comment (best-effort)
    pr_number = os.environ.get("PR_NUMBER", "")
    if not pr_number:
        return

    comment_marker = f"<!-- junit-report: {heading} -->"
    body_lines = [
        comment_marker,
        f"## {heading}",
        "",
        "| Package | Test | Error |",
        "|---------|------|-------|",
    ]
    for pkg, name, short, _ in failures:
        safe = short.replace("|", "\\|")
        body_lines.append(f"| `{pkg}` | `{name}` | {safe} |")
    body_lines.append("")
    body_lines.append("_Updated by CI — this comment is replaced on each push._")
    body = "\n".join(body_lines)

    try:
        # Check for existing comment to update
        result = subprocess.run(
            ["gh", "pr", "view", pr_number, "--json", "comments",
             "--jq", f'.comments[] | select(.body | startswith("{comment_marker}")) | .url'],
            capture_output=True, text=True, timeout=15,
        )
        existing_url = result.stdout.strip().split("\n")[0] if result.stdout.strip() else ""

        if existing_url:
            # Extract comment ID from URL and update
            comment_id = existing_url.rstrip("/").split("/")[-1]
            subprocess.run(
                ["gh", "api", f"repos/{{owner}}/{{repo}}/issues/comments/{comment_id}",
                 "-X", "PATCH", "-f", f"body={body}"],
                capture_output=True, timeout=15,
            )
        else:
            subprocess.run(
                ["gh", "pr", "comment", pr_number, "--body", body],
                capture_output=True, timeout=15,
            )
    except Exception:
        pass  # Best-effort — don't fail the step

    # Exit non-zero so the step shows as failed (red X) in the UI
    sys.exit(1)

if __name__ == "__main__":
    main()
