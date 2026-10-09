---
name: tsguard
description: >-
  Run Tsguard quality checks for TypeScript projects and act on its normalized
  agent or JSON results. Use for type errors, lint, FTA scores, coverage,
  security, maintainability, structural dependencies, baseline comparisons,
  advisory criticality, or requests to run or interpret Tsguard.
---

# Tsguard

Delegate analysis to Tsguard. Use normalized findings to select work; use raw analyzer diagnostics only when a finding needs more evidence.

## Invoke the installed CLI

Work from the project containing `package.json`, or supply `--root <project>`. Prefer the project-local npm distribution:

```sh
npx --no-install tsguard check --output agent
```

For a pnpm project, use `pnpm exec tsguard check --output agent`. Verify local installation with the same executor and `tsguard --version`; check that `--help` advertises `--output` and `--root` before relying on this contract. If the package is missing, use the project's package manager to install `@oxguard/tsguard` as a dev dependency when installation is authorized. Do not silently fetch a CLI with plain `npx`. Do not run `setup` or install a global binary as part of routine analysis.

Use the command the user requested. Otherwise default to `check`:

- `check`: blocking, fail-fast lint → FTA → types → zero-config typed lint → coverage → security, then advisory maintainability, duplication and baseline comparison. Later analyses remain not_run after an earlier blocking failure.
- `lint`, `types`, `fta`, `coverage`, `security`, `npm-audit`, `secrets`: individual gates.
- `criticality`: advisory function/method caller ranking and `CRITICALITY.md`.
- `typed-lint`: explicit compiler-backed unsafe operations and unnecessary-condition checks. The zero-config check runs it automatically; project lint policy remains in control when a lint config exists. A disabled strictNullChecks option makes unnecessary-condition coverage incomplete.
- `maintainability`, `smells`, `structure`: advisory compiler facts for unchanged forwarding chains, repeated catch policy, silent constant fallbacks, runtime module cycles, coupling and dependency depth. Graph artifacts contain all selected facts, not runtime impact guarantees.
- `change --baseline <Git ref>`: advisory baseline comparison using inert source files. Missing or incomplete inputs leave change not_run; absence of a baseline does not fail the command. Retain --baseline when retrieving JSON.
- `audit`: advisory criticality → dead-code → duplicates; exit 0 even on findings.
- `fix`: source formatting/lint mutation; run when source changes are authorized.

Keep requested scope and thresholds. Supported flags include `--root`, `--dirs <d1,d2>`, `--exclude <d1,d2>`, `--timeout <seconds>` and `--max-fta-score <n>`. CLI overrides root `oxguard.toml`, then defaults apply. Unknown flags, missing values and invalid output modes are errors. Agent/JSON modes accept captured stdout without `--allow-pipe`. Help, version, doctor, setup and hooks retain human output; do not give them `--output agent/json`.

Opengrep is required for the security gate. A missing or untrusted engine is an execution failure, not a passing or skipped security assessment. npm installations use the package-owned engine; reinstall with optional dependencies if it is missing. Standalone installations verify a pinned SHA-256 digest and use the OS user cache outside the scanned project; authorized `tsguard setup` installs or repairs that engine.

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

FTA reports file scores and the configured cap; a cap failure exposes only the first failing file because the analyzer exits before producing JSON. Do not invent per-function scores, later failing files or unreported component measurements. Biome and Opengrep preserve native rule IDs; TypeScript preserves TS codes. Unsupported tool details produce a stable gate-level fallback, whose cause must be checked in diagnostics.

## Use criticality as advisory context

Read `criticality.in_degree` measurements and `CRITICALITY.md`. The report ranks up to 30 functions/methods by distinct callers; JSON has all measured functions, including zero-caller functions. Repeated calls from one caller count once. Use the rank to identify functions that need careful tests before changes, not as a blocking threshold or proof of runtime impact. Static unresolved and external calls are disclosed in the report. Criticality alone does not evaluate module cycles or change quality; retrieve the maintainability/change results for those questions.

## Interpret maintainability evidence

Treat LONG_DELEGATION_CHAIN, FRAGMENTED_DELEGATION, REPEATED_ERROR_HANDLER, SILENT_EXCEPTION_FALLBACK and HIGH_MODULE_COUPLING as review cues, not instructions to flatten every abstraction. Verify the reported call path or repeated catch bodies. Preserve dependency injection, stable APIs, intentional best-effort policies and necessary domain validation. Do not add unsafe logging or change failure propagation just to remove an advisory.

Read module.fan_in, module.fan_out and module.dependency_depth with the graph artifact. Depth measures edges between strongly connected components; type-only imports are separate. Missing local and nonliteral dynamic imports make graph assessment partial. External references and unresolved calls remain disclosed limitations; this is a static, scoped graph.

Do not claim a maintainability improvement from smaller files or individual FTA scores. Read POSSIBLE_COMPLEXITY_DISPLACEMENT evidence against the resolved baseline SHA: decreased maximum per-file branches, unreduced total branches and increased structural cost support review, not a proof of equivalent behavior. Function branch counts are observations, not cognitive/FTA scores. Historical FTA and duplicate comparisons, runtime dispatch and rename tracking are not evaluated by change.

## Drill down and report

Resolve finding diagnostic IDs through the JSON `diagnostics` array. Read only the relevant stdout/stderr file when evidence is insufficient. Structured modes save complete streams under `node_modules/.cache/oxguard/diagnostics/`; a later run can overwrite them. `--log-file` is optional additional logging; `--tail` limits human display only. Do not create a temp log and read every analyzer log for the normal path.

If the user supplied an existing JSON result or log, interpret that artifact without rerunning unless asked. Label legacy prose as unnormalized and do not invent stable IDs or counts that it does not establish. Launcher errors before Go starts retain stderr/exit output; do not expect JSON for those failures.

Report command/scope, semantic status, real exit code, total findings, blocking versus advisory work, execution problems and the next concrete action. State any omitted or unreviewed findings and gates that have not run. After fixes, verify the complete result before claiming success.

## Verify assessment completeness

Read `assessment` and `gates` in JSON. `assessment: complete` means every requested gate ran with supported normalization; it does not mean findings passed. Each gate has `status` (`passed`, `advisory`, `failed`, `error` or `not_run`) and `normalization` (`complete`, `partial` or `not_run`). The agent summary includes incomplete assessment, not-run and partial counts. Retrieve JSON with the same invocation when any count is nonzero to name the affected gates. Do not claim a complete assessment from a partial adapter, execution error, or fail-fast result. Use the relevant isolated gate to investigate, then rerun the full requested command after authorized fixes.

Read global coverage thresholds and per-file measurements directly; per-file measurements do not add per-file blocking thresholds. Native test failures are quality findings; no-test collection and missing or malformed reports are execution problems. Native analyzer reports are listed in `artifacts` and refreshed for each run; use those reports for additional upstream fields. Secretlint findings retain rules and positions while omitting secret values.

Native dependency advisories are informational; `tsguard.dependencies.policy_failed` records audit-ci's blocking decision without guessing which individual advisory caused it. The CLI invokes audit-ci with `--moderate` and does not pass `--config`; do not claim a project's allowlist file was loaded automatically. Use `related` locations for duplicate pairs. Report unsupported details explicitly rather than converting raw diagnostic prose into invented normalized findings. PyGuard retains existing Level 1 and criticality capabilities; do not claim Python parity for the new TypeScript structural or change analyses.
