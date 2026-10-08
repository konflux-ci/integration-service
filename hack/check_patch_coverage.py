#!/usr/bin/env python3
"""Estimate Codecov patch coverage from a git diff and Go cover profile."""

from __future__ import annotations

import argparse
import fnmatch
import re
import subprocess
import sys
from collections import defaultdict
from itertools import groupby
from pathlib import Path


HUNK_RE = re.compile(r"^@@ -\d+(?:,\d+)? \+(\d+)(?:,(\d+))? @")
PROFILE_RE = re.compile(
    r"^(?P<file>.+?):(?P<start>\d+)\.(?P<start_column>\d+),"
    r"(?P<end>\d+)\.(?P<end_column>\d+) \d+ (?P<count>\d+)$"
)
GO_FILE_FIX_PATTERNS = (
    re.compile(r"^\s*$"),
    re.compile(r"^\s*//.*$"),
    re.compile(r"^\s*[{}]\s*(//.*)?$"),
    re.compile(r"^\s*func\s*[{]\s*(//.*)?$"),
)
GO_BLOCK_COMMENT_START_RE = re.compile(r"^\s*/\*\s*$")
GO_BLOCK_COMMENT_STOP_RE = re.compile(r"^\s*\*/\s*$")
CI_GO_VERSION_RE = re.compile(r"^\s*go-version:\s*[\"']?([^\"'\s]+)", re.MULTILINE)
LOCAL_GO_VERSION_RE = re.compile(r"\bgo(?P<version>\d+(?:\.\d+)+)\b")


def run_git(args: list[str]) -> str:
    result = subprocess.run(["git", *args], check=False, text=True, capture_output=True)
    if result.returncode != 0:
        raise RuntimeError(result.stderr.strip() or f"git {' '.join(args)} failed")
    return result.stdout


def ci_go_version(workflow: Path) -> str | None:
    if not workflow.exists():
        return None
    match = CI_GO_VERSION_RE.search(workflow.read_text(encoding="utf-8"))
    return match.group(1) if match else None


def local_go_version() -> str | None:
    result = subprocess.run(["go", "version"], check=False, text=True, capture_output=True)
    if result.returncode != 0:
        return None
    match = LOCAL_GO_VERSION_RE.search(result.stdout)
    return match.group("version") if match else None


def warn_for_go_version_mismatch(workflow: Path) -> None:
    expected = ci_go_version(workflow)
    actual = local_go_version()
    if expected and actual and expected != actual:
        print(
            f"Warning: local Go {actual} differs from CI Go {expected}; "
            "Codecov will use CI's coverage report.",
            file=sys.stderr,
        )


def ignored_patterns(path: Path) -> list[str]:
    patterns: list[str] = []
    in_ignore = False
    for raw_line in path.read_text(encoding="utf-8").splitlines():
        line = raw_line.strip()
        if line == "ignore:":
            in_ignore = True
            continue
        if in_ignore and line and not line.startswith("-"):
            break
        if in_ignore and line.startswith("-"):
            value = line[1:].split("#", 1)[0].strip().strip('"\'')
            if value:
                patterns.append(value)
    return patterns


def configured_patch_target(path: Path) -> float | None:
    """Read coverage.status.patch.default.target from codecov.yml."""
    if not path.exists():
        return None

    stack: list[tuple[int, str]] = []
    for raw_line in path.read_text(encoding="utf-8").splitlines():
        content = raw_line.split("#", 1)[0].rstrip()
        if not content.strip() or ":" not in content:
            continue
        indent = len(content) - len(content.lstrip())
        key, value = content.strip().split(":", 1)
        while stack and indent <= stack[-1][0]:
            stack.pop()
        if not value.strip():
            stack.append((indent, key.strip('"\'')))
            continue
        path_keys = [item[1] for item in stack]
        if path_keys == ["coverage", "status", "patch", "default"] and key == "target":
            target = value.strip().strip('"\'').removesuffix("%")
            try:
                return float(target)
            except ValueError:
                return None
    return None


def is_ignored(path: str, patterns: list[str]) -> bool:
    return any(fnmatch.fnmatch(path, pattern) for pattern in patterns)


def changed_lines(base: str) -> dict[str, set[int]]:
    merge_base = run_git(["merge-base", base, "HEAD"]).strip()
    if not merge_base:
        raise RuntimeError(f"git merge-base {base} HEAD returned no commit")
    diff = run_git(["diff", "--unified=0", merge_base, "--", "*.go"])
    changed: dict[str, set[int]] = defaultdict(set)
    current_file: str | None = None
    current_line = 0
    for line in diff.splitlines():
        if line.startswith("+++ b/"):
            current_file = line[6:]
            continue
        if line.startswith("@@"):
            match = HUNK_RE.match(line)
            if match:
                current_line = int(match.group(1))
            continue
        if current_file is None or line.startswith("---"):
            continue
        if line.startswith("+"):
            changed[current_file].add(current_line)
            current_line += 1
        elif line.startswith("-"):
            continue
        else:
            current_line += 1
    for filename in run_git(["ls-files", "--others", "--exclude-standard", "--", "*.go"]).splitlines():
        if filename:
            changed[filename].update(range(1, len(Path(filename).read_text(encoding="utf-8").splitlines()) + 1))
    return changed


def combine_partials(
    partials: set[tuple[int | None, int | None, int]],
) -> list[list[int | None]] | None:
    """Combine Go coverage column ranges using Codecov's best-hit semantics."""
    if len(partials) == 1:
        return [list(next(iter(partials)))]

    columns: dict[int, list[int]] = defaultdict(list)
    for start, end, hits in partials:
        if end is not None:
            for column in range(start or 0, end):
                columns[column].append(hits)

    last_column = (
        max(columns) if columns else max(start or 0 for start, _, _ in partials)
    ) + 1
    end_of_line: list[int] = []
    for start, end, hits in partials:
        if end is None:
            for column in range(start or 0, last_column):
                columns[column].append(hits)
            end_of_line.append(hits)

    merged_columns = [(column, max(hits)) for column, hits in columns.items()]
    grouped_columns = groupby(sorted(merged_columns), lambda item: item[1])
    results: list[list[int | None]] = []
    for hits, grouped in grouped_columns:
        items = list(grouped)
        results.append([items[0][0], items[-1][0] + 1, hits])

    if results:
        first = results[0]
        if first[0] == 0 and first[1] == 1:
            results.pop(0)
            if not results:
                return [[0, None, first[2]]]

        if end_of_line:
            end_hits = max(end_of_line)
            last = results[-1]
            if last[1] == last_column and last[2] == end_hits:
                results[-1] = [last[0], None, end_hits]
            else:
                results.append([last_column, None, end_hits])

    return results or None


def coverage_lines(profile: Path) -> dict[str, dict[int, bool]]:
    partials_by_file: dict[str, dict[int, set[tuple[int | None, int | None, int]]]] = defaultdict(
        lambda: defaultdict(set)
    )
    for raw_line in profile.read_text(encoding="utf-8").splitlines()[1:]:
        if not raw_line:
            continue
        match = PROFILE_RE.match(raw_line)
        if not match:
            if raw_line.rsplit(":", 1)[-1].endswith("%"):
                continue
            raise ValueError(f"invalid Go coverage entry: {raw_line}")
        filename = match.group("file")
        start = int(match.group("start"))
        end = int(match.group("end"))
        start_column = int(match.group("start_column"))
        end_column = int(match.group("end_column"))
        hits = int(match.group("count"))
        lines = partials_by_file[filename]
        if start == end:
            lines[start].add((start_column, end_column, hits))
        else:
            lines[start].add((start_column, None, hits))
            for line_number in range(start + 1, end):
                lines[line_number].add((0, None, hits))
            if end_column > 2:
                lines[end].add((None, end_column, hits))

    coverage: dict[str, dict[int, bool]] = defaultdict(dict)
    for filename, lines in partials_by_file.items():
        for line_number, partials in lines.items():
            best_hit = max(partial[2] for partial in partials)
            combined = combine_partials(partials)
            coverage[filename][line_number] = (
                all(partial[2] > 0 for partial in combined) if combined else best_hit > 0
            )
    return coverage


def coverage_for_file(coverage: dict[str, dict[int, bool]], filename: str) -> dict[int, bool]:
    """Find a profile entry for a repository-relative path."""
    if filename in coverage:
        return coverage[filename]
    suffix = f"/{filename}"
    for profile_name, lines in coverage.items():
        if profile_name.endswith(suffix):
            return lines
    return {}


def is_executable_go_line(line: str) -> bool:
    """Apply the Go line fixes sent by the Codecov CLI uploader."""
    return not any(pattern.match(line) for pattern in GO_FILE_FIX_PATTERNS)


def codecov_ignored_lines(source_lines: list[str]) -> set[int]:
    """Return source lines removed by Codecov's Go uploader file fixes."""
    ignored = {
        line_number
        for line_number, line in enumerate(source_lines, start=1)
        if not is_executable_go_line(line)
        or GO_BLOCK_COMMENT_START_RE.match(line)
        or GO_BLOCK_COMMENT_STOP_RE.match(line)
    }
    starts = [
        line_number
        for line_number, line in enumerate(source_lines, start=1)
        if GO_BLOCK_COMMENT_START_RE.match(line)
    ]
    stops = [
        line_number
        for line_number, line in enumerate(source_lines, start=1)
        if GO_BLOCK_COMMENT_STOP_RE.match(line)
    ]
    for start, stop in zip(starts, stops):
        ignored.update(range(start + 1, stop))
    return ignored


def check(base: str, profile: Path, config: Path, threshold: float) -> tuple[int, int, list[str]]:
    changed = changed_lines(base)
    profile_mtime = profile.stat().st_mtime_ns
    stale_sources = sorted(
        filename
        for filename in changed
        if Path(filename).exists() and Path(filename).stat().st_mtime_ns > profile_mtime
    )
    if stale_sources:
        displayed = ", ".join(stale_sources[:3])
        if len(stale_sources) > 3:
            displayed += f", and {len(stale_sources) - 3} more"
        raise OSError(
            f"{profile} is older than changed Go source ({displayed}); "
            "run 'make test' again"
        )
    coverage = coverage_lines(profile)
    ignored = ignored_patterns(config) if config.exists() else []

    coverable = 0
    covered = 0
    missing: list[str] = []
    for filename, lines in changed.items():
        if is_ignored(filename, ignored):
            continue
        file_coverage = coverage_for_file(coverage, filename)
        source_lines = Path(filename).read_text(encoding="utf-8").splitlines()
        ignored_line_numbers = codecov_ignored_lines(source_lines)
        for line_number in sorted(lines):
            if line_number not in file_coverage:
                continue
            if line_number > len(source_lines) or line_number in ignored_line_numbers:
                continue
            coverable += 1
            if file_coverage[line_number]:
                covered += 1
            else:
                missing.append(f"{filename}:{line_number}")

    if not coverable:
        print("Patch coverage: not applicable (no changed coverable Go lines).")
        return covered, coverable, missing

    percentage = 100 * covered / coverable
    state = "PASS" if percentage >= threshold else "WARN"
    print(
        f"Patch coverage estimate: {percentage:.2f}% "
        f"({covered}/{coverable} coverable changed lines) [{state}]"
    )
    if percentage < threshold:
        print(f"Estimated patch coverage is below the {threshold:g}% target.")
    if missing:
        print("Uncovered changed lines:")
        print("\n".join(f"  - {line}" for line in missing))
        packages = sorted({str(Path(line.rsplit(":", 1)[0]).parent) for line in missing})
        print(
            "Suggested next step: add or extend tests in the affected package(s), "
            "covering the changed happy and error paths."
        )
        print(f"Affected package(s): {', '.join(packages)}")
    return covered, coverable, missing


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--base", default="origin/main", help="git ref to compare against")
    parser.add_argument("--profile", type=Path, default=Path("cover.out"))
    parser.add_argument("--config", type=Path, default=Path("codecov.yml"))
    parser.add_argument(
        "--threshold",
        type=float,
        help="target percentage; defaults to codecov.yml patch target, then 85 if unset",
    )
    parser.add_argument(
        "--fail-under",
        type=float,
        help="return 1 when the estimate is below this value",
    )
    args = parser.parse_args()
    threshold = args.threshold
    if threshold is None:
        configured_target = configured_patch_target(args.config)
        threshold = configured_target if configured_target is not None else 85.0

    try:
        if not args.profile.exists():
            raise OSError(f"{args.profile} not found; run 'make test' from the repository root first")
        warn_for_go_version_mismatch(Path(".github/workflows/pr.yaml"))
        covered, coverable, _ = check(args.base, args.profile, args.config, threshold)
    except (OSError, RuntimeError, ValueError) as error:
        print(f"Patch coverage check could not run: {error}", file=sys.stderr)
        return 2

    if args.fail_under is not None and coverable and 100 * covered / coverable < args.fail_under:
        return 1
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
