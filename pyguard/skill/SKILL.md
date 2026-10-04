---
name: pyguard
description: Run Python quality gates through PyGuard, act on normalized agent/JSON results, retrieve omitted findings, and distinguish source findings from execution failures. Use for requests to run pyguard, check Python quality, fix Python lint, inspect PyGuard results, correr pyguard, or analizar calidad python.
---

# PyGuard

Delegate analysis to the Go CLI and its Python analyzers. Use its normalized results to explain actionable findings; do not reconstruct findings or caller counts from raw logs.

## Prepare

Use the requested command and scope. Default to `check` for a general quality request. Use `audit` for advisory criticality, dead-code and dependency analysis. Ask only when the intended operation is ambiguous. Do not run `fix`, `setup`, installation, or source edits unless the user authorizes that operation.

Verify `command -v pyguard`. If missing, explain that execution is blocked and offer installation with the official release installer. Do not claim the latest released binary already contains an unmerged feature. Verify `pyguard --help` advertises `--output` and `--root`; if absent, request an updated compatible binary rather than silently interpreting legacy output as the normalized contract.

Run from the project or pass `--root /absolute/project/path` containing `pyproject.toml`. Preserve requested `--dirs`, timeout and exclusions. Targets are relative to the project root. Most commands default to that root; criticality retains `functions`/`cdk` unless `--dirs` is explicit. Project settings come from `[tool.pyguard]` in `pyproject.toml`. Conventional test-file exclusions affect the owned complexity helpers and Radon; they do not disable Ruff or coverage.

If an owned helper is missing or incompatible, report the execution failure and suggest an explicitly authorized `pyguard setup` to deploy updated `tools/analysis` scripts. Do not overwrite project helpers without authorization.

## Run the bounded contract

```sh
pyguard check --root /absolute/project/path --output agent
```

Substitute the requested analysis command. Analysis commands accept `human`, `agent` and `json`; setup, doctor, hooks and Lambda invoke/test retain human interfaces. Structured modes accept captured/piped stdout without `--allow-pipe`. Use `--tail` and `--log-file` only when additional diagnostics are needed, never as the normal log-parsing workflow.

Record the actual process exit code and semantic status. The agent summary shows at most ten findings, total `findings`, gate names, diagnostic availability and `omitted: N (use --output json)`. Truncation of long source names or evidence is possible. When `omitted` is positive, retrieve JSON for the same command, root, scope and configuration before assessing all findings or reporting a complete inventory:

```sh
pyguard check --root /absolute/project/path --output json
```

JSON is complete for executed gates, not for steps skipped by fail-fast. If rerunning is blocked or the source changed, report the incomplete inventory explicitly. Prefer JSON immediately when an exact complete inventory is requested. Do not count analyzer log lines as findings or invent findings that are absent from the normalized result.

## Act on the result

Use schema `"1"`: `status`, `command`, `exit_code`, `findings`, `measurements`, `artifacts` and `diagnostics`. Preserve finding `id`, `gate`, `rule`, source `location`, `category`, `status`, evidence and available observed/threshold values. Do not compare unrelated language-specific metrics as equal scores.

- `quality` with `blocking`: explain the rule/location and a grounded fix. Preserve Ruff codes and mypy error codes; use owned metric thresholds as reported.
- `quality` with `advisory`: describe advisory evidence without treating it as a failed quality gate. Criticality ranks distinct static function callers; it does not prove runtime impact.
- `tool_missing`, `dependency_failure`, `timeout`, `invalid_configuration`, `lock_contention`, `startup_failure`, `interrupted`, `diagnostics_failure`, `adapter_failure`, `artifact_failure` or `unclassified_failure`: resolve or explain execution/configuration needs. Do not interpret these as instructions to change source. For unknown failures, inspect only the referenced gate diagnostics as needed.

A run can be `pass`, `fail`, `error`, `advisory` or `skipped`. Advisory commands retain exit 0 even when `status` is `error`. Never equate code 0 with successful analysis or code 1 with a source defect. Codes 3/4 indicate invalid invocation/configuration and lock contention; human heavy commands also retain pipe refusal code 5.

`check` stops at the first failing gate: Ruff → mypy → Radon → annotations → coverage → security. After an authorized fix, rerun the same command to reach later gates. Do not claim unexecuted gates passed. Use isolated commands for requested detail, not to silently replace the full check.

## Present and drill down

Lead with semantic outcome, command, actual exit code and total findings. Give concise rule/location/action entries, stating advisory or execution errors clearly. Include an omission/incomplete-inventory notice if JSON retrieval could not complete. Mention generated artifacts when relevant.

Resolve finding diagnostic IDs through `diagnostics[].path`, relative to the explicit/discovered project root. Raw stdout/stderr files live in `.pyguard-cache/diagnostics`; a later run can replace them. Inspect them before rerunning when investigating an execution failure. An existing CRITICALITY.md is not evidence that a failed current run produced a report.

For supplied JSON or agent output, interpret the provided record without rerunning unless needed and authorized. If supplied legacy prose is the only evidence, label the interpretation as diagnostic evidence with unknown complete finding count; do not present it as schema-1 results. Request a compatible CLI result when exact findings or semantic categories are needed.
