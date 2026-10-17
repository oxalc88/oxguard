# Test selection experiment (PR 1, shadow only)

Decision: retain the full mandatory suite. Use deterministic greedy set cover for
further experiments. Clustering did not improve measured replay time. No production
package, dependency, required check, or release policy changes in this PR.

## Run

Requires Python 3.12+, Node 24, and the Go toolchains declared in the three go.mod
files. No pip or npm install is needed for the prototype itself. The optional
installed distribution test still downloads its existing pinned toolchain.

From the repository root:

```sh
python -m unittest discover -s experiments/test-selection -p 'test_*.py'
python experiments/test-selection/collect.py --go --repeats 2 --output eval-reports/selection.json
python experiments/test-selection/probe.py eval-reports/selection.json eval-reports/probed.json.gz
python experiments/test-selection/compare_selection.py eval-reports/probed.json.gz eval-reports/comparison.json --changed evals/compare.cjs
```

`collect.py --distribution` also measures the two installed npm tests separately.
This is expensive: it runs the full suite, then each test twice. It is opt-in.
Collection fails on missing runners, failed/skipped tests, ambiguous IDs, or absent
measurements. Go subtests remain an indivisible parent test. Cross-platform runners
are unchanged; collect on each platform before comparing environments. A platform
that skips a test cannot provide evidence for its removal.

To reproduce selection from the committed measurement, without recollecting:

```sh
python experiments/test-selection/compare_selection.py experiments/test-selection/evidence.json.gz eval-reports/comparison.json --changed evals/compare.cjs
```

The compressed JSON contains every test ID, command, timing sample, covered range,
mutation result, and clean replay. `summary.json` contains the proposed exploratory
set explicitly. `ci-baseline.json` contains job IDs and elapsed runner time from logs.
These reports are observations, never reusable green-check authorizations.

## Measurements

Base: `b9a314c3d2211df29aeddbddaec8e615ddc62593`, latest main when work began.
Linux x64, Node 24.19.0, Python 3.12.14; Go versions resolve from each module.
Two isolated timing/coverage samples per test: 33 Node harness tests and 90 Go tests.
All passed. One Go test had variable coverage and is retained by every selector.

The full observed coverage union is **1,421 executed units**. Go units are
statement-block spans; Node units are executed V8 ranges, including child Node
processes. These are different measurements, not interchangeable line/branch
percentages. Retention means retention of this full observed union only. Go's
independent `go test -count=1 -cover ./...` baseline was:

| Package | Statement coverage |
|---|---:|
| contract | 84.0% |
| tsguard | 49.9% |
| tsguard/internal/hooks | 40.0% |
| pyguard | 26.0% |
| pyguard/internal/hooks | 41.6% |

Clean shadow replay, with protection disabled **for analysis only**:

| Approach | Tests | Actual wall time | Coverage union retained |
|---|---:|---:|---:|
| Full measured set | 123 | 6.926 s | 100% |
| Greedy, coverage + observed defects | 96 | 6.422 s | 100% |
| K-means, k=5 + evidence repair | 96 | 6.977 s | 100% |
| Fuzzy C-means, k=5 + evidence repair | 97 | 6.947 s | 100% |
| Impact: evals/compare.cjs | 24 | 3.012 s | Partial, change-specific |

Clustering was also evaluated at k=10 and k=15. It selected 97–100 tests and
provided no useful advantage over greedy selection. It groups tests by binary
executed-range and observed-defect vectors, not CI jobs. Seed is 0. K-means uses
squared Euclidean distance; FCM uses membership exponent 2, with overlapping
assignments at least as large as uniform membership. Both stop after 40 iterations
or unchanged centers. These are experiment settings, not quality thresholds.
Representatives use measured duration; global greedy repair preserves every
observed coverage unit and distinct detected defect, rather than trusting distance
to a centroid. No statistical dependencies are installed or shipped.

Greedy saved 0.505 s (7.3%) in one paired replay, before about 50 ms of selector
work. Isolated-test estimates show larger savings because they include process
startup per test; they must not be presented as actual CI savings. The full warm
collector baselines were 2.358 s for Node, 0.407 s contract, 1.921 s Tsguard, and
1.644 s PyGuard. These use native full-suite commands; replay uses explicitly
selected package/file commands. Compare like with like.

Repeated timing samples changed the exploratory greedy set (Jaccard 0.939).
All protected selections retained 123/123 tests, with stability 1.0. Fixed inputs
and seed produce deterministic selections; timings themselves are not deterministic.
This is another reason not to activate selection in CI yet.

## Defect evidence and safety boundary

Four explicit contract mutations were applied in temporary copies: accept a broken
candidate, accept an invalid schema, accept duplicate case IDs, and hide missed
findings. The full suite detected all four. The **actual greedy Node selection was
also run against all four mutants and detected each one**. Schema, duplicate-case,
and false-negative mutations each had only one detecting test. Their coverage
similarity does not permit dropping those tests. The probe stores the full
mutation-to-test matrix. This small sample is not a general mutation score.

All current tests default to protected: this repository's suites exercise contracts,
negative cases, security boundaries, and regressions. Missing coverage and variable
coverage also force retention. The safe proposed minimum is therefore all 123
measured tests **plus every unmeasured mandatory check**. The 96-test greedy set
is an experimental candidate, not a proven equivalent suite. Impact selection is a
dynamic lower bound: it falls back to the full set for unmapped inputs, including
configuration, dependency, and toolchain changes. No automatic skip command or
CI selection hook is added.

Historical evidence was inspected in recent commits, PR #9 (no review comments),
and Actions runs 38803077904, 38807747978, 38815616310, and 38815971900. The two
failed runs caught Ultracite smoke-fixture defects in the second npm distribution
test across all nine OS/manager jobs while PyGuard passed. That distribution test
stays mandatory. The Linux main job spent 182.101 s on its two distribution tests
and 1.137 s on 33 harness tests. The 13 main jobs consumed 2,155.737 elapsed runner
seconds (35.93 runner-minutes), before billing rounding or platform multipliers.
Dollar cost is unavailable: account billing and discounts were not measured.
PR 1 changes no matrix jobs, so realized mandatory CI savings are **zero**.

## Verification and limits

The local npm distribution run passed both tests in 212.513 s, including 39 installed
maintainability cases. Python installed evaluations passed 15 behavior and 22 parity
cases. All three Go suites, 33 Node harness tests, 2 Python helper tests, 6 identity
hook tests, and the prototype's 5 safety tests passed. The existing PyGuard job runs
the five new safety tests on Linux, macOS, and Windows; no new workflow or job exists.

Per-test historical failure rates, comprehensive mutation detection, branch
coverage, Python helper subprocess coverage, and comparable local Windows/macOS
measurements are unavailable. The collector does not label tests redundant on this
basis. Installed eval scenario durations already exist in their JSON reports, but
those scenarios are not individually selectable from the parent distribution test.
Their current unit is the protected parent; splitting it requires separate evidence.
Existing Level 2/3 Python gaps remain recorded in the existing parity reports and
are unchanged.

Next evidence to collect: repeated paired replays on the existing CI runners,
mutation samples outside the harness, and test-level failure artifacts from future
CI runs. Only then consider whether any specific protected test can be safely
replaced. PRs 2–5 are outside this change.
