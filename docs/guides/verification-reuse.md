# Verification reuse

Every PR and `main` push runs the full suite. Release verification may reuse checks from a successful `npm.yml` push run on `main` for the exact candidate commit; the same matrix jobs still report.

| Checks | Release behavior |
|---|---|
| Go/Node harness and Python regression, helper, behavior and parity checks | Reuse matching evidence |
| npm/pnpm installed distribution, shared parity gate, release archive and tarball smoke checks | Always run fresh |

Evidence is each job's `verification.json`. It binds the tracked tree, commits, runner image, Node, Go, package manager and Python package versions, plus a SHA-256 index of the reports. Any mismatch, missing artifact or other source runs the checks instead. Downloaded content is never executed.

To exercise reuse without releasing, dispatch `npm distribution checks` with `reuse-verification=true` and an optional SHA. Local checks:

```sh
node --test scripts/verification-evidence.test.cjs
python scripts/release-smoke.py archive tsguard v1.2.3 linux amd64 dist
python scripts/release-smoke.py npm v1.2.3 dist/npm
```

For agents: a green required check for the current commit is verification evidence; changed inputs still need their checks.
