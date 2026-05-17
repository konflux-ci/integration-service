# Integration Service AI Skills

Repository-specific AI skills for the integration-service Kubernetes operator. These skills are tool-agnostic and can be used with any AI agent (Claude Code, Codex, Goose, etc.) via symlinks to the agent's skill directory.

## Available Skills

| Skill | Description |
|-------|-------------|
| [running-unit-tests](running-unit-tests/SKILL.md) | How to run, write, and troubleshoot unit tests (envtest, Ginkgo, mock loader, coverage) |
| [running-e2e-tests](running-e2e-tests/SKILL.md) | How to build, configure, and run e2e tests against a real cluster (Kind or OpenShift) |
| [pr-definition-of-done](pr-definition-of-done/SKILL.md) | Checklist for PR readiness: commits, code generation, tests, CI checks, documentation |
| [debugging-running-instance](debugging-running-instance/SKILL.md) | How to debug the service on a cluster: logs, probes, metrics, webhooks, env vars |
| [kind-cluster-setup](kind-cluster-setup/SKILL.md) | Local dev environment setup, CRD installation, deployment, and full Konflux stack |
| [ci-cd-quirks](ci-cd-quirks/SKILL.md) | Non-obvious CI/CD behavior: vendoring, hermetic builds, security scans, gotchas |

## Setup for Claude Code

Skills are symlinked from `.claude/skills/` for automatic discovery:

```
.claude/skills/running-unit-tests -> ../../skills/running-unit-tests
.claude/skills/running-e2e-tests -> ../../skills/running-e2e-tests
...
```

## Setup for Other Agents

Create symlinks from your agent's skill directory to `skills/`:

```bash
# Example for Codex
ln -s ../../skills/running-unit-tests .agents/skills/running-unit-tests
```
