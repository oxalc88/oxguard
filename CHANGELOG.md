# Changelog

All notable changes to oxguard are documented here.
Format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).

## [Unreleased]

**Upgrade note:** PyGuard cognitive complexity is advisory in this release: a function scoring above 20 reports the warning `pyguard.complexipy.cognitive_exceeded` but never changes the exit code or the gate/overall pass–fail outcome. The next release will make it blocking, so functions above 20 will then fail the radon gate. Review and refactor them before upgrading.

**Upgrade note:** For projects with no Ruff configuration, PyGuard now enables blocking Ruff rules S110 (try-except-pass) and B012 (jump in finally). A `pass`-only handler on a bare `except:` or on `except Exception:`/`except BaseException:`, or a control-flow statement in `finally`, therefore fails the ruff gate and `pyguard check`. Ruff's default `check-typed-exception = false` leaves typed handlers such as `except ValueError: pass` unflagged. Projects with a `[tool.ruff]`/`[tool.ruff.*]` table, `ruff.toml`, or `.ruff.toml` are unaffected: their Ruff configuration remains authoritative, including any `ignore` entries.

### Added

- PyGuard measures the advisory Python function graph in `pyguard structure` (#23 P5). The same in-scope files and test/exclusion rules as the module graph feed pyan3 2.9.0's defined `FunctionDef`/`AsyncFunctionDef` use edges; NetworkX condenses SCCs. Every function gets `function.fan_in` (distinct in-scope defined callers, `callers`), `function.fan_out` (distinct in-scope defined callees, `callees`) and `function.call_depth` (longest path between SCCs in `edges`, with no invented depth inside a cycle) at location file/line/symbol, and the project gets `function.recursive_components` (`components`) and `calls.unresolved` (`calls`). Repeated calls count once; a self-call is not its own caller or callee but still forms one recursive component, so `function.fan_in` differs from the unchanged `pyguard criticality` `criticality.in_degree` only for self-recursive functions (criticality counts the self-call once). `calls.unresolved` counts distinct pyan3 unresolved call references from in-scope functions (undefined names, unresolved attributes/imports and called parameters, including indirect subscript callees and parameters bound by an enclosing function and called from a nested function); parameter placeholder reads are not call references, and repeated identical unresolved targets from one caller are deduplicated by pyan3. Module-level, external and out-of-scope calls are not edges. The function graph emits no findings and never changes the exit code or blocking outcome. Sources with a UTF-8 BOM or a valid PEP 263 declaration are mirrored as UTF-8 for pyan3 while reported locations stay original and pyan3's original inferred import root is preserved, even when an ancestor `__init__.py` marker is outside the requested scope; a file pyan3 cannot parse leaves the assessment incomplete with `pyguard.structure.not_evaluated`, while missing/broken pyan3 or NetworkX is an execution error. The command remains absent from `pyguard check` and also runs through `audit` and the `structure.module_graph` `diagnose` analysis. `structure.function_graph` is now `implemented` with the four criticality-semantics cases plus Tsguard-parity, resolved method/nested-function, mutual-recursion SCC depth, test-file-exclusion, unresolved-dynamic-call, encoded-source import-root, indirect/closure unresolved-call and unparsable-`not_evaluated` controls. (#23)
- PyGuard measures per-module coupling in the advisory runtime module graph (#23 P4b). `pyguard structure` now emits `module.fan_in` (distinct in-scope runtime importers) and `module.fan_out` (distinct in-scope runtime dependencies) for every module, using the same `structure` level and `modules` unit as Tsguard and `module.dependency_depth`, and reports `pyguard.structure.HIGH_MODULE_COUPLING` when a module has at least 3 importers and at least 8 dependencies. The thresholds are fixed constants with no configuration, and the finding is advisory (severity `warning`, category `quality`): it never changes the exit code or blocking outcome and remains absent from `pyguard check`. Human-mode `pyguard structure` and `pyguard audit` render coupling and cycle advisories from the same normalized adapter payload instead of analyzer prose. Imports under `TYPE_CHECKING:`, deferred function/method-local imports and external imports never count, out-of-scope/excluded/test modules are boundaries with no measurement, and a self-import remains one runtime edge and one cycle but is not counted as its own importer or dependency. The `structure.coupling` parity capability is now `implemented` with a 3/8 positive case, a human-mode rendering case that shows both coupling and runtime-cycle advisories, 2/8 and 3/7 negative cases, and TYPE_CHECKING-only, deferred, external, configured-exclusion and conventional-test-file boundary cases. (#23)
- PyGuard builds an advisory runtime module graph for Python projects with grimp 3.18 and NetworkX 3.7 (#23 P4a). `pyguard structure` reports in-scope runtime imports as `module.count`, `module.edges`, `module.cycles`, `module.type_imports`, `module.deferred_imports` and per-module `module.dependency_depth` (longest path between SCCs, never an invented depth inside a cycle), and emits one advisory `pyguard.structure.CIRCULAR_DEPENDENCY` warning per runtime strongly connected component, including self-loops. Cycles never change the exit code or blocking outcome, including when other findings are present; like Tsguard, a cycle can change an otherwise passing overall/gate status to `advisory`. Imports under `if TYPE_CHECKING:`/`typing.TYPE_CHECKING` are excluded and counted as type imports; imports inside function or method bodies are excluded from runtime edges/cycles and counted as deferred imports; external imports are not edges. Import roots come from the project layout, independently of graph scope: files under `<project>/src` use `src` as their root (including the default `.` scope and narrowed `--dirs` paths inside `src`), and every other file uses the project root. Identities are full dotted paths below those fixed roots (namespace directories included) and are never renamed from import names. Resolution inventories every Python file under the roots, so an existing out-of-scope target is a scope boundary with no edge or parent fallback; scope/exclusion/test filters only choose graph nodes. `from X import name` validates `name` against the full inventory (submodule, subpackage, or an AST-defined/exported `__init__` name) before the boundary applies, and namespace `from . import x`/`from pkg import x` validates `x` as a child module/package. Unresolved or invalid relative imports are never passed to grimp; a grimp resolver panic is incomplete `not_evaluated` evidence instead of an execution failure. A missing module inside the project package root, a duplicate module identity or an unparsable source makes the assessment incomplete with `pyguard.structure.not_evaluated` instead of crashing, while missing/broken grimp or NetworkX is an execution error. The command also runs in the informational `audit` group and as the `structure.module_graph` `diagnose` analysis; it is not part of `pyguard check`. grimp is pinned exactly at 3.18 in the dev-dependency manifest and hash-locked evaluation toolchain, and `pyguard doctor` reports its version. The `structure.runtime_graph` and `structure.false_positive_controls` parity capabilities are now `implemented` with runtime-cycle, self-loop, nested-package, namespace-package, namespace-relative-import cycle, acyclic-chain/depth, scope/test-exclusion, duplicate-identity, TYPE_CHECKING-only, external-import, external-name-collision, deferred-import, excluded-target, default-scope src-layout cycle, narrowed-scope boundary and missing-child, package-export, invalid-relative-import and unresolved-local cases. (#23)
- PyGuard applies a zero-config exception-handling baseline when the project root has no Ruff configuration. It adds `--extend-select S110,B012` to every Ruff lint invocation PyGuard owns (`check`/`ruff`, machine and human output, `diagnose` `ruff.lint`, and `fix`'s `ruff check --fix`) and records the `ruff.baseline_version` measurement (level `code`, location `.`, value 1, unit `version`). Both rules are blocking through the existing Ruff adapter: exit code 1 and a failed ruff gate. A project with `[tool.ruff]` or a `[tool.ruff.*]` table in `pyproject.toml`, `ruff.toml`, or `.ruff.toml` keeps exactly its current invocation with no extra flags and no measurement, so project `ignore` policy is preserved; unreadable or malformed configuration is treated conservatively as configured. `# noqa: S110` remains effective and S112 is not added. S110 keeps Ruff's default scope (`check-typed-exception = false`): it flags `pass`-only bare/`Exception`/`BaseException` handlers, not typed handlers such as `except ValueError: pass`, and an ellipsis handler is not flagged. The `quality.zero_config_exception_handling` parity capability is now `implemented` with positive, negative and boundary cases, including a logging-versus-pass contrast, a typed-exception limitation boundary, an ignored S110 project and Ruff's existing test-file coverage. (#23)
- PyGuard measures cognitive complexity for every function with complexipy 8.3.0, pinned exactly like radon and pyan3 in the dev dependency group and in the hash-locked Python evaluation toolchain. Each function is one `complexipy.cognitive` measurement at level `code`, unit `score`, threshold 20. A score strictly above 20 yields the advisory warning `pyguard.complexipy.cognitive_exceeded` (severity `warning`, category `quality`) that never changes the exit code or the gate/overall pass–fail status, including when no other complexity finding exists; exactly 20 is clean. `pyguard doctor` reports the complexipy version, and `pyguard diagnose` exposes the `complexipy.cognitive` analysis. The Python maintainability harness adds the `quality.cognitive_complexity` capability with an advisory-warning positive case; flat/no-finding, deeply nested never-blocking and test-file-exclusion negative cases; and nested-function-folding plus exact 20-versus-21 boundary cases. complexipy folds nested functions into the enclosing function, so pyguard keeps one outer measurement rather than inventing an inner symbol. (#23)
- Python maintainability parity eval harness (#23): `evals/python/maintainability.cjs` runs seeded `criticality` cases through the built PyGuard binary against fixtures it writes to a temp directory and emits the `maintainability.python` report (case set, capability and kind mirrored by `evals/maintainability-capabilities.json`). Every capability now carries a `pyguard` slot with status `planned`, `implemented` or `deferred`; `structure.function_graph` is seeded with positive, negative and boundary criticality cases (cross-module distinct callers, uncalled zero, single-caller dedup, self-recursion boundary), and P8 `change.native_duplication` is `deferred`. `evals/maintainability-parity.cjs` accepts the report as an optional second argument: `not_implemented` is accepted, `pyguard_maintainability_ready` requires every non-deferred capability to pass all three case kinds, `pyguard_cases_passed` tracks the seeded cases, and a failing Tsguard or Python case exits non-zero. CI runs the suite on all three OSes and feeds it to the parity job.

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
