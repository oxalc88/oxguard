# PyGuard agent and JSON result contract

PyGuard and Tsguard share the Go `contract` module for schema-1 results, stable finding IDs, ordering, JSON and bounded agent reporting. Each CLI keeps its own analyzer orchestration and adapters. Python analysis stays in the existing tools and owned helpers; no analysis moves into a launcher or skill.

```sh
pyguard mypy --root /path/to/project --dirs src --output agent
pyguard mypy --root /path/to/project --dirs src --output json
pyguard check --output agent
```

The default remains human output. Structured output is available for analysis commands; setup, doctor, hooks and Lambda invoke/test retain human interfaces. Help/version remain text. Structured modes accept pipes without `--allow-pipe`; human heavy commands retain pipe refusal. `--init` retains its human-only secrets-baseline workflow.

## Shared envelope

See the [Tsguard schema reference](tsguard-result-contract.md) for the common fields and finding identity. Both CLIs return `schema_version`, `status`, `command`, `exit_code`, `findings`, `measurements`, `artifacts` and `diagnostics`, including empty arrays. JSON has complete normalized records for executed gates and no rendered `output` field. A fail-fast check does not claim later gates ran.

Agent output uses the same limits in both languages: ten findings, at most 26 lines and 6 KiB. It includes total findings and `omitted: N (use --output json)` when needed. Retrieve JSON with the same command/root/scope before claiming a complete inventory. Long individual evidence or source labels can be truncated in agent output; JSON retains them.

Finding IDs hash gate, rule and project-relative location, not analyzer prose or absolute roots. Ruff and mypy codes remain their native identifiers. Owned rules include `pyguard.radon.cc_exceeded`, `pyguard.radon.mi_threshold_failed`, `pyguard.halstead.<metric>_exceeded`, `pyguard.types.annotation_complexity`, `pyguard.criticality.ranked` and `pyguard.execution.<category>`. Moving a finding can change its ID; these IDs are not cross-revision baselines.

## Adapters and limits

| Analyzer | Normalized data |
|---|---|
| Ruff | Native lint JSON: codes, locations, messages; formatting failure has the stable `pyguard.ruff.format_required` rule |
| mypy | Native JSON lines: error codes, locations and messages |
| Radon | Native CC/MI JSON: measurements and existing threshold failures |
| Owned annotation/Halstead helpers | Versioned JSON findings with locations, observed values and thresholds; annotations include remediation |
| pyan3 criticality | Versioned JSON with distinct caller measurements, advisory ranked findings and a generated CRITICALITY.md reference |
| Coverage, Bandit, pip-audit, secrets, vulture, deptry and unsupported details | Stable gate-level fallback plus raw diagnostic references; no fragile exhaustive prose parser |

Known executable/module absence, uv dependency transport failures, analyzer configuration exits, timeouts and malformed structured output have execution categories. A failed unsupported analyzer produces `pyguard.<gate>.failed` with `unclassified_failure`; do not assume it is a source defect. This deliberately incomplete normalization avoids delegating unlimited logs to the LLM. Analyzer versions must support the requested native structured formats; invalid output produces `adapter_failure`, not a false pass. Real evals pin Ruff 0.14.0, mypy 1.18.2, Radon 6.0.1 and pyan3 1.2.0.

Python-specific thresholds remain unchanged: CC above 10, Halstead effort above 50,000/difficulty above 30/bugs above 0.4, annotation depth above 2 or length above 40, and coverage below 80%. The existing `radon mi -n B` invocation rejects grades B/C, which Radon's actual rank function defines as MI at or below 19; structured output preserves that behavior despite the older human description “below grade B.”

## Inputs, execution and diagnostics

Unknown flags, missing/empty values, invalid output modes and invalid numbers are rejected with code 3. `--timeout` is a positive integer; `--tail` is non-negative. `--root` must directly contain a readable valid `pyproject.toml`; omitting it retains upward cwd discovery. Malformed project configuration is an explicit error. Relative targets use the project root; `--log-file` uses invocation cwd, matching Tsguard.

Semantic `status` distinguishes pass, blocking failure, execution error, advisory and skipped runs. Exit codes retain 0/1/3/4 and human pipe refusal 5. Audit/criticality/dead-code/deps retain advisory exit 0 even if normalized status is `error`; consumers must inspect status/categories. Check retains Ruff → mypy → Radon → annotations → coverage → security and fail-fast behavior.

Structured analyzer streams are saved under `.pyguard-cache/diagnostics`, separately for stdout/stderr. Findings reference diagnostic IDs; JSON gives project-relative paths. `--tail` never limits normalization. `--log-file` still appends complete raw streams; log or diagnostic write failure is an execution error in structured modes. Later invocations overwrite numbered diagnostics, so inspect or copy evidence before rerunning.

Owned helpers must be deployed or updated by `pyguard setup`; analysis does not silently overwrite project helpers. Missing helpers produce `tool_missing`; old helpers without JSON produce `adapter_failure`. Installing a newer native binary alone does not update previously deployed project scripts.

## Criticality correctness and scope

Criticality passes explicit CLI `--dirs` to the existing pyan3 analyzer. It uses absolute source paths and function-use edges, not DOT namespace/definition edges. Repeated calls from one function count once; defined functions with zero callers also have measurements. `criticality.in_degree` uses the same meaning and unit as Tsguard. The artifact keeps the top 30 functions with callers. Analysis or artifact failures are explicit and do not register an old report as newly generated.

This is static analyzer evidence, not proof of runtime targets. Python dynamic dispatch, callbacks and reflection remain limited by pyan3 resolution. No cycles, dependency depth, architectural rules or baseline comparison are added. Criticality retains its legacy `functions`/`cdk` defaults when `--dirs` is omitted; explicit `--dirs` now reaches the script. Other analysis commands retain their project-root defaults. Project test exclusions remain respected by the shared path collector.

Level 1 analyzers are existing. The normalized contract and Python agent parity are implemented by PR #5. The advisory caller milestone is repaired; remaining Level 2 rules and Level 3 are still missing. A unified CLI and Python distribution through uvx remain future work.
