# Diagnostic evaluation against the P0 handoff

The new `diagnose` commands resolve the handoff's two incomplete-inventory examples: TypeScript reports both failing files, and Python reports independent CC, MI, and Halstead findings. Existing fail-fast commands retain their established behavior. The workflow change applies when collecting independent findings before selecting a repair; it does not make the example projects pass their quality gates.

## Native findings

| Handoff case | Preserved baseline invocation | Diagnostic result |
| --- | --- | --- |
| TypeScript independent file failures | Capped FTA returns one file; later full-check gates remain unrun. Windows returned `a.ts`, Linux returned `b.ts`. | Both `a.ts` at 24.3816168561 and `b.ts` at 24.2401524811 are reported against the unchanged fixture cap 1. Later independent analyses run or disclose unavailable inputs. |
| Python independent metric failures | Radon returns after `quote` CC 12 > 10, omitting catalog MI and Halstead. | CC 12 > 10, catalog MI 16.1808131464 <= 19, and catalog Halstead findings are all visible. Coverage and secrets limitations remain explicit. |

The TypeScript fixture is a cap-1 diagnostic stress test. It does not reproduce a real application's failure at the default cap of 60. No policy threshold, fixture, or original expected result was changed. The synthetic consolidation controls still cross their original default thresholds; diagnostic completeness does not resolve the separate design question of whether a score warrants extraction or local simplification.

## Fresh observed-agent evaluation

Twelve history-free investigations used fresh copies of the frozen multiple-failures fixtures and the same neutral task: investigate the failing quote quality check, explain available findings and analyses that did not run, then propose changes. Eight runs compared the baseline and candidate with unchanged guidance, with two repetitions per language/version. Four additional runs evaluated the candidate with the narrow diagnostic-routing guidance update. An independent reviewer received unlabeled captures and graded acquired evidence, final reporting, integrity, and source/policy changes.

| Condition | TypeScript complete inventories | Python complete inventories | Total |
| --- | --- | --- | --- |
| Preserved v0.9.0 and original guidance | 0/2 | 0/2 | 0/4 |
| New command and original guidance | 1/2 | 2/2 | 3/4 |
| New command and diagnostic-routing guidance | 2/2 | 2/2 | 4/4 |

All twelve captures passed trace-integrity and protected-reference checks, honestly disclosed limitations, and left source, configuration, and thresholds unchanged. The incomplete inventories above were distinct from truthful reporting: five agents accurately reported the findings available from fail-fast execution without collecting both independent source failures. Their honest limitation reports were not counted as complete assessments.

The guidance update directs failing-workflow investigations to `diagnose --output json` with the original scope after confirming command support. General quality requests still default to `check`. The old recorded skill captures were retained in `evals/agent-traces/history/*-before-diagnose-guidance.json`; fresh command observations and four additional history-free skill-verification agents refreshed the current replay inputs. They cover omitted results, source repair, offline tool repair, advisory interpretation, change interpretation, guide routing, and policy boundaries. Offline repair initially encountered omitted cache-path configuration; successful explicit cache-selected retries are recorded alongside the failed attempts rather than presented as unassisted setup success.

These are small exploratory samples. Exact serving-model identity and sampling seeds were unavailable; agents inherited parent settings. The guided condition was selected after observing a discovery gap, so its four successes are not a blinded causal estimate or a general failure rate. The preserved original 10-pass/2-failure agent baseline remains historical evidence and is not relabeled by these results.

## Broader validation

The Linux npm distribution suite passed through a packed, installed launcher, including all 31 native corpus cases and 39 maintainability/structure/change cases. All 378 preserved npm lock entries were present in the installed dependency graph with matching versions and integrities. The packaging test now also asserts that the installed `diagnose` command returns both low-cap FTA files and records unavailable Git comparison explicitly.

The shared contract suite passed 25 tests, TypeScript Go suites passed 132 tests, evaluator/observer/replay harnesses passed 51 tests, and Python behavior/parity corpora passed 15/22 cases. The Python Go run passed 60 tests and reproduced the separately documented `TestAdvisoryAndMissingHelper` mismatch. Its expectation remains unchanged: the available helper reports missing default `functions` input as `invalid_configuration`/exit 1 where the test expects `tool_missing`/exit 0.

Both CLIs cross-compiled for Windows amd64. Windows runtime verification requires the repository's Windows CI jobs; a Linux cross-build does not establish Windows execution behavior. The complete release/platform matrix is not claimed from the local checks.

The original archive, frozen fixtures, dependency locks, native reports, and original agent captures remain intact. Native diagnostics and fresh-agent findings are separate evidence. Passing the supplied behavior tests does not establish coverage, security, or complete application correctness, and the diagnostic command does not perform source repairs.

See [diagnostic usage](diagnose.md), the separate acceptance evaluator at `evals/p0/diagnose.cjs`, and `evals/p0/diagnose-evaluation-results.json` for the reviewed investigation records and provenance.
