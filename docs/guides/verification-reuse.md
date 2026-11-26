# Verification reuse

Full regular verification runs capability and maintainability evals once per language on Linux; PRs may reuse the unchanged component under the rules below, while every `main` push verifies both. npm and pnpm 12 run installed distribution checks on Linux, macOS and Windows; macOS and Windows npm jobs also run selected platform-sensitive capability cases, while PyGuard runs selected existing behavior and maintainability cases on those platforms. Go platform regression tests and real installed CLI integration checks are retained. Only complete Linux reports feed shared parity; smoke selections cannot write full evidence. Release verification may reuse checks from a successful `npm.yml` push run on `main` for the exact candidate commit. Until platform smoke coverage is measured, release calls also run full npm and Python evals on macOS and Windows. pnpm 10 runs distribution-only on release calls until its planned removal after April 30, 2027; regular CI retains its check names as explicit no-op jobs to avoid invalidating unknown required-check settings. pnpm 11 is not in the supported test matrix.

## Component selection on pull requests

| Exact change scope | Fresh verification |
|---|---|
| Only `tsguard/**` or `npm/**`, excluding skills | Tsguard; reuse verified PyGuard base evidence |
| Only `pyguard/**` or `evals/python/**`, excluding skills | PyGuard; reuse verified Tsguard base evidence |
| Shared contracts, other evals, workflows, scripts, skills, mixed or unknown paths | Both |
| Docs only | Existing fast path |

A single scope job checks the exact PR merge commit and both parents with Git's raw diff. Only regular-file additions, modifications and deletions in an unambiguous component qualify. Renames, copies, symlinks, submodules, empty or oversized diffs, mixed paths and unrecognized files run both components. The docs-only allowlist is deliberately limited to `docs/**`, `.github/ISSUE_TEMPLATE/**`, root README, CHANGELOG, PROGRESS, AGENTS and LICENSE; component Markdown fixtures are not silently treated as docs-only.

An unchanged component may reuse evidence only from a successful push workflow on the exact PR base on main, completed within 24 hours, with all of its required platform steps successful and its Linux artifact present. The manifest must match the base tree, repository, suite, group, GitHub-hosted Linux image and Node version, and every report hash and case set must validate. Missing, stale, incompatible or corrupt evidence, API errors or artifact-transfer failures fall back to both components. The shared Node harness still runs on Linux for every non-docs PR. Required job names are unchanged; reused jobs explicitly say they did not execute fresh tests. The original reports retain their base revision. The parity gate independently checks the base evidence and exact impact before combining it with fresh candidate reports, and publishes the source commit/run in the reports and `verification-provenance.json`. Ordinary parity still rejects mixed revisions. A PR cannot claim fresh npm registry resolution when using base evidence; the unpinned npm distribution always runs fresh on main and release.

Main pushes run both components. Dispatch and release calls never use cross-revision PR reuse. A release always verifies all platforms, including pnpm 10 until the documented cutoff. Dispatch with `release-compatibility=true` exercises the release verification path, including strict agent traces, without publishing.


| Checks | Release behavior |
|---|---|
| Go/Node harness and Python regression, helper, behavior and parity checks | Reuse matching evidence |
| npm/pnpm installed distribution, shared parity gate, release archive and tarball smoke checks | Always run fresh |

Evidence is each job's `verification.json`. It binds the tracked tree, commits, runner image, Node, Go, package manager and Python package versions, plus a SHA-256 index of the reports. Any mismatch, missing artifact or other source runs the checks instead. Downloaded content is never executed.

To exercise reuse without releasing, dispatch `npm distribution checks` with `reuse-verification=true` and an optional SHA. Local checks:

```sh
node --test scripts/verification-evidence.test.cjs scripts/component-verification.test.cjs
python scripts/release-smoke.py archive tsguard v1.2.3 linux amd64 dist
python scripts/release-smoke.py npm v1.2.3 dist/npm
```

For agents: a green required check for the current commit is verification evidence; changed inputs still need their checks.
