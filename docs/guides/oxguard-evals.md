# OxGuard capability evals

The evals contain 30 Tsguard cases and 37 PyGuard cases (15 behavior checks and 22 contract/parity requirements). They complement unit, installed-distribution and release smoke tests. This does not implement Level 3 project baselines or change analysis.

## Known answers

`evals/cases.json` records stable case IDs, historical references, fixture names and expected semantic results. `evals/fixtures/` contains reviewed source and configuration. Expectations are not generated from the candidate's output.

The corpus covers native Python coverage pass/failure, Bandit pass/failure, Vulture/deptry advisory findings, scored TypeScript fail-fast, clean and faulty types/lint, FTA pass/failure, known caller counts, recursion/repeated calls/exclusions/path aliases, malformed compiler configuration, missing tools, unknown/missing flags, invalid output modes, lock contention, omitted findings, deterministic repeat runs, explicit roots and symlink log paths. Finding identity is checked structurally and across repeated runs; English messages and huge logs are not golden snapshots.

The history provides evidence for regressions. Clean controls are new independent fixtures, clearly labelled. Passing these small fixtures does not establish precision/recall on arbitrary production repositories.

## Run through an installed package

Use Node 24, Go from `tsguard/go.mod`, and the normal installed-package harness:

```sh
node --test evals/evals.test.cjs
OXGUARD_EVAL_REPORT_DIR=eval-reports node --test npm/distribution.test.cjs
```

The harness builds, packs and installs the actual npm distribution. Evals invoke its Node launcher and native Go binary, with real packaged analyzers. Each case gets a fresh temporary project. No analysis logic or package dependency changes are required. Set `TSGUARD_PACKAGE_MANAGER=pnpm` to exercise that installation path locally; CI already protects both managers in its existing nine-job matrix.

For a package already installed in your own test workspace:

```sh
node evals/run.cjs --launcher /absolute/path/node_modules/@oxguard/tsguard/bin/tsguard.cjs \
  --revision COMMIT_SHA --report eval-reports/candidate.json
```

Do not use an uninstalled CLI or plain `npx` fetch as a substitute. The evaluator creates its own projects; it does not run analysis against your application.

## Compare native CLI revisions

On a PR, the Linux npm job evaluates the base commit and candidate using the **same current corpus, evaluator, installed launcher and pinned toolchain**. The harness builds the baseline native binary in an isolated git worktree, swaps it into the temporary installed native package, evaluates it, then restores the candidate before the existing integration assertions continue.

```sh
OXGUARD_EVAL_REPORT_DIR=eval-reports \
OXGUARD_EVAL_BASELINE_REF=FULL_40_CHARACTER_COMMIT_SHA \
node --test npm/distribution.test.cjs

node evals/compare.cjs eval-reports/npm-linux-x64/baseline.json \
  eval-reports/npm-linux-x64/candidate.json eval-reports/comparison.json
```

This isolates native CLI capability changes. It is not a comparison of entire historical npm packages or historical toolchains. The separate distribution tests protect launcher/package behavior. A baseline predating JSON is marked `unsupported`, not treated as a successful run or scored for prose it cannot normalize. Failed startup/preflight and malformed JSON are failures.

Reports with different corpus/evaluator hashes, Node/platform/architecture, toolchains or case sets cannot be compared. A newly added fixture is run on both revisions with the same oracle. Do not compare old report files against a changed corpus and call the difference a capability gain.

## Results and merge rule

Each case has `pass`, `fail`, `unsupported` or `skipped`:

- `pass`: all semantic assertions match the independent expectation.
- `fail`: wrong findings/counts/categories/status, malformed output, execution failure or timeout; a failed launch never silently becomes unsupported.
- `unsupported`: a working historical CLI does not advertise a required interface.
- `skipped`: an explicitly conditional symlink test cannot create a symlink on this environment. Other cases cannot skip themselves.

Comparison reports classify gained, regressed, retained and unresolved cases. Changes in symlink applicability are listed separately and never counted as a capability gain. Losing a previously passing conditional case blocks readiness. The candidate must pass every required case, even when the baseline also failed.

Detection metrics match rule/gate/category/status and source location one-to-one. They count true positives, false positives (including duplicates), and misses in marked detection cases. Clean controls contribute unexpected-finding counts. No supported observations means precision/recall is null, not 100%.

Case duration, output bytes and maximum agent bytes/lines are recorded. Runtime includes repeat/agent invocations for the applicable case. Agent limits remain 26 lines / 6 KiB. Every child has a 60-second harness deadline; slow installation uses the existing separate budget. Runtime deltas are observations, not a noisy CI performance gate. Memory, token cost and installation latency are not measured.

CI triggers once per PR update and on pushes to `main`; branch pushes do not start a duplicate run. A newer PR commit cancels its older in-progress checks. Release workflow calls retain their full validation and are not cancelled by this policy. The matrix remains 13 jobs: nine npm/pnpm platform combinations, three Python platform jobs and one shared report.

CI runs CLI evals on the three npm platform jobs and uploads candidate JSON. The Linux PR job also uploads baseline JSON plus JSON/Markdown comparisons. All nine npm/pnpm integration jobs remain required by the workflow's tests; repo branch-protection policy is separate. The eval-enabled integration budget is ten minutes; non-eval jobs keep the existing six-minute integration budget.

Require existing cases to remain passing and a new capability to have positive, negative and boundary cases. Changes to expected answers require a documented behavior decision and review; never rewrite an oracle merely to get green CI. Corpus changes must accompany their fixtures/provenance. Store reports as CI artifacts rather than committing generated observations as golden answers.

## Evaluation coverage and remaining gaps

- TypeScript coverage, Opengrep, Secretlint and native dependency audit now have scored known-answer cases; Python coverage and Bandit also have scored cases. Network/provider failures, broader analyzer versions and unsupported backends remain focused integration/unit coverage rather than exhaustive scored native cases.
- Historical installation timeouts and registry routing remain integration checks, not detection metrics. Live security-advisory feeds are outside these deterministic evals; the integration harness's local provider must not imply production safety.
- `evals/agent-scenarios.json` defines four history-based scenarios now exercised through real independent agents for both skills: omitted findings, execution repair, advisory criticality and fail-fast. `agent-traces/` records native results, argv, source fingerprints and read-only semantic answers. `agent-actions.cjs` scores observable actions; CI replays those captures rather than making live LLM calls. Skill content changes invalidate the corresponding capture until fresh forward-testing replaces it.
- Larger independent projects, unseen holdout cases, memory measurements and controlled performance benchmarks remain future work.

## Python behavior and agent parity

`evals/python/cases.json` contains language-specific controls with the same quality intent: clean/incorrect types and lint, complexity pass/threshold/boundary, annotation depth boundaries, fail-fast, lock/pipe refusal, missing uv and full diagnostic logs. Ruff/mypy JSON, Radon JSON and the owned annotation model independently verify known answers. Those oracle records are **not** normalized PyGuard CLI findings. Separate contract cases assert exact normalized records; no aggregate production precision/recall score is claimed.

Python requirements, including transitive dependencies and hashes, are pinned in `evals/python/requirements.txt`. The evaluator checks the installed versions, runs uv offline, and copies helper scripts from the evaluated source into fresh projects, matching setup's deployment location. It does not test release installer or setup dependency installation. These remain separate smoke-test responsibilities.

```sh
uv venv --python 3.12 /tmp/pyguard-evals
uv pip install --python /tmp/pyguard-evals/bin/python --require-hashes -r evals/python/requirements.txt
(cd pyguard && go build -o /tmp/pyguard-eval .)
node evals/python/run.cjs --binary /tmp/pyguard-eval --source . \
  --python /tmp/pyguard-evals/bin/python --revision COMMIT_SHA \
  --report-dir eval-reports/pyguard
```

On Windows the venv interpreter is `Scripts/python.exe`. Use uv 0.12.19, as CI does. Optional `--baseline-binary`, `--baseline-source` and `--baseline-revision` evaluate a rebuilt base commit with its own source helper scripts and the same pinned tools.

Two reports deliberately answer different questions:

- `behavior.json`: every supported behavior case must pass, including when the baseline failed. This preserves the strict candidate rule.
- `parity.json`: 22 positive requirements probe full JSON, stable IDs/categories, bounded agent output with omission disclosure, input validation/root and known caller counts. Missing advertised interfaces are unsupported; violated requirements fail. The probes actually run even when the flags are absent. Requirements not met never count as passes. All are now required for candidate readiness.

Current Python evidence: all 15 baseline behavior cases remain passing, and all 22 current parity cases pass. The original nine parity requirements now pass; five additional structured adapter cases cover lint, complexity failure/boundary, annotations and caller measurements. `agent_ready` and parity `candidate_ready` are true on the tested candidate/toolchain. Historical baseline missing interfaces remain unsupported and historical defects remain failed; the CLI entry point requires every candidate parity case to pass, independently of baseline outcomes. The regression comparison also blocks loss of any previously passing capability.

CI adds three Python platform jobs with base/candidate comparisons on PRs. A final report job combines Linux evidence in `parity.json` and `parity.md`. The Python reports include source-script provenance and toolchain versions. Reports are uploaded even if the eval step fails.

`evals/capabilities.json` maps shared intent across language-specific case IDs. FTA scores and Radon cyclomatic counts are not interchangeable numbers. The matrix uses `pass`, `fail`, `unsupported`, `skipped` and `not_evaluated`; it does not call untested behavior equivalent. The TS fail-fast path now has a scored native installed-package case, so all 18 shared matrix entries are evaluated. Reports must come from the same revision and environment.

See [PyGuard agent evaluation](pyguard-agent-evaluation.md) for repository evidence and implemented parity. Both CLIs share result/reporting Go code while keeping language-specific analyzers. Existing thresholds and fail-fast gate order remain protected; no unified CLI or packaging refactor is included.

## Level 1 completion gate

The existing report job runs `evals/level1-ready.cjs` and publishes `level1-ready.json`; no extra workflow or runner is added. Readiness requires every case in both current corpora, all 18 shared capabilities, and the recorded action scores for both current skills. Missing cases, failed/unsupported/skipped outcomes, stale skill captures, missing complete JSON retrieval or source changes during execution repair block readiness. An initial TypeScript forward-test had an invalid fixture because the distribution harness deliberately renamed its native package; that attempt is retained as `invalid-initial-typescript.json` and earns no score. A fresh independent task supplied the scored replacement.

The traces prove scoped behavior on these fixtures: both agents retrieve all 12 findings before editing, fix them, rerun the failed gate and full check, and disclose unexecuted gates. Authorized offline repair restores TypeScript with npm and mypy with uv without changing source. Read-only answers distinguish missing tools from source defects and criticality from blocking policy. Python full-check verification remains blocked by missing tests in its fixture; TypeScript's final run is blocked by an unavailable audit endpoint. Those are truthful error results, not failed skill scenarios or claims that all gates passed. Additional models, production projects, analyzer versions and long-term capability accuracy need new evaluation evidence. Passing this gate does not authorize implementing or releasing Level 2/3.

## Contract eval design and cost

Expectations come from the public result contract, historical failures and native analyzer formats. Coverage fixtures have five equal TypeScript functions with three/four/five called (60/80/100%), plus Python source with three of four lines (75%), four of five (80%) and all lines covered (100%). Assertions check numeric measurements, observed/threshold values, rule IDs, locations, compatible exits and assessment completeness. Native failed tests and no-test collection are separate cases; missing/malformed report unit tests require execution errors rather than invented coverage. The evaluator itself is mutation-tested so dropping or changing numbers cannot retain a passing score.

Security uses a project-local Opengrep rule and fixed clean/two-match source. Secretlint receives a synthetic PEM-shaped payload that is not a usable key; normalized output must preserve rule/location and omit payload/native messages. Dependency cases run real npm audit and audit-ci against the existing child-process local provider, with no advisories, high severity or low severity. Each invocation uses the project's isolated npm cache to avoid advisory cache contamination between cases. Both base and candidate use the same provider whose source is included in evaluator provenance; no live vulnerability/rule feed is scored as a fixed answer.

These are development-only fixtures and evaluator scripts. npm manifests still package only the existing launcher/config/native files; no evaluator or Python test dependency ships in the product. There are no extra workflows, jobs or services outside the existing test process. Reports record runtime and output size; the added corpus runs only in the three existing npm eval jobs and Python jobs, while pnpm retains installation compatibility checks. Broader independent repositories and live multi-model agent evaluation are still future lanes; recorded agent-action replay remains explicitly labelled.
