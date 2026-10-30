# OxGuard capability evals

The evals contain 30 Tsguard cases and 42 PyGuard cases (16 behavior checks and 26 contract/parity requirements). They complement unit, installed-distribution and release smoke tests.

## Known answers

`evals/cases.json` records stable case IDs, fixture names and expected semantic results; `evals/fixtures/` holds reviewed source and configuration. Expectations are never generated from the candidate's output. Finding identity is checked structurally and across repeated runs; messages and logs are not golden snapshots. Passing these small fixtures does not establish production precision/recall.

## Run through an installed package

Use Node 24 and Go from `tsguard/go.mod`:

```sh
node --test evals/evals.test.cjs
OXGUARD_EVAL_REPORT_DIR=eval-reports node --test npm/distribution.test.cjs
```

The harness builds, packs and installs the real npm distribution and runs each case in a fresh temporary project. Set `TSGUARD_PACKAGE_MANAGER=pnpm` to exercise pnpm. For a package already installed elsewhere:

```sh
node evals/run.cjs --launcher /absolute/path/node_modules/@oxguard/tsguard/bin/tsguard.cjs \
  --revision COMMIT_SHA --report eval-reports/candidate.json
```

Do not substitute an uninstalled CLI or plain `npx`.

## Compare revisions

On a PR, the Linux npm job evaluates the base and candidate native binaries with the same corpus, evaluator, launcher and toolchain:

```sh
OXGUARD_EVAL_REPORT_DIR=eval-reports \
OXGUARD_EVAL_BASELINE_REF=FULL_40_CHARACTER_COMMIT_SHA \
node --test npm/distribution.test.cjs
node evals/compare.cjs eval-reports/npm-linux-x64/baseline.json \
  eval-reports/npm-linux-x64/candidate.json eval-reports/comparison.json
```

Reports with different corpus/evaluator hashes, environments or case sets cannot be compared.

## Results and merge rule

Each case is `pass`, `fail`, `unsupported` (a historical CLI lacks the interface) or `skipped` (only the conditional symlink case). The candidate must pass every required case, even when the baseline also failed; losing a previously passing case blocks. A new capability needs positive, negative and boundary cases. Changing an expected answer requires a documented behavior decision and review; never rewrite an oracle to get green CI. Store reports as CI artifacts, not committed answers.

## PyGuard

Requires Python >=3.12 (3.14.1 is not supported).

```sh
uv venv --python 3.12 /tmp/pyguard-evals
uv pip install --python /tmp/pyguard-evals/bin/python --require-hashes -r evals/python/requirements.txt
(cd pyguard && go build -o /tmp/pyguard-eval .)
node evals/python/run.cjs --binary /tmp/pyguard-eval --source . \
  --python /tmp/pyguard-evals/bin/python --revision COMMIT_SHA \
  --report-dir eval-reports/pyguard
```

Use uv 0.12.19 (on Windows the interpreter is `Scripts/python.exe`). `--baseline-binary`, `--baseline-source` and `--baseline-revision` evaluate a rebuilt base commit. `behavior.json` requires every behavior case to pass; `parity.json` requires all 26 parity requirements. `evals/python/cases.json` holds the PyGuard corpus; a case pins the analyzer's own exit status with `oracle_exit_code` when findings do not imply it (mypy exits 2 on a syntax abort). `evals/capabilities.json` maps the 18 shared capabilities across both languages. See [PyGuard agent evaluation](pyguard-agent-evaluation.md).

### Python maintainability parity

`evals/maintainability-capabilities.json` gives every maintainability capability a `pyguard` slot with status `planned`, `implemented` or `deferred` and its positive, negative and boundary case IDs. `evals/python/maintainability.cjs` runs the built binary against fixtures it writes to a temp directory. `quality.zero_config_exception_handling` is implemented with nine `pyguard ruff` cases: in a project with no Ruff configuration, `except Exception: pass` and a `return` in `finally` are blocking S110/B012 findings with exit 1 and a `ruff.baseline_version` measurement; the same `Exception` handler with a logging body is clean while its `pass` body blocks, and handlers that log or re-raise, `# noqa: S110`, and an ellipsis body produce no S110. Ruff's default `check-typed-exception = false` leaves `except ValueError: pass` unflagged, which is recorded as a boundary. A project that configures Ruff with `ignore = ["S110"]` keeps that ignore and records no baseline. The test-file boundary records Ruff's existing policy: conventional test files are still linted even though the complexity analyzers exclude them. `structure.function_graph` seeds four `criticality` cases: distinct cross-module callers rank at their caller count (repeated calls by one caller count once), an uncalled function stays at `in_degree` 0, repeated calls from a single caller never inflate the count, and the self-recursion boundary scores exactly 1 (a self-call is one defined caller; neither 0 nor a depth-dependent count). `quality.cognitive_complexity` is implemented with six `pyguard radon` cases backed by complexipy 8.3.0: a function scoring 21 produces the advisory `pyguard.complexipy.cognitive_exceeded` warning while the exit code and gate/overall pass–fail outcome match a clean project (the result status becomes `advisory` but never `fail`), a flat function scores 0 with no finding, a deeply nested function scoring 36 is never blocking, root-level test files and whole `tests/`/`test/` directories are excluded from measurement and warnings, a nested function folds into one outer measurement and remains non-blocking, and the boundary measures exactly 20 with no finding next to 21 with one. `structure.runtime_graph` is implemented with `pyguard structure` cases backed by grimp 3.18/NetworkX 3.7: a runtime `a<->b` SCC reports one advisory `pyguard.structure.CIRCULAR_DEPENDENCY` with `module.cycles` 1 and depth 0 at exit 0, a self-import is a one-module cycle, an acyclic `a->b->c` chain has no finding and depth 2/1/0, configured-directory plus test-file/directory exclusions keep `module.count` exact, nested packages retain their intermediate dotted names, namespace packages without `__init__.py` resolve both absolute and `from . import` runtime cycles, duplicate module identities keep one file deterministically and report incomplete evidence instead of crashing, and namespace-relative imports validate child modules, and UTF-8 BOM or PEP 263 non-UTF-8 sources are parsed as bytes so their complete runtime cycles remain detected. `structure.false_positive_controls` is implemented with a positive mixed runtime/type-only cycle, a `TYPE_CHECKING`-only cycle with zero edges and two type imports, external `json`/`os` imports with no edges, an external-name collision where `src/vendor/json.py` cannot turn `import json` into a cycle, a deferred in-function import that closes a potential cycle without creating one (`module.deferred_imports` counted), a legitimate package-export import, an excluded target that stays a scope boundary rather than a missing module or false parent edge, a narrowed file scope that treats an existing sibling as a boundary instead of unresolved, a default `.` scope that still recognizes a `src`-layout runtime cycle, a narrowed file scope that still reports a missing relative child, and unresolved local imports (including `from . import missing` and invalid `from ...missing import X`) that leave the assessment incomplete with `pyguard.structure.not_evaluated` rather than crashing. Python-provided module dunders (`__path__`, `__name__`, `__spec__` and others) close a real package cycle instead of being unresolved, runtime binding statements (import aliases, loop/with/except targets, walrus, class/def and `global` assignment) count as package exports, a conditional `VALUE` bound in both branches of a `sys.version_info` check closes a real package cycle with complete evidence, and module-level `__getattr__`, runtime star re-exports and `globals()`/`setattr()` assignments make arbitrary imported names valid. `structure.coupling` is implemented with `pyguard structure` cases backed by per-module `module.fan_in`/`module.fan_out`: a hub with exactly three in-scope importers and eight in-scope runtime dependencies reports one advisory `pyguard.structure.HIGH_MODULE_COUPLING` at exit 0 and still reports no blocking outcome, while two importers/eight dependencies, three importers/seven dependencies, a TYPE_CHECKING-only, deferred or external eighth dependency, and a hub whose importer or dependency is excluded by configuration or is a conventional test file stay clean with exact fan counts and scope boundaries. A human-mode case runs the real binary without `--output json` and requires the coupling warning and a runtime cycle to remain visible at exit 0. Other capabilities stay `planned` with no cases yet, and P8 `change.native_duplication` is `deferred`.

```sh
node evals/python/maintainability.cjs --binary /tmp/pyguard-eval \
  --python /tmp/pyguard-evals/bin/python --output eval-reports/pyguard/maintainability.json
```

The report is the `maintainability.python` suite; its case set must match the capability spec exactly or the parity report rejects it. `node evals/maintainability-parity.cjs <tsguard.json> [<pyguard.json>]` takes the Python report as an optional second argument: without it, behavior is unchanged (`not_evaluated`, or `deferred` for deferred slots). With it, `deferred` stays deferred, `planned` capabilities report `not_implemented` (accepted for CI, never a pass) and list their seed outcomes in `pyguard_cases`, and only an `implemented` capability whose positive, negative and boundary cases are all present and passing reports `pass`. `pyguard_maintainability_ready` requires every non-deferred capability to be `pass`, `maintainability_parity_ready` combines it with `tsguard_scope_ready`, and the process exits non-zero when the Tsguard scope fails or any Python case fails. The harness unit tests mock the binary; they run a real build when `PYGUARD_EVAL_BINARY` and `PYGUARD_EVAL_PYTHON` (or `EVAL_PY`) are set, and CI runs the suite on all three OSes.

## Level 1 completion gate

The report job runs `evals/level1-ready.cjs` and publishes `level1-ready.json`. Readiness requires every case in both corpora and all 18 shared capabilities. `level1_agent_contract_ready` also requires the agent traces to be ready and current: on PRs, failing or stale traces are a warning; releases set `OXGUARD_REQUIRE_CURRENT_TRACES=1` and block.

### Agent traces (schema 2)

`agent-traces/{typescript,python}.json` record `skill_inventory` and `sessions.{quality,repair,review,focused}`, each with `agent`, `model`, `effort`, `read_source`, `skill_files` (path → normalized SHA-256) and the raw stream hash. `skill_files` are the files the agent actually read plus a declared minimum per session. Changing the inventory makes every session stale; otherwise a session is stale only when one of its own files changed. `node evals/agent-actions.cjs` warns per stale session and fails on wrong behavior, or on stale with `--require-current`. `agent-actions.test.cjs` (blocking) tests the scorer and the skill word limits: total ≤ 700, entry ≤ 150.

### Refresh skill evidence

Re-record only stale sessions, with fresh independent agents; never replace only hashes. Keep the capture directory at `$OXGUARD_CAPTURE_DIR` (default `/workspace/oxguard-capture`) with `ts-toolchain/`, `npm-cache/`, `py-env/`, `uv-cache/`, `bin-real/pyguard` and the packed tsguard `*.tgz`. Then:

```sh
evals/agent-capture/record.sh <effort> [previous-replay-dir] lang/session ...
node evals/agent-capture/assemble.cjs captures <language> <capture-root>/replay [session=dir ...] [--note file]
```

`record.sh` rebuilds fixtures, keeps the sessions not re-recorded and runs the selected agents in parallel; `assemble.cjs` builds the trace from saved streams and hashes each session's installed skill copy. Give agents the task scope without scorer expectations, and keep fixtures outside the repository. `evals/record-agent-command.cjs` is the wrapper that observes the agents' real commands (argv, exit code, output and source fingerprints); configure it with absolute tool and project paths:

```json
{
  "tools": { "pyguard": "/absolute/path/pyguard", "uv": "/absolute/path/uv" },
  "projects": [{ "root": "/absolute/path/fixture", "events": "/absolute/path/events.jsonl", "env": { "UV_OFFLINE": "1" } }]
}
```
