# OxGuard capability evals

The first eval corpus contains 17 Tsguard CLI cases. It complements unit,
installed-distribution and release smoke tests. It does not implement Level 3
project baselines or change analysis.

## Known answers

`evals/cases.json` records stable case IDs, historical references, fixture names
and expected semantic results. `evals/fixtures/` contains reviewed source and
configuration. Expectations are not generated from the candidate's output.

The corpus covers clean and faulty types/lint, FTA pass/failure, known caller
counts, recursion/repeated calls/exclusions/path aliases, malformed compiler
configuration, missing tools, unknown/missing flags, invalid output modes,
lock contention, omitted findings, deterministic repeat runs, explicit roots
and symlink log paths. Finding identity is checked structurally and across
repeated runs; English messages and huge logs are not golden snapshots.

The history provides evidence for regressions. Clean controls are new independent
fixtures, clearly labelled. Passing these small fixtures does not establish
precision/recall on arbitrary production repositories.

## Run through an installed package

Use Node 24, Go from `tsguard/go.mod`, and the normal installed-package harness:

```sh
node --test evals/evals.test.cjs
OXGUARD_EVAL_REPORT_DIR=eval-reports node --test npm/distribution.test.cjs
```

The harness builds, packs and installs the actual npm distribution. Evals invoke
its Node launcher and native Go binary, with real packaged analyzers. Each case
gets a fresh temporary project. No analysis logic or package dependency changes
are required. Set `TSGUARD_PACKAGE_MANAGER=pnpm` to exercise that installation
path locally; CI already protects both managers in its existing nine-job matrix.

For a package already installed in your own test workspace:

```sh
node evals/run.cjs --launcher /absolute/path/node_modules/@oxguard/tsguard/bin/tsguard.cjs \
  --revision COMMIT_SHA --report eval-reports/candidate.json
```

Do not use an uninstalled CLI or plain `npx` fetch as a substitute. The evaluator
creates its own projects; it does not run analysis against your application.

## Compare native CLI revisions

On a PR, the Linux npm job evaluates the base commit and candidate using the
**same current corpus, evaluator, installed launcher and pinned toolchain**.
The harness builds the baseline native binary in an isolated git worktree,
swaps it into the temporary installed native package, evaluates it, then restores
the candidate before the existing integration assertions continue.

```sh
OXGUARD_EVAL_REPORT_DIR=eval-reports \
OXGUARD_EVAL_BASELINE_REF=FULL_40_CHARACTER_COMMIT_SHA \
node --test npm/distribution.test.cjs

node evals/compare.cjs eval-reports/npm-linux-x64/baseline.json \
  eval-reports/npm-linux-x64/candidate.json eval-reports/comparison.json
```

This isolates native CLI capability changes. It is not a comparison of entire
historical npm packages or historical toolchains. The separate distribution tests
protect launcher/package behavior. A baseline predating JSON is marked
`unsupported`, not treated as a successful run or scored for prose it cannot
normalize. Failed startup/preflight and malformed JSON are failures.

Reports with different corpus/evaluator hashes, Node/platform/architecture,
toolchains or case sets cannot be compared. A newly added fixture is run on both
revisions with the same oracle. Do not compare old report files against a changed
corpus and call the difference a capability gain.

## Results and merge rule

Each case has `pass`, `fail`, `unsupported` or `skipped`:

- `pass`: all semantic assertions match the independent expectation.
- `fail`: wrong findings/counts/categories/status, malformed output, execution
  failure or timeout; a failed launch never silently becomes unsupported.
- `unsupported`: a working historical CLI does not advertise a required interface.
- `skipped`: an explicitly conditional symlink test cannot create a symlink on
  this environment. Other cases cannot skip themselves.

Comparison reports classify gained, regressed, retained and unresolved cases.
Changes in symlink applicability are listed separately and never counted as a
capability gain. Losing a previously passing conditional case blocks readiness.
The candidate must pass every required case, even when the baseline also failed.

Detection metrics match rule/gate/category/status and source location one-to-one.
They count true positives, false positives (including duplicates), and misses in
marked detection cases. Clean controls contribute unexpected-finding counts.
No supported observations means precision/recall is null, not 100%.

Case duration, output bytes and maximum agent bytes/lines are recorded. Runtime
includes repeat/agent invocations for the applicable case. Agent limits remain
26 lines / 6 KiB. Every child has a 60-second harness deadline; slow installation
uses the existing separate budget. Runtime deltas are observations, not a noisy
CI performance gate. Memory, token cost and installation latency are not measured.

CI runs CLI evals on the three npm platform jobs and uploads candidate JSON.
The Linux PR job also uploads baseline JSON plus JSON/Markdown comparisons.
All nine npm/pnpm integration jobs remain required by the workflow's tests; repo
branch-protection policy is separate. The eval-enabled integration budget is ten
minutes; non-eval jobs keep the existing six-minute integration budget.

Require existing cases to remain passing and a new capability to have positive,
negative and boundary cases. Changes to expected answers require a documented
behavior decision and review; never rewrite an oracle merely to get green CI.
Corpus changes must accompany their fixtures/provenance. Store reports as CI
artifacts rather than committing generated observations as golden answers.

## Lanes still missing

- PyGuard analysis has no known-answer corpus in this first slice; its existing
  hook/setup/doctor tests remain separate.
- Opengrep, coverage, secrets and dependency transport/advisories are protected
  by existing tests but have no scored cases in this corpus yet.
- Historical installation timeouts and registry routing remain integration checks,
  not detection metrics. Live security-advisory feeds are outside these deterministic
  evals; the integration harness's local provider must not imply production safety.
- `evals/agent-scenarios.json` provides four history-based prompts and observable
  rubrics: omitted findings, execution errors, advisory criticality and fail-fast.
  No LLM or coding agent is run by this evaluator. These scenarios remain unscored
  until a real agent trace is captured. A proposed plan is not proof of action.
- Larger independent projects, unseen holdout cases, memory measurements and
  controlled performance benchmarks remain future work.
