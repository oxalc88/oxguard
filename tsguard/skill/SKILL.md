---
name: tsguard
description: >-
  Run Tsguard quality checks for TypeScript projects and act on its normalized
  agent or JSON results. Use for type errors, lint, FTA scores, coverage,
  security, advisory criticality, or requests to run or interpret Tsguard.
---

# Tsguard

Delegate analysis to Tsguard. Use normalized findings to select work; use raw analyzer diagnostics only when a finding needs more evidence.

## Invoke the installed CLI

Work from the project containing `package.json`, or supply `--root <project>`. Prefer the project-local npm distribution:

```sh
npx --no-install tsguard check --output agent
```

For a pnpm project, use `pnpm exec tsguard check --output agent`. Verify local installation with the same executor and `tsguard --version`. If the package is missing, use the project's package manager to install `@oxguard/tsguard` as a dev dependency when installation is authorized. Do not silently fetch a CLI with plain `npx`. Do not run `setup` or install a global binary as part of routine analysis.

Use the command the user requested. Otherwise default to `check`:

- `check`: blocking, fail-fast lint → FTA → types → coverage → security.
- `lint`, `types`, `fta`, `coverage`, `security`, `npm-audit`, `secrets`: individual gates.
- `criticality`: advisory function/method caller ranking and `CRITICALITY.md`.
- `audit`: advisory criticality → dead-code → duplicates; exit 0 even on findings.
- `fix`: source formatting/lint mutation; run when source changes are authorized.

Keep requested scope and thresholds. Supported flags include `--root`, `--dirs <d1,d2>`, `--exclude <d1,d2>`, `--timeout <seconds>` and `--max-fta-score <n>`. CLI overrides root `oxguard.toml`, then defaults apply. Unknown flags, missing values and invalid output modes are errors. Agent/JSON modes accept captured stdout without `--allow-pipe`. Help, version, doctor, setup and hooks retain human output; do not give them `--output agent/json`.

## Consume findings completely

Use `--output agent` for the first bounded view. It shows the total count and at most ten findings. **If `omitted: N (use --output json)` appears, retrieve JSON with the same command, root, scope and thresholds before claiming that all findings have been reviewed or addressed.** The summary cannot describe omitted findings. A rerun produces a new result, so keep source/configuration unchanged during retrieval.

```sh
npx --no-install tsguard check --output json
```

Parse JSON as data. Require `schema_version: "1"`. Use `status`, `exit_code`, `findings`, `measurements`, `artifacts` and `diagnostics`. Process the complete `findings` array, grouping work by rule and location; summarize groups without pasting the full JSON into context. Preserve the total count and any unresolved groups. Finding fields include stable `id`, `rule`, `gate`, `level`, `severity`, `status`, `category`, optional `location`, numeric `observed` / `threshold`, `evidence`, and diagnostic IDs. Do not reconstruct these fields from prose.

Distinguish `blocking`, `advisory`, and `execution_error` findings. Read the semantic run status as well as the real process code:

- `quality`: inspect the reported rule/location; use evidence to guide fixes.
- `tool_missing`, `startup_failure`, `dependency_failure`, `timeout`: resolve
execution or dependencies; do not treat them as source defects.
- `invalid_configuration`: correct flags, project root or configuration.
- `lock_contention`: report the active lock; do not delete it or retry blindly.
- `adapter_failure`, `analyzer_failure`, `diagnostics_failure`, `artifact_failure`,
`lock_failure`, `interrupted`, `unclassified_failure`: use the referenced diagnostics to identify the cause before making changes.

Exit 1 alone does not mean “fix source.” `audit` and `criticality` can have `status: error` with exit 0. A passed gate does not prove that later gates ran: `check` stops at the first blocking failure. Fix authorized issues, rerun the failed gate, then rerun `check` to reach the remaining gates.

FTA reports file scores and the configured cap; do not invent per-function scores or unreported component measurements. Biome and Opengrep preserve native rule IDs; TypeScript preserves TS codes. Unsupported tool details produce a stable gate-level fallback, whose cause must be checked in diagnostics.

## Use criticality as advisory context

Read `criticality.in_degree` measurements and `CRITICALITY.md`. The report ranks up to 30 functions/methods by distinct callers; JSON has all measured functions, including zero-caller functions. Repeated calls from one caller count once. Use the rank to identify functions that need careful tests before changes, not as a blocking threshold or proof of runtime impact. Static unresolved and external calls are disclosed in the report. No cycle, depth, wrapper, architecture-rule or baseline/change comparison analysis exists yet.

## Drill down and report

Resolve finding diagnostic IDs through the JSON `diagnostics` array. Read only the relevant stdout/stderr file when evidence is insufficient. Structured modes save complete streams under `node_modules/.cache/oxguard/diagnostics/`; a later run can overwrite them. `--log-file` is optional additional logging; `--tail` limits human display only. Do not create a temp log and read every analyzer log for the normal path.

If the user supplied an existing JSON result or log, interpret that artifact without rerunning unless asked. Label legacy prose as unnormalized and do not invent stable IDs or counts that it does not establish. Launcher errors before Go starts retain stderr/exit output; do not expect JSON for those failures.

Report command/scope, semantic status, real exit code, total findings, blocking versus advisory work, execution problems and the next concrete action. State any omitted or unreviewed findings and gates that have not run. After fixes, verify the complete result before claiming success.

## Verify assessment completeness

Read `assessment` and `gates` in JSON. `assessment: complete` means every requested gate ran with supported normalization; it does not mean findings passed. Each gate has `status` (`passed`, `advisory`, `failed`, `error` or `not_run`) and `normalization` (`complete`, `partial` or `not_run`). The agent summary includes incomplete assessment, not-run and partial counts. Retrieve JSON with the same invocation when any count is nonzero to name the affected gates. Do not claim a complete assessment from a partial adapter, execution error, or fail-fast result. Use the relevant isolated gate to investigate, then rerun the full requested command after authorized fixes.

Read coverage measurements and owned threshold rules directly. Native dependency advisories are informational in Tsguard; audit-ci's policy decision remains the blocker, including allowlists. Use `related` locations for duplicate pairs. Report unsupported details explicitly rather than converting raw diagnostic prose into invented normalized findings. Existing analyzers remain Level 1; advisory criticality parity is present, but remaining Level 2 and all Level 3 checks are absent.
