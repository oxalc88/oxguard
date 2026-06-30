# Changelog

All notable changes to oxguard are documented here.
Format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).

## [0.5.2] — 2026-06-30

### Changed

- **tsguard — FTA gate excludes conventional test files by default** — `*.test.*`,
  `*.spec.*`, and directories `__tests__/`, `__mocks__/`, `__fixtures__/` are excluded
  from FTA scoring. Test files are intentionally repetitive; the maintainability index
  doesn't apply to them. Lint and coverage gates still enforce quality on test code.
  Project-root `fta.json` exclusions are merged in automatically. Configure per-project
  in `oxguard.toml`:

  ```toml
  fta-exclude-tests = false          # re-enable FTA scoring on test files
  fta-exclude = ["*.pbt.ts"]         # add project-specific patterns
  ```

- **pyguard — complexity gates exclude conventional test files by default** — `test_*.py`,
  `*_test.py`, `conftest.py`, and the `tests/` directory are excluded from radon cc/mi/hal,
  Halstead, and type-annotation complexity gates. Ruff and coverage still gate test code.
  Configure per-project in `pyproject.toml`:

  ```toml
  [tool.pyguard]
  exclude-tests = false          # re-enable complexity gating on test files
  exclude = ["*/fixtures/*"]     # add project-specific patterns
  ```

## [0.5.0] — 2026-06-30

### Added

- **Multi-tool skill deployment** — `tsguard setup` / `pyguard setup` now writes both a
  hook config *and* an AI skill or instructions file for each selected tool:
  - Claude Code → `.claude/skills/{tool}/SKILL.md` (full 8-step interpret-and-fix skill)
  - Cursor → `.cursor/rules/{tool}.mdc` (agent rule triggered on file edit)
  - GitHub Copilot → `.github/copilot-instructions.md` (appended block, idempotent)
  - Codex CLI → `AGENTS.md` (project-level instructions with all gate commands)
  - OpenCode → `.opencode/plugins/{tool}/index.ts` + `opencode.json` (plugin)
  - Kiro → `.kiro/agents/{tool}.json` (agent definition with prompt + postToolUse hook)
- **Skill source files in binary** — skill content for all 6 tools is embedded in the
  binary at build time (`//go:embed skill/`), so deployed files are always in sync with
  the installed version.
- **`oxguard.toml` config file** (tsguard) — projects can set `dirs`, `exclude`,
  `fta-score-cap`, and `timeout` at the repo root instead of passing CLI flags every time.
  CLI flags still take precedence. See `tsguard --help` for the merge order.

### Changed

- **tsguard security stack** — replaced Python-dependent `detect-secrets` + `semgrep`
  with fully Node-native tools. Zero Python required on the dev machine:
  - Secrets: `secretlint` + `@secretlint/secretlint-rule-preset-recommend` (npm devDep)
  - CVEs: PM-native `npm audit` / `pnpm audit` / `yarn audit` (informational) +
    `audit-ci` with threshold config (hard gate)
  - SAST: **Opengrep** — a self-contained project-local binary (~50 MB, LGPL-2.1,
    bundles its own runtime). Downloaded by `tsguard setup` into
    `node_modules/.cache/oxguard/opengrep`. No pip, no uv, no global writes.
    Prints `[SKIP]` if binary is absent; run `tsguard setup` to download.
- **Dirs scoping** — `--dirs` and `--exclude` flags are now wired into every gate
  (lint, FTA, types, coverage, security), not just security. Default excludes now
  include `.agents`, `.claude`, `.opencode`, `.kiro`, `skills` so AI tool config
  directories are never scanned.
- **PM audit is now informational** — PM-native `npm audit` output is advisory; `audit-ci`
  is the hard gate (configurable via `.auditcirc.json`).

### Fixed

- tsguard: HTTP timeout added to Opengrep binary download (no more silent hang on slow
  connections).
- tsguard: lockfile detection now walks parent directories, so monorepos with a root
  lockfile are detected correctly from a sub-package.
- tsguard: `vitest` and `@vitest/coverage-v8` major version mismatch now fails with a
  clear error instead of a silent coverage skip.
- tsguard: `secretlint` now receives positional directory args instead of a glob pattern,
  fixing false negatives on some project layouts.

### Removed

- tsguard: `detect-secrets` and `semgrep` are no longer installed or required. Projects
  that had these in devDependencies can remove them.
- pyguard: `semgrep` removed from required uv dev deps (it was listed but never invoked).
- `install.sh`: removed orphaned `~/.local/share/oxguard` analysis copy (the binary never
  read it; analysis scripts are deployed per-project by `pyguard setup`).

## [0.4.2] — 2026-06-07

- fix(tsguard): walk parent directories when detecting package manager lockfiles

## [0.4.1] — 2026-06-06

- fix(tsguard): validate vitest and coverage-v8 major version alignment
