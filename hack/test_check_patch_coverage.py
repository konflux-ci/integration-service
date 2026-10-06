import os
import subprocess
import tempfile
import unittest
from contextlib import chdir, redirect_stdout
from io import StringIO
from pathlib import Path

from check_patch_coverage import (
    changed_lines,
    check,
    coverage_for_file,
    coverage_lines,
    ignored_patterns,
)

SCRIPT = Path(__file__).with_name("check_patch_coverage.py")


def init_repository(repository: Path) -> None:
    subprocess.run(["git", "init", "-q"], cwd=repository, check=True)
    subprocess.run(["git", "config", "user.email", "test@example.com"], cwd=repository, check=True)
    subprocess.run(["git", "config", "user.name", "Test"], cwd=repository, check=True)
    subprocess.run(["git", "config", "commit.gpgsign", "false"], cwd=repository, check=True)
    (repository / "README.md").write_text("fixture\n", encoding="utf-8")
    subprocess.run(["git", "add", "README.md"], cwd=repository, check=True)
    subprocess.run(["git", "commit", "--no-verify", "-qm", "fixture"], cwd=repository, check=True)


class CoverageHelperTest(unittest.TestCase):
    def test_changed_lines_are_compared_from_the_merge_base(self):
        with tempfile.TemporaryDirectory() as directory:
            repository = Path(directory)
            init_repository(repository)
            source = repository / "example.go"
            source.write_text("first := 1\nsecond := 2\n", encoding="utf-8")
            subprocess.run(["git", "add", "example.go"], cwd=repository, check=True)
            subprocess.run(
                ["git", "commit", "--no-verify", "-qm", "base"],
                cwd=repository,
                check=True,
            )
            subprocess.run(["git", "branch", "target"], cwd=repository, check=True)
            subprocess.run(["git", "switch", "-qc", "feature"], cwd=repository, check=True)
            source.write_text("first := 10\nsecond := 2\n", encoding="utf-8")
            subprocess.run(
                ["git", "commit", "--no-verify", "-am", "feature", "-q"],
                cwd=repository,
                check=True,
            )
            subprocess.run(["git", "switch", "-q", "target"], cwd=repository, check=True)
            source.write_text("first := 1\nsecond := 20\n", encoding="utf-8")
            subprocess.run(
                ["git", "commit", "--no-verify", "-am", "target", "-q"],
                cwd=repository,
                check=True,
            )
            subprocess.run(["git", "switch", "-q", "feature"], cwd=repository, check=True)

            with chdir(repository):
                changed = changed_lines("target")

        self.assertEqual(changed["example.go"], {1})

    def test_compound_brace_line_is_counted_as_coverable(self):
        with tempfile.TemporaryDirectory() as directory:
            repository = Path(directory)
            init_repository(repository)
            (repository / "example.go").write_text("} {\n", encoding="utf-8")
            profile = repository / "cover.out"
            profile.write_text("mode: set\nexample.go:1.1,1.4 1 1\n", encoding="utf-8")

            with chdir(repository), redirect_stdout(StringIO()):
                covered, coverable, _ = check("HEAD", profile, repository / "codecov.yml", 85)

        self.assertEqual((covered, coverable), (1, 1))

    def test_brace_only_lines_are_not_counted_as_coverable(self):
        with tempfile.TemporaryDirectory() as directory:
            repository = Path(directory)
            init_repository(repository)
            (repository / "example.go").write_text(
                "package sample\n\nfunc example() {\n\treturn\n}\n",
                encoding="utf-8",
            )
            profile = repository / "cover.out"
            profile.write_text("mode: set\nexample.go:3.1,5.2 1 1\n", encoding="utf-8")

            with chdir(repository), redirect_stdout(StringIO()):
                covered, coverable, _ = check("HEAD", profile, repository / "codecov.yml", 85)

        self.assertEqual((covered, coverable), (2, 2))

    def test_block_comment_lines_are_not_counted_as_coverable(self):
        with tempfile.TemporaryDirectory() as directory:
            repository = Path(directory)
            init_repository(repository)
            (repository / "example.go").write_text(
                "/*\ncomment text\n*/\nvalue := 1\n",
                encoding="utf-8",
            )
            profile = repository / "cover.out"
            profile.write_text(
                "mode: set\n"
                "example.go:1.1,1.3 1 0\n"
                "example.go:2.1,2.13 1 0\n"
                "example.go:3.1,3.3 1 0\n"
                "example.go:4.1,4.11 1 1\n",
                encoding="utf-8",
            )

            with chdir(repository), redirect_stdout(StringIO()):
                covered, coverable, _ = check("HEAD", profile, repository / "codecov.yml", 85)

        self.assertEqual((covered, coverable), (1, 1))

    def test_line_with_covered_and_uncovered_blocks_is_not_fully_covered(self):
        with tempfile.TemporaryDirectory() as directory:
            profile = Path(directory) / "cover.out"
            profile.write_text(
                "mode: set\n"
                "example.go:10.1,10.20 1 1\n"
                "example.go:10.20,12.2 1 0\n",
                encoding="utf-8",
            )

            coverage = coverage_lines(profile)

        self.assertFalse(coverage["example.go"][10])

    def test_overlapping_blocks_keep_the_best_coverage(self):
        with tempfile.TemporaryDirectory() as directory:
            profile = Path(directory) / "cover.out"
            profile.write_text(
                "mode: set\n"
                "example.go:10.1,10.20 1 0\n"
                "example.go:10.1,10.20 1 1\n",
                encoding="utf-8",
            )

            coverage = coverage_lines(profile)

        self.assertTrue(coverage["example.go"][10])

    def test_malformed_coverage_entry_is_rejected(self):
        with tempfile.TemporaryDirectory() as directory:
            profile = Path(directory) / "cover.out"
            profile.write_text(
                "mode: set\nexample.go:not-a-coverage-region\n",
                encoding="utf-8",
            )

            with self.assertRaisesRegex(ValueError, "invalid Go coverage entry"):
                coverage_lines(profile)

    def test_profile_paths_can_be_matched_to_repository_paths(self):
        with tempfile.TemporaryDirectory() as directory:
            profile = Path(directory) / "cover.out"
            profile.write_text(
                "mode: set\n"
                "github.com/konflux-ci/integration-service/status/status.go:10.1,12.3 1 1\n"
                "github.com/konflux-ci/integration-service/status/status.go:20.1,20.2 1 0\n",
                encoding="utf-8",
            )
            coverage = coverage_lines(profile)

        lines = coverage_for_file(coverage, "status/status.go")
        self.assertTrue(lines[10])
        self.assertTrue(lines[12])
        self.assertFalse(lines[20])

    def test_ignore_patterns_are_read_from_codecov_yaml(self):
        with tempfile.TemporaryDirectory() as directory:
            config = Path(directory) / "codecov.yml"
            config.write_text(
                "ignore:\n"
                '  - "vendor/**" # vendored code\n'
                '  - "generated.go"\n'
                "coverage:\n",
                encoding="utf-8",
            )
            self.assertEqual(ignored_patterns(config), ["vendor/**", "generated.go"])

    def test_configured_patch_target_is_used_by_default(self):
        with tempfile.TemporaryDirectory() as directory:
            repository = Path(directory)
            init_repository(repository)
            (repository / "example.go").write_text("value := 1\n", encoding="utf-8")
            (repository / "cover.out").write_text(
                "mode: set\nexample.go:1.1,1.11 1 0\n",
                encoding="utf-8",
            )
            (repository / "codecov.yml").write_text(
                "coverage:\n"
                "  status:\n"
                "    patch:\n"
                "      default:\n"
                "        target: 90%\n",
                encoding="utf-8",
            )

            result = subprocess.run(
                ["python3", str(SCRIPT), "--base", "HEAD"],
                cwd=repository,
                check=False,
                text=True,
                capture_output=True,
            )

        self.assertEqual(result.returncode, 0)
        self.assertIn("below the 90% target", result.stdout)

    def test_go_version_mismatch_with_ci_is_reported(self):
        with tempfile.TemporaryDirectory() as directory:
            repository = Path(directory)
            init_repository(repository)
            (repository / "example.go").write_text("value := 1\n", encoding="utf-8")
            (repository / "cover.out").write_text(
                "mode: set\nexample.go:1.1,1.11 1 1\n",
                encoding="utf-8",
            )
            workflow = repository / ".github" / "workflows" / "pr.yaml"
            workflow.parent.mkdir(parents=True)
            workflow.write_text('go-version: "1.26.0"\n', encoding="utf-8")
            fake_bin = repository / "fake-bin"
            fake_bin.mkdir()
            fake_go = fake_bin / "go"
            fake_go.write_text("#!/bin/sh\nprintf 'go version go1.25.0 linux/amd64\\n'\n", encoding="utf-8")
            fake_go.chmod(0o755)
            environment = os.environ.copy()
            environment["PATH"] = f"{fake_bin}:{environment['PATH']}"

            result = subprocess.run(
                ["python3", str(SCRIPT), "--base", "HEAD"],
                cwd=repository,
                check=False,
                text=True,
                capture_output=True,
                env=environment,
            )

        self.assertEqual(result.returncode, 0)
        self.assertIn("local Go 1.25.0 differs from CI Go 1.26.0", result.stderr)

    def test_profile_older_than_changed_source_is_rejected(self):
        with tempfile.TemporaryDirectory() as directory:
            repository = Path(directory)
            init_repository(repository)
            source = repository / "example.go"
            source.write_text("value := 1\n", encoding="utf-8")
            profile = repository / "cover.out"
            profile.write_text("mode: set\nexample.go:1.1,1.11 1 1\n", encoding="utf-8")
            os.utime(profile, ns=(1_000_000_000, 1_000_000_000))
            os.utime(source, ns=(2_000_000_000, 2_000_000_000))

            result = subprocess.run(
                ["python3", str(SCRIPT), "--base", "HEAD"],
                cwd=repository,
                check=False,
                text=True,
                capture_output=True,
            )

        self.assertEqual(result.returncode, 2)
        self.assertIn("cover.out is older than changed Go source", result.stderr)


if __name__ == "__main__":
    unittest.main()
