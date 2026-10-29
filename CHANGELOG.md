# Changelog

All notable changes to oxguard are documented here.
Format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).

## [Unreleased]

### Changed

- Tsguard and PyGuard pin `golang.org/x/sys` v0.49.0 (was v0.42.0) and `golang.org/x/term` v0.47.0 (was v0.41.0). The bump resolves advisory GO-2026-5024 (CVE-2026-39824: `NewNTUnicodeString` in `golang.org/x/sys/windows` truncates strings longer than the 16-bit `NTUnicodeString` size instead of failing), which is fixed upstream in v0.44.0. The advisory is Windows-only and not reachable from our code, which never calls `NewNTUnicodeString`. `tsguard/go.mod` raises its `go` directive to 1.26.0 because these releases require it, matching `pyguard`; no Go source changed.
- Python evaluation toolchain pins pyan3 2.9.0 (was 1.2.0). PyGuard's counting policy is unchanged: absolute-path inputs, distinct callers over pyan3 function-use edges, and Node `defined`/`filename`/`ast_node` filtering with `nodes`/`uses_edges` access. Upstream 2.x resolution changed: on `criticality-cross` it resolves function-to-function use edges that 1.2.0 left unresolved (2 there, 0 before), and parameter-annotation resolution can change caller counts — so symbols, edges and rankings can differ between the pins. 2.x also infers the analysis root from the given files (correct dotted module names for namespace packages such as `src`/`lib` without `__init__.py`) and drops files it cannot parse instead of failing the run; see Fixed below. (#23)
- Python evaluation toolchain pins mypy 2.4.0 (was 1.18.2). PyGuard's mypy integration is unchanged for the normal exit 0/1 paths: native `--output json` lines with native rule codes, exit 0 for a clean analysis and exit 1 for blocking findings. mypy 2.x adds `end_line` and `end_column` to each JSON line (`hint` was already emitted by 1.18.2), and its own defaults (`--local-partial-types`, `--strict-bytes`, native parser) can change which findings a project reports. (#23)
- Python evaluation toolchain pins pytest 9.1.1 (was 8.4.2); pytest-cov stays 7.1.0 and is compatible. PyGuard's pytest integration is unchanged and verified on 9.x: `pytest --cov --cov-fail-under=80` with `--cov-report=json` and `--junitxml` under `-o junit_family=xunit1`, the exit codes 0–5 (5 for no tests collected), native coverage JSON totals and JUnit file/line failure evidence all behave as on 8.x. (#23)
- The pytest 9.1.1 pin resolves advisory PYSEC-2026-1845 (CVE-2025-71176, untrusted `/tmp/pytest-of-{user}` directory handling, fixed upstream in 9.0.3) in the evaluation toolchain. (#23)
- Python evaluation toolchain pins certifi 2026.10.23 (was 2026.7.22) and filelock 4.2.4 (was 4.0.10); regenerating the hash-pinned lockfile also moves the implicit pip pin to 26.2.2 (was 26.2.1). The certifi bump resolves PYSEC-2026-4320 (revoked eMudhra intermediate CA certificates chaining to the bundled emSign ECC Root CA - G3), and filelock 4.2.4 resolves CVE-2026-112940, CVE-2026-112913, CVE-2026-112907, CVE-2026-112908, CVE-2026-112909, CVE-2026-112910, CVE-2026-112911, CVE-2026-112912, CVE-2026-112915, CVE-2026-112934, CVE-2026-112935, CVE-2026-112938 and CVE-2026-112939, all fixed upstream in 4.2.1. `pip-audit -r evals/python/requirements.txt --no-deps` reports no known vulnerabilities.

### Fixed

- PyGuard's criticality analysis reports source files skipped by pyan3 as an explicit failure instead of a quiet partial ranking. pyan3 1.2.0 raised on a syntax error; 2.9.0 only logs a warning and drops the file from `visitor.filenames`, which could previously exit 0 with functions missing from the ranking. The analyzer now compares the requested file set with `visitor.filenames` (same strings, so no path-normalization false positives) and fails with `adapter_failure`: the gate is `error`, the assessment `incomplete`, no `CRITICALITY.md` is registered, and the `analyze_criticality.py` helper exits 1 with the `adapter_failure` payload; the advisory `pyguard criticality` gate keeps exit 0 with gate status `error` and assessment `incomplete`. (#23)
- Test-runner detection recognizes pytest 9 configuration files: a `pytest.toml`/`.pytest.toml` `[pytest]` table and the `pyproject.toml` `[tool.pytest]` table (alongside the legacy `[tool.pytest.ini_options]`) now select pytest for the coverage gate. Projects that keep tests outside `tests/`/`test/` and migrated off `pytest.ini` are no longer reported as having no test runner. (#23)
- PyGuard reports mypy syntax aborts as structured evidence instead of a configuration failure. mypy 2.x writes native JSON diagnostics and exits 2 when errors prevent further checking; PyGuard keeps those lines as blocking `pyguard.mypy.<code>` findings (for example `pyguard.mypy.syntax`) with file, line, column and message, and adds a `pyguard.execution.analyzer_failure` execution error. The mypy gate is `error`, the assessment `incomplete`, and the run exits 1, so an aborted check can never pass. Exit 2 with empty or non-JSON stdout still reports `invalid_configuration`. (#23)

## [0.10.1] — 2026-10-24

**Upgrade note:** `tsguard change` now exits 1 when its assessment is incomplete. CI jobs that previously passed with an incomplete `change` assessment will now fail; see Changed below.

### Fixed

- Tsguard change: cognitive measurement no longer aborts with `Unexpected cognitive diagnostic` when user `biome-ignore` comments for other rules become unused, malformed or misplaced under the isolated cognitive config (`suppressions/unused`, `suppressions/parse`, `suppressions/incorrect`). User code and thresholds are untouched; other unexpected diagnostics still fail. `biome-ignore-start` cognitive suppressions now mark the measurement partial. Same failure existed in 0.9.0. (#27)

### Changed

- Tsguard `change` now exits 1 when its assessment is incomplete (missing/unresolvable baseline, analyzer error, partial measurement), consistent with `diagnose`. Advisory findings alone still exit 0; `check` is unaffected. (#27)
- `AGENTS.md` selects agent models by role, with a NaN transition rule. (#26)

## [0.10.0] — 2026-10-24

### Added

- `tsguard diagnose` and `pyguard diagnose` continue independent analyses after a failure and report per-analysis execution coverage, so failure investigations see every finding at once. Existing fail-fast commands and thresholds are unchanged.

### Changed

- Failure investigations in the Tsguard and PyGuard skills route through `diagnose`, and omitted findings are re-run with `--output json` first. The skills are split into a short router plus task workflows (`check`, `fix`, `security`, `review`) within the existing 700-word bundle limit.
- Agent-trace evidence uses schema 2 with per-session skill-file hashes: stale traces warn on pull requests and block releases (`OXGUARD_REQUIRE_CURRENT_TRACES`). Review and focused traces were re-recorded with fresh Codex agents.
- `AGENTS.md` is the single entry point for the contributor process; obsolete plans, evaluation snapshots and release history were removed from the docs.

### Fixed

- `diagnose` guards empty tool argument lists and exports Radon excludes once.

## [0.9.0] — 2026-10-16

Changes released in 0.8.0 (2026-10-10), 0.8.1 (2026-10-15) and 0.9.0 (2026-10-16). They were not cut into separate sections at the time.

### Added

- Release-tag version labels and `VERSION` metadata for packaged documentation
  and deployed skill bundles. Release archives and the main npm package include
  tagged docs; generated links target the matching Git tag. Hook redeployment
  refreshes skill versions after CLI upgrades. Pre-publication smoke checks reject
  missing/stale documentation metadata without rewriting repository source docs.

- Versioned zero-config Tsguard lint policy using Ultracite/Biome, preserving
  project syntax-lint and formatting policy.
- Advisory resolved delegation/handler smells and static module graph
  measurements, including cycles, fan-in/out, SCC depth and coupling.
- Explicit inert Git baseline comparisons for supported complexity displacement.
  Missing baselines remain nonblocking and not evaluated.
- Installed maintainability regression cases and explicit new Python parity
  gaps. Existing Level 1 checks and fail-fast behavior remain protected.
- Native `typed-lint` using pinned Oxlint/tsgolint semantic rules only: unsafe
  assignments/assertions and promise misuse block; unnecessary conditions are
  advisory. Biome retains syntax lint and formatting. Unsupported TypeScript 7
  configurations, unresolved types and scope limits remain incomplete.
- Advisory silent catch exits, literal promise rejection fallbacks and dropped
  rejected `allSettled` results in supported symbol-resolved `flatMap` paths.
- Async single-return forwarding modes and per-function branch nesting/source
  locations; structured FTA failures include first-file complexity context.
- Review-skill guidance reads the target project's `AGENTS.md` and separates
  analyzer findings from helper, failure-policy and logging-owner judgments.

### Changed

- Remove stale v0.8 candidate labels from current guides. Explain matching
  installed binaries, embedded skills and repository documentation through the
  same release tag; preserve historical plan version names.

- Split Tsguard and PyGuard consumer skills into short task routers, four
  workflows, capped at 700 words per complete language bundle. Remove OxGuard release procedures
  from consumer instructions. Embed and deploy the complete bundles for Claude,
  Codex and Copilot, with project instruction pointers and existing-file protection.
  Compare old/new skills with fresh agent actions; fingerprint every task guide so stale captures fail the existing readiness gate.

- Refresh the canonical Tsguard and PyGuard skills for independent security
  scans, reviewed dependency exposure, strict result states and verified-input
  reuse. Remove separately maintained Cursor rules and their deployment; retain
  optional Cursor execution hooks and preserve existing project rule files.
  Refresh real agent captures and cover the new guidance in the existing evals.
- Update Secretlint and its recommended preset 13.0.6 → 13.0.7; Oxlint
  1.87.0 → 1.88.0 with oxlint-tsgolint 7.0.2003 → 7.0.2004; FTA
  3.0.1 → 3.1.0; Knip 6.38.0 → 6.41.0; and jscpd 5.3.2 → 5.4.1.
  Preserve Node support, analyzer policy and existing quality thresholds.
- Upgrade paired Vitest and coverage-v8 from 5.0.2 to 5.0.4; correct the
  packed-consumer test fixture to use the bundled Vitest globals through
  `globalThis` without declaring Vitest as a project dependency. Keep the
  fixture Biome-formatted, with an idempotent fix check, and verify that
  Ultracite rejects an undeclared dependency without disabling the rule.
- Upgrade the pinned npm lint toolchain to Ultracite 8.0.1 and Biome 2.5.15.
  Preserve the existing Biome preset, project-selected lint configurations,
  cognitive complexity limit of 20, schema-1 output and advisory behavior.
  Raise supported Node.js 22.x to 22.18+ as required by Ultracite 8;
  keep Node.js 24.x and 26+ support. Add package and runtime compatibility tests.
  This does not resolve the transitive `fast-glob → micromatch → braces`
  vulnerability (CVE-2026-93687); dependency acceptance remains blocked until
  that path is removed or an approved mitigation is documented.
- `check` includes applicable maintainability, duplication and change
  analysis through the existing schema-1 contract. Updated agent guidance explains
  advisory evidence and incomplete assessments.
- Tsguard maintainability readiness is independent of deferred new Python
  capabilities. Its blocking order now includes typed lint after TypeScript
  compilation; native semantic compatibility limits are documented explicitly.

## [0.7.2] — 2026-10-09

### Fixed

- Release validation now reads the requested tag from the dispatch input or
  pushed ref before propagating the verified tag and commit to downstream jobs.
  The `v0.7.1` attempt failed before publishing any release or npm packages.
- Standalone Tsguard verifies pinned Opengrep binaries in the OS user cache
  outside the scanned project. Missing or untrusted SAST engines block checks.
- Root-wide SAST passes explicit project targets without `node_modules` or
  `.git`, avoiding scanner discovery stalls in Windows pnpm dependency trees.
- PyGuard executes embedded analysis helpers from private temporary copies
  instead of trusting project-local `tools/analysis` files. Secrets scanning
  receives the requested directory scope.
- Generated agent hooks and skills stay inside the selected project and use
  trusted command references.
- Bundled agent skills and installation documentation describe the hardened
  engine and helper behavior.

### Removed

- Obsolete Kiro hook generators and agent templates. Supported integrations
  remain documented in the README.

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

## [0.5.1] — 2026-06-30

### Changed

- **Default scan scope is now `.` (project root)** — both `tsguard` and `pyguard` previously
  defaulted to opinionated directory lists (`src,cdk` and `functions,cdk` respectively).
  The new default scans from the project root so the gate works out of the box on any
  layout without requiring `--dirs`. Override per-project with `--dirs src,lib` or set
  `dirs` in `oxguard.toml` / `[tool.pyguard]`.

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
