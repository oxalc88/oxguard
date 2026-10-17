# Toolchain inventory and verified updates

Follow-up observed against main `de16790` on 2026-10-17 UTC. Registry metadata is evidence of available versions, not proof of analyzer compatibility. No quality thresholds or required checks changed.

## npm and native runtime

All 14 product analyzer pins are in `npm/toolchain.json`; npm resolves their transitive dependencies during installation. The generated launcher uses only Node built-ins. Platform packages carry the Go CLI plus checksum-verified Opengrep binaries for Linux x64/arm64 (glibc and musl), macOS x64/arm64, and Windows x64. `npm/native-tools.json` retains all seven engine hashes. Opengrep remains v1.23.0: changing it requires new hashes and native behavior evidence.

| npm analyzer | Before | Registry latest | Decision |
|---|---|---|---|
| ultracite | 8.0.1 | 8.0.1 | Retain; already latest |
| @biomejs/biome | 2.5.15 | 2.5.15 | Retain; already latest |
| typescript | 5.9.3 | 7.0.2 | Keep 5.9 fallback; verify project 7 type checking / typed lint only |
| oxlint | 1.87.0 | 1.88.0 | Stable package updates |
| oxlint-tsgolint | 7.0.2003 | 7.0.2004 | Stable package updates |
| vitest | 5.0.4 | 5.0.5 | Update patch group |
| @vitest/coverage-v8 | 5.0.4 | 5.0.5 | Update patch group |
| vite | 8.3.1 | 8.3.4 | Update patch group |
| fta-cli | 3.0.1 | 3.1.0 | Stable package updates |
| knip | 6.38.0 | 6.41.0 | Stable package updates |
| jscpd | 5.3.2 | 5.4.1 | Stable package updates |
| secretlint | 13.0.6 | 13.0.7 | Stable package updates |
| @secretlint/secretlint-rule-preset-recommend | 13.0.6 | 13.0.7 | Stable package updates |
| audit-ci | 7.1.0 | 7.1.0 | Retain; already latest |

PR #14 updated Vitest and coverage-v8 to **5.0.5**, plus Vite to **8.3.4**. Primary npm metadata declares:

- Vitest 5.0.5 requires coverage-v8 5.0.5 and supports Vite `^6.4.0 || ^7.0.0 || ^8.0.0`.
- coverage-v8 5.0.5 requires Vitest 5.0.5.
- Vitest Node range: `^22.12.0 || ^24.0.0 || >=26.0.0`; Vite range: `^20.19.0 || >=22.12.0`. Both accept every existing OxGuard Node version.

The follow-up groups are Secretlint with its recommended rule preset; Oxlint with oxlint-tsgolint; and FTA/Knip/jscpd. Fresh registry metadata confirms all seven target versions. Secretlint requires Node 22+, Oxlint and Knip require `^20.19.0 || >=22.12.0`, and jscpd requires Node 18+. These fit the existing Node policy; FTA and tsgolint declare no Node engine range. Oxlint accepts tsgolint `>=7.0.2003`, including the selected 7.0.2004. Hosted distribution checks remain required for every supported OS.

The canonical Node policy is now `tsguard/runtime-compatibility.json`. npm derives its engines field and Go embeds the same data for setup/doctor. The accepted versions remain `^22.18.0 || ^24.0.0 || >=26.0.0`. Documentation describes this policy but does not execute a second definition.

## Go

| Module dependency | Pinned | Go proxy latest | Decision |
|---|---|---|---|
| BurntSushi/toml | v1.6.0 | v1.6.0 | Retain |
| x/sys | v0.42.0 | v0.49.0 | Defer native/toolchain change |
| x/term | v0.41.0 | v0.47.0 | Defer native/toolchain change |

`go mod tidy` corrects direct-dependency classification in both CLI modules. It removes no versions and changes no sums. The local shared contract dependency remains. Minimum Go versions remain 1.25.0 for contract/Tsguard and 1.26.1 for PyGuard; lowering PyGuard or raising Tsguard requires separate compatibility evidence.

## Python evaluators

The hashed `evals/python/requirements.txt` locks all 53 dependencies declared in requirements.in, including analyzer transitives. Keep the existing pins in this PR: mypy/pytest have major upgrades, while changes to other analyzers can change findings and known answers. The release smoke workflow incorrectly required Semgrep even though `requiredUvDevDeps` removed it and PyGuard uses Bandit for baseline SAST. Remove that obsolete expectation; retain every evaluator dependency. The direct analyzer inventory is:

| Tool | Pinned | PyPI latest |
|---|---|---|
| bandit | 1.8.6 | 1.9.4 |
| coverage | 7.10.7 | 7.16.2 |
| deptry | 0.23.1 | 0.25.1 |
| detect-secrets | 1.5.0 | 1.5.0 |
| mypy | 1.18.2 | 2.4.0 |
| pip_audit | 2.9.0 | 2.10.1 |
| pyan3 | 1.2.0 | 2.9.0 |
| pytest | 8.4.2 | 9.1.1 |
| pytest-cov | 7.0.0 | 7.1.0 |
| radon | 6.0.1 | 6.0.1 |
| ruff | 0.14.0 | 0.17.1 |
| vulture | 2.14 | 2.16 |

Sources: [npm registry metadata](https://registry.npmjs.org/), [Go module proxy](https://proxy.golang.org/), and [PyPI JSON metadata](https://pypi.org/). Exact requests used `https://registry.npmjs.org/<name>/latest`, `https://proxy.golang.org/<module>/@latest` (uppercase escaped as required), and `https://pypi.org/pypi/<name>/json`. Re-querying these URLs may produce later releases.

## Verification

Run the full existing mandatory suites; PR 1 selection is still shadow evidence and does not authorize omissions. Commands used:

```sh
(cd tsguard && go test ./...)
(cd pyguard && go test ./...)
node --test scripts/verification-evidence.test.cjs evals/evals.test.cjs evals/python/evals.test.cjs evals/agent-actions.test.cjs evals/record-agent-command.test.cjs evals/maintainability-parity.test.cjs
node --test npm/distribution.test.cjs
python pyguard/tests/analysis_contract_test.py
```

The first local distribution attempt stopped at Go VCS stamping (`error obtaining VCS status: exit status 128`) in the isolated worktree. Re-running locally with `GOFLAGS=-buildvcs=false` bypasses metadata stamping only. No workflow or product default includes this workaround. Combined with PR 3, all three Go modules passed, as did 38 Node helper tests, two Python helper tests, two installed npm distribution tests, 30/30 capability scenarios and 39/39 installed maintainability scenarios. The combined installed run took 159.08 seconds locally; this is not a controlled timing comparison. Hosted Windows/macOS results require CI.

## Stable package updates verification

Each group was applied cumulatively and ran the unchanged packed-distribution test with
`OXGUARD_EVAL_REPORT_DIR=eval-reports/updates-<group> node --test npm/distribution.test.cjs`.
The registry versions above were rechecked before installation. Linux x64, Node 24.19.0:

| Cumulative group | Distribution tests | Capability cases | Maintainability cases | Wall time |
|---|---:|---:|---:|---:|
| Secretlint + recommended preset | 2/2 | 31/31 | 39/39 | 176.642 s |
| + Oxlint + tsgolint | 2/2 | 31/31 | 39/39 | 155.595 s |
| + FTA + Knip + jscpd | 2/2 | 31/31 | 39/39 | 173.708 s |

Each capability run detected all 18 expected quality findings with zero observed false
positives or false negatives. No fixtures, assertions, thresholds, analyzer policy,
Node range or mandatory checks changed. These timings are observations, not a speed
comparison. All three Go suites, 38 Node harness tests, two Python helper tests,
six identity-hook tests and five selector safety tests also passed. Local Go commands
used the same worktree-only `GOFLAGS=-buildvcs=false` workaround described above.
Hosted Windows/macOS distribution checks remain required before release. Go, Python,
TypeScript, Opengrep and the previously updated Vitest/Vite pins are unchanged here.

## Separately deferred PyGuard gaps

PyGuard retains Level 1 contracts and advisory distinct-caller criticality. Full Level 2 structural analysis and Level 3 Git baseline/change analysis parity remain deferred, as recorded in [the architecture evaluation](../architecture-evaluation.md). No Level 2/3 work is included here.
