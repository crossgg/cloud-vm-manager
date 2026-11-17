#!/usr/bin/env python3
"""Extract release notes for a specific version from CHANGELOG.md."""

from __future__ import annotations

import argparse
import os
import re
import sys
from pathlib import Path
from typing import Optional, Tuple

HEADING_RE = re.compile(r"^##\s+\[?([^\]\s]+)\]?(?:\s+-\s+.*)?\s*$")


def normalize_version(ver: str) -> str:
    return ver.strip().lstrip("vV")


def extract_section(changelog_text: str, target_version: str) -> Tuple[Optional[str], Optional[str]]:
    """Return (section_body, previous_version_heading) for target_version."""
    target_norm = normalize_version(target_version)
    lines = changelog_text.splitlines()

    start_idx: Optional[int] = None
    end_idx: Optional[int] = None
    prev_heading: Optional[str] = None

    for idx, line in enumerate(lines):
        match = HEADING_RE.match(line.strip())
        if not match:
            continue
        heading_ver = match.group(1).strip()
        if start_idx is None:
            if heading_ver.lower() == "unreleased":
                continue
            if normalize_version(heading_ver) == target_norm:
                start_idx = idx + 1
        else:
            end_idx = idx
            if heading_ver.lower() != "unreleased":
                prev_heading = heading_ver
            break

    if start_idx is None:
        return None, None

    if end_idx is None:
        end_idx = len(lines)

    body_lines = lines[start_idx:end_idx]
    body = "\n".join(body_lines).strip()
    if not body:
        return None, None

    return body, prev_heading


def format_compare_tag(version: str) -> str:
    v = version.strip()
    return v if v.startswith(("v", "V")) else f"v{v}"


def write_github_output(output_path: str, found: bool) -> None:
    if not output_path:
        return
    with open(output_path, "a", encoding="utf-8") as fh:
        fh.write(f"found={'true' if found else 'false'}\n")


def main() -> int:
    parser = argparse.ArgumentParser(description="Extract version notes from CHANGELOG.md")
    parser.add_argument("--version", required=True, help="Target version, e.g. v1.2.0")
    parser.add_argument("--changelog", default="CHANGELOG.md", help="Path to CHANGELOG.md")
    parser.add_argument("--output", required=True, help="Path to write extracted release notes")
    parser.add_argument("--repo", default="", help="GitHub owner/repo for Full Changelog compare link")
    parser.add_argument("--prev-tag", default="", help="Fallback previous git tag if not present in CHANGELOG.md")
    parser.add_argument("--github-output", default="", help="Optional GITHUB_OUTPUT path")
    args = parser.parse_args()

    changelog_path = Path(args.changelog)
    if not changelog_path.is_file():
        print(f"Changelog file not found: {changelog_path}", file=sys.stderr)
        write_github_output(args.github_output, False)
        return 0

    text = changelog_path.read_text(encoding="utf-8")
    body, prev_ver = extract_section(text, args.version)
    if not body:
        print(f"No changelog section found for version {args.version}; fallback enabled.")
        write_github_output(args.github_output, False)
        return 0

    compare_prev = prev_ver or args.prev_tag.strip()
    if args.repo and compare_prev and normalize_version(compare_prev) != normalize_version(args.version):
        prev_tag = format_compare_tag(compare_prev)
        curr_tag = format_compare_tag(args.version)
        body = f"{body}\n\n**Full Changelog**: https://github.com/{args.repo}/compare/{prev_tag}...{curr_tag}"

    out_path = Path(args.output)
    out_path.parent.mkdir(parents=True, exist_ok=True)
    out_path.write_text(body + "\n", encoding="utf-8")
    write_github_output(args.github_output, True)
    print(f"Extracted release notes for {args.version} -> {out_path}")
    return 0


if __name__ == "__main__":
    sys.exit(main())
