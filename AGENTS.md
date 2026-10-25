# Working here

1. Branch from an up-to-date `origin/main` in a separate worktree. Never push with `--force` and never commit to `main` directly.
2. Commit with the repository identity (see `docs/guides/git-identity.md`).
3. Before opening a PR, run the full gate: `go vet` and `go test` in `contract`, `tsguard` and `pyguard`; `node --test evals/evals.test.cjs` and the harness tests; `python3 scripts/test-identity-hooks.py`; `python3 scripts/test-release-docs.py`; `node --test npm/distribution.test.cjs`; and `node evals/agent-actions.cjs` (0 fail, 0 stale).
4. Open the PR with `--base main`. Merge with a merge commit, and only when every CI check is green. If anything fails, fix it; do not merge.

# Model selection

Select the model by role, not by provider. The implementer and the reviewer must be separate sessions. Luna (`gpt-6-luna`) is not used.

| Period | Implements | Reviews |
|---|---|---|
| Until 2026-10-30 | `mimo-v2.6-flash` through `pi` | Codex Sol (`gpt-6.1-sol`) |
| From 2026-10-31 | Codex Sol (`gpt-6.1-sol`) | Codex Sol, in a separate session |

- Until 2026-10-30, if `mimo-v2.6-flash` fails verification two times, escalate to `glm5.3-flash`; if that also fails two times, escalate to Sol.
- Sol starts at effort `medium`. When verification fails, raise the effort one step: `medium` → `high` → `xhigh` → `max`.
- Record the actual agent, provider, model and effort in trace provenance.

## Transition on 2026-10-31

- NaN Builders models (through `pi`) are available until 2026-10-30 only.
- From 2026-10-31, use Codex only. Do not start `pi`, OMP, OpenCode or Grok routes that use NaN models. Do not set `NAN_API_KEY`.

# Agent traces

If you change a skill file, re-record the affected sessions with fresh agents (`docs/guides/oxguard-evals.md#refresh-skill-evidence`); never update only hashes. On a PR, a stale trace is a warning; on a release it blocks.

# Releases

Follow `docs/guides/releases.md`.

# Document formatting

Keep each prose paragraph on one logical line. Do not insert manual line breaks to wrap prose at a fixed column width. Preserve meaningful line breaks in headings, lists, tables and code blocks.
