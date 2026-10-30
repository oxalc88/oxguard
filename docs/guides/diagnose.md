# Collect a complete assessment before repair

Use `diagnose` to collect independent quality analyses in one run. A failing lint check does not prevent complexity, type, coverage, or security checks from running. Python CC, MI, Halstead and advisory cognitive complexity are separate analyses, so a CC failure does not hide maintainability debt. Existing `check`, `fta`, and `radon` commands retain their established execution policy.

```sh
tsguard diagnose --dirs src --output json
pyguard diagnose --dirs src --output json
```

The commands use the existing project root, exclusions, thresholds, locking, and per-tool timeout settings. They do not repair source or install missing tools. Python uses `uv run --no-sync` with interpreter downloads disabled; select the intended installed uv project environment before running it. Standalone TypeScript package execution disables automatic npx installation. Configure the pinned package runtime when evaluating against a preserved baseline.

## Read the result

The JSON envelope retains the existing fields and adds `analyses` for `diagnose`. Each entry contains `name`, parent `gate`, `status`, `normalization`, diagnostic references, and a reason when coverage is incomplete. Python includes distinct `radon.cc`, `radon.mi`, `radon.halstead`, `complexipy.cognitive` and `structure.module_graph` entries; cognitive and runtime-cycle warnings stay advisory and never make a complete assessment fail. A structure result with an unresolved local import or unparsable source is `partial` and adds `pyguard.structure.not_evaluated`, so it makes the assessment incomplete rather than passing silently. TypeScript reports the FTA measurement collection ceiling separately as `fta.collection_score_cap`; findings and `fta.score` measurements retain the effective project policy threshold.

| Result | Meaning |
| --- | --- |
| `failed` analysis with `complete` normalization | The analysis completed and found a policy violation. Other independent analyses still run. |
| `error` analysis | A tool could not start, timed out, or could not produce a valid result. Its reason and diagnostics explain the limitation. |
| `partial` normalization | Some requested evidence could not be normalized or resolved. The overall assessment is incomplete. |
| `not_run` analysis | Required input was unavailable. For example, change comparison requires an explicit usable `--baseline` Git reference. The reason identifies the missing input. |
| `assessment: complete` | Every planned analysis completed with full normalization. Findings can still block the project. |
| `assessment: incomplete` | Some requested analysis is unavailable, partial, errored, or not run. Collected findings remain available. |

`diagnose` returns exit 1 when blocking findings, execution failures, or incomplete assessment prevent success. Advisory findings retain their existing policy. Invocation errors, lock failures, and human-mode pipe refusal retain their existing exit codes. JSON contains all findings; agent output remains bounded and discloses both omitted findings and the number of incomplete analyses. Human output lists every analysis and finding after collection.

The scope matches each language's full check, including TypeScript's existing maintainability, duplicate, and change advisories. Python additionally exposes the advisory `structure.module_graph` analysis even though `pyguard check` does not run the `structure` command; the module graph is informational evidence, not a new blocking gate. Project tests can execute and normal analyzer reports or caches can be written. `diagnose` is an assessment command, not a guarantee of side-effect-free test execution.

## Verify against frozen P0 fixtures

The separate evaluator `evals/p0/diagnose.cjs` accepts an options file with `frozenFixtures`, `binaries` keyed by `typescript` and `python`, a new `output` directory, and optional `runtime`, `env`, and per-tool `timeout`. It copies the frozen multiple-failures projects before invoking either binary. It requires both TypeScript FTA findings, both Python CC/MI findings, visible Halstead execution, truthful incomplete coverage, unique diagnostic references, and unchanged project inputs. It stores full native output and a summary with binary and evaluator hashes.

```sh
node evals/p0/diagnose.cjs /absolute/path/to/diagnose-options.json
```

Keep the original P0 expected results and evidence. This evaluator defines acceptance for the new command; it does not relabel the original fail-fast baseline or substitute native output for observed-agent evidence.
