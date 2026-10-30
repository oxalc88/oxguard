# PyGuard agent and JSON result contract

PyGuard and Tsguard share the Go `contract` module for schema-1 results, stable finding IDs, ordering, JSON and bounded agent reporting. Each CLI keeps its own analyzer orchestration and adapters. Python analysis stays in the existing tools and owned helpers; no analysis moves into a launcher or skill.

```sh
pyguard mypy --root /path/to/project --dirs src --output agent
pyguard mypy --root /path/to/project --dirs src --output json
pyguard structure --dirs src --output json
pyguard check --output agent
```

The default remains human output. Structured output is available for analysis commands; setup, doctor, hooks and Lambda invoke/test retain human interfaces. Help/version remain text. Structured modes accept pipes without `--allow-pipe`; human heavy commands retain pipe refusal. `--init` retains its human-only secrets-baseline workflow.

## Shared envelope

See the [Tsguard schema reference](tsguard-result-contract.md) for the common fields and finding identity. Both CLIs return `schema_version`, `status`, `assessment`, `gates`, `command`, `exit_code`, `findings`, `measurements`, `artifacts` and `diagnostics`, including empty arrays. JSON has complete normalized records for executed gates and no rendered `output` field. A fail-fast check does not claim later gates ran.

Agent output uses the same limits in both languages: ten findings, at most 26 lines and 6 KiB. It includes total findings and `omitted: N (use --output json)` when needed. Retrieve JSON with the same command/root/scope before claiming a complete inventory. Long individual evidence or source labels can be truncated in agent output; JSON retains them.

Finding IDs hash gate, rule and project-relative location (including symbols and related duplicate locations when present), not analyzer prose or absolute roots. Ruff and mypy codes remain their native identifiers, except mypy diagnostics captured by an exit-2 syntax abort, which are namespaced as `pyguard.mypy.<code>`. Owned rules include `pyguard.radon.cc_exceeded`, `pyguard.radon.mi_threshold_failed`, `pyguard.complexipy.cognitive_exceeded`, `pyguard.halstead.<metric>_exceeded`, `pyguard.types.annotation_complexity`, `pyguard.criticality.ranked`, `pyguard.structure.CIRCULAR_DEPENDENCY`, `pyguard.structure.not_evaluated` and `pyguard.execution.<category>`. Moving a finding can change its ID; these IDs are not cross-revision baselines.

## Adapters and limits

| Analyzer | Normalized data |
|---|---|
| Ruff | Native lint JSON: codes, locations, messages; formatting failure has the stable `pyguard.ruff.format_required` rule. With no project Ruff configuration, lint also selects blocking S110/B012 and records `ruff.baseline_version` |
| mypy | Native JSON lines: error codes, locations and messages; an exit-2 syntax abort keeps those lines as `pyguard.mypy.<code>` findings and adds `pyguard.execution.analyzer_failure` |
| Radon | Native CC/MI JSON: measurements and existing threshold failures |
| complexipy | Native JSON: one `complexipy.cognitive` measurement per complexipy-reported function and advisory warnings above 20; nested functions fold into the enclosing function's score, so they do not get a separate measurement. An unreadable report or failed input is an execution error, never an empty pass |
| Owned annotation/Halstead helpers | Versioned JSON findings with locations, observed values and thresholds; annotations include remediation |
| pyan3 criticality | Versioned JSON with distinct caller measurements, advisory ranked findings and a generated CRITICALITY.md reference |
| grimp runtime module graph | grimp 3.18 facts normalized by Go: module count/edges/cycles/type-only/deferred counts, per-module SCC condensation depth, advisory CIRCULAR_DEPENDENCY findings and not_evaluated for unresolved-local/unparsable input. Missing grimp/NetworkX is an execution error |
| pytest / coverage.py | Native JSON global/per-file coverage, existing 80% floor and JUnit test failures/execution errors |
| Bandit | Native JSON `test_id`, severity, locations and scanner errors |
| pip-audit | Native vulnerability IDs, package identity and fixed-version remediation; skipped dependencies are execution errors |
| detect-secrets | Owned JSON over native scan records, preserving baseline comparison and excluding secret values/hashes |
| Vulture | Native API items with CLI configuration, exclusions and confidence; advisory |
| deptry | Native JSON `DEP` codes, modules and locations; advisory |
| Unsupported details | Stable partial gate-level fallback with diagnostic references |

Known executable/module absence, uv dependency transport failures, analyzer configuration exits, timeouts and malformed structured output have execution categories. A failed unsupported analyzer produces `pyguard.<gate>.failed` with `unclassified_failure`; do not assume it is a source defect. This deliberately incomplete normalization avoids delegating unlimited logs to the LLM. Analyzer versions must support the requested native structured formats; invalid output produces `adapter_failure`, not a false pass. Real evals pin Ruff 0.17.2, mypy 2.4.0, Radon 6.0.1, complexipy 8.3.0, pyan3 2.9.0, grimp 3.18 and NetworkX 3.7. mypy 2.x reports syntax errors as native JSON lines and exits 2 when errors prevent further checking; PyGuard normalizes those lines as blocking `pyguard.mypy.<code>` findings and records `pyguard.execution.analyzer_failure`, so the gate is `error`, the assessment `incomplete` and the process exits 1. Exit 2 without structured stdout remains `invalid_configuration`.

Python-specific thresholds are CC above 10, Halstead effort above 50,000/difficulty above 30/bugs above 0.4, annotation depth above 2 or length above 40, and coverage below 80%; cognitive complexity above 20 is advisory in this release and becomes blocking next. `pyguard structure` is advisory only: runtime import SCCs (including self-loops) emit `pyguard.structure.CIRCULAR_DEPENDENCY`, and its project measurements use `module.count`/`module.edges`/`module.cycles`/`module.type_imports`/`module.deferred_imports` with per-module `module.dependency_depth`. Imports under `if TYPE_CHECKING:` are type-only; function/method-local imports are deferred; neither creates a runtime edge/cycle, while both are counted. External imports are not edges. Import roots come from the project layout, independently of graph scope: files under `<project>/src` use `src` as their root (including the default `.` scope and narrowed `--dirs` paths inside `src`), and every other file uses the project root. Identities are the full dotted path below those roots (namespace directories included), never renamed from import names. Every `.py` file under those roots participates in resolution, while `--dirs`, exclusions and test filters only choose graph nodes; an existing out-of-scope target is a boundary with no edge, not a missing module or a parent-package fallback. `from X import name` validates `name` against the full inventory (submodule, subpackage, or an AST-defined/exported `__init__` name) before the boundary applies. Export validation is a deliberate over-approximation: a name counts as exported when any statement in the package `__init__` AST can bind it, at any nesting level, including ordinary `if`/`elif`/`else` branches, `try`, `while`, `match`, `with`, function bodies (including `global` declarations), `Store` names, `def`/`class` names, import aliases and except/match targets. Only bodies of `if TYPE_CHECKING:` blocks and annotation-only `AnnAssign` statements are excluded. It also accepts Python-provided module attributes such as `__path__`, `__name__`, `__spec__` and `__file__`, and any name when the package `__init__` defines module-level `__getattr__`, performs a runtime `from ... import *`, or assigns through `globals()`/`setattr()`. Because exports are over-approximated, only a clearly missing name makes the result incomplete. Namespace-package `from . import x`/`from pkg import x` validates `x` as a child module or package. Unresolved or invalid relative imports never reach grimp, and a grimp resolver panic is incomplete `not_evaluated` evidence, not an execution failure. A missing module inside a project package root, a duplicate module identity or an unparsable source leaves the assessment incomplete with `pyguard.structure.not_evaluated`; source parsing honors a UTF-8 BOM and PEP 263 declarations by reading bytes, so valid non-UTF-8 modules keep the same line numbers as grimp. but a cycle never changes the exit code or blocking outcome; like Tsguard, it can turn an otherwise passing overall/gate status into `advisory`. Exactly 20 is clean, and the warning never changes the exit code or the gate/overall pass–fail status now. Zero-config exception handling is blocking: when the project root has no Ruff configuration, PyGuard adds `--extend-select S110,B012` to its Ruff lint invocations and records `ruff.baseline_version` (level `code`, location `.`, value 1, unit `version`). S110 and B012 then appear as native blocking Ruff findings. S110 follows Ruff's default scope (`check-typed-exception = false`): `pass`-only bare/`Exception`/`BaseException` handlers are flagged, while `except ValueError: pass` is not. A project `[tool.ruff]`/`[tool.ruff.*]` table, `ruff.toml`, or `.ruff.toml` keeps the existing invocation with no extra flags and no measurement, so project ignores remain authoritative; unreadable or malformed configuration is treated as configured. `# noqa: S110` is respected, S112 is not selected, and Ruff's existing test-file coverage is unchanged. complexipy reports nested functions as part of their enclosing function, so the single outer measurement carries the folded score and is the symbol to review. The existing `radon mi -n B` invocation rejects grades B/C, which Radon's actual rank function defines as MI at or below 19; structured output preserves that behavior despite the older human description “below grade B.”

## Inputs, execution and diagnostics

Unknown flags, missing/empty values, invalid output modes and invalid numbers are rejected with code 3. `--timeout` is a positive integer; `--tail` is non-negative. `--root` must directly contain a readable valid `pyproject.toml`; omitting it retains upward cwd discovery. Malformed project configuration is an explicit error. Relative targets use the project root; `--log-file` uses invocation cwd, matching Tsguard.

Semantic `status` distinguishes pass, blocking failure, execution error, advisory and skipped runs. Exit codes retain 0/1/3/4 and human pipe refusal 5. Audit/criticality/dead-code/deps retain advisory exit 0 even if normalized status is `error`; consumers must inspect status/categories. Check retains Ruff → mypy → Radon → annotations → coverage → security and fail-fast behavior.

Structured analyzer streams are saved under `.pyguard-cache/diagnostics`, separately for stdout/stderr. Findings reference diagnostic IDs; JSON gives project-relative paths. `--tail` never limits normalization. `--log-file` still appends complete raw streams; log or diagnostic write failure is an execution error in structured modes. Later invocations overwrite numbered diagnostics, so inspect or copy evidence before rerunning.

Owned helpers must be deployed or updated by `pyguard setup`; analysis does not silently overwrite project helpers. Missing helpers produce `tool_missing`; old helpers without JSON produce `adapter_failure`. Installing a newer native binary alone does not update previously deployed project scripts.

## Criticality correctness and scope

Criticality passes explicit CLI `--dirs` to the existing pyan3 analyzer. It uses absolute source paths and function-use edges, not DOT namespace/definition edges. Repeated calls from one function count once; defined functions with zero callers also have measurements. `criticality.in_degree` uses the same meaning and unit as Tsguard. The artifact keeps the top 30 functions with callers. Analysis or artifact failures are explicit and do not register an old report as newly generated.

This is static analyzer evidence, not proof of runtime targets. Python dynamic dispatch, callbacks and reflection remain limited by pyan3 resolution. Criticality remains caller-only; runtime cycles and dependency depth are provided separately by the advisory `pyguard structure` command and are not part of `check`. No architectural layer/boundary rules or baseline comparison are added. Criticality retains its legacy `functions`/`cdk` defaults when `--dirs` is omitted; explicit `--dirs` now reaches the script. Other analysis commands retain their project-root defaults. Project test exclusions remain respected by the shared path collector and by the structure graph.

Level 1 analyzers are existing. The normalized contract and Python agent parity are implemented by PR #5. The advisory caller milestone is repaired; remaining Level 2 rules and Level 3 are still missing. A unified CLI and Python distribution through uvx remain future work.

## Assessment completion

Both guards expose the same additive schema-1 `assessment` and ordered `gates` fields. Each gate records execution status (`passed`, `advisory`, `failed`, `error` or `not_run`) and normalization (`complete`, `partial` or `not_run`). A complete assessment means all requested gates ran with supported normalization; it does not mean blocking findings passed. Errors, partial adapters and fail-fast steps make the assessment incomplete. The agent summary exposes assessment and not-run/partial counts; JSON names those gates. A failed quality gate can have complete normalization.

Native report artifacts are refreshed under `.pyguard-cache/reports` before execution. Coverage keeps the global 80% floor and adds per-file measurements without per-file thresholds. JUnit supplies test names/locations; collection or execution errors remain execution problems. Owned helpers, including `vulture_report.py` and the secrets adapter, execute from private temporary copies embedded in the binary. `pyguard setup` deploys project-local reference copies only; update the binary to update runtime helpers. Missing helpers/reports or unsupported formats cannot certify a passing assessment. Raw reports retain additional native fields for drill-down.

## Diagnose analyses

`pyguard diagnose` runs every independent analysis without stopping at the first failure, so a CC violation does not hide MI, Halstead or later gates. JSON adds an `analyses` array; each entry has `name`, parent `gate`, `status` (`passed`, `advisory`, `failed`, `error` or `not_run`), `normalization` (`complete`, `partial` or `not_run`), referenced diagnostic IDs, and a reason when normalization is incomplete. Any incomplete analysis makes the assessment `incomplete`, even when all executed gates passed. Python splits radon into `radon.cc`, `radon.mi`, `radon.halstead` and `complexipy.cognitive` analyses, ruff into `ruff.lint` and `ruff.format`, and adds the advisory `structure.module_graph` analysis. A partial structure result (for example an unresolved local import) is reported as an incomplete analysis with a `pyguard.structure.not_evaluated` finding; it does not become a clean pass.

Diagnose never installs tooling: it runs via `uv run --no-sync` with `UV_PYTHON_DOWNLOADS=never`, so missing tools surface as errors, not setup. It does not detect a stale environment; select the intended synchronized uv environment first. In human mode it refuses a piped stdout unless `--allow-pipe`; use `--output json` for structured collection. Exit 1 covers blocking findings, execution failures and incomplete assessment. `check` and `radon` retain their fail-fast policy. Details in [diagnose.md](diagnose.md).


## Dependency exposure reviews

All security engines run even if another security engine fails. Native audits still
scan every dependency, and complete reports remain in the diagnostic artifacts.
A development dependency is not automatically safe: tools can execute in CI and
can affect shipped artifacts. Unreviewed exposure keeps the existing blocking
policy (audit-ci severity policy for TypeScript; pip-audit findings for Python).

An optional `.oxguard-dependency-policy.json` accepts a **specific package and
advisory** only after a review concludes that the product is not exposed and
explicitly accepts the tooling risk. It is a reviewed risk decision, not automatic
reachability analysis. Bind it to SHA-256 hashes of the exact manifest and lockfile:

```json
{
  "inputs": {
    "pyproject.toml": "<sha256 of current manifest bytes>",
    "uv.lock": "<sha256 of current lockfile bytes>"
  },
  "reviews": [{
    "ecosystem": "pypi",
    "package": "affected-build-tool",
    "advisory": "GHSA-example",
    "scope": "development_tool",
    "product_exposure": "not_exposed",
    "tooling_risk": "accepted",
    "reason": "Record the advisory's affected operation, product exclusion evidence, CI exposure and accepted risk here."
  }]
}
```

`scope` accepts `development_tool` or `ci`. Unknown exposure, missing reasons,
invalid input hashes and changed manifests/lockfiles cannot authorize exceptions.
Malformed or stale policy reports an execution error and retains native findings.
Do not add a review based only on a package's dev flag. Reassess the decision when
source, packaging or CI changes could alter exposure; hashes detect dependency
changes, not behavioral reachability changes. Reviewed findings remain visible as
advisories with the reason and the original advisory ID. No exceptions are shipped
with OxGuard.
