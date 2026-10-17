# Verification reuse (PR 2)

PR 1 found no basis for removing protected tests. Every PR and main push still runs
the existing suite. Release verification can reuse identified checks from a green
main run; the same required matrix jobs still report their result.

## Reuse boundary

| Checks | Release behavior |
|---|---|
| Go and Node harness regression checks in npm jobs | Reuse matching trusted evidence |
| Python regression, helper, behavior and parity checks | Reuse matching hash-pinned toolchain evidence |
| npm/pnpm installed distribution checks | Always run fresh: npm transitives have no committed lock |
| Shared parity/readiness gate | Always validate the current run's reports |
| Actual release archives and npm tarballs | Always smoke-check before artifact upload/publication |

Evidence comes only from a completed, successful `npm.yml` push run on `main` in
this repository, for the **exact candidate commit**. PR artifacts, cancelled/failed
runs, other workflows/repositories, expired artifacts and missing artifacts are
ineligible. Artifacts are downloaded with the existing GitHub Actions action and a
read-only token. Downloaded content is never executed.

The manifest binds the complete tracked tree (source, tests, configuration,
dependency pins, workflows, skills), source and executing-workflow commits,
matrix group, hosted runner image/version, kernel/architecture, complete Node
runtime version, effective Go version/settings, and package-manager version.
Python evidence also binds the interpreter and every installed package version;
installation still enforces the existing hash-pinned requirements file. Dirty
tracked inputs cannot produce evidence. Missing hosted-image identity prevents
reuse. The executing workflow must come from the same commit as the candidate.

Reports must exist, have the correct candidate revision, and match the SHA-256
payload index written after all job checks pass. Source, environment, checksum or
provenance mismatch runs existing checks. Evidence recording also fails if inputs
change during execution. No age-based exception or arbitrary acceptance threshold
is used. External live security scans are not cached by this mechanism.

Each successful job adds `verification.json` to its existing report artifact.
There is no new cache service, workflow, matrix job, or dependency. PRs 3–5 are not
part of this change. Full distribution reuse remains deferred until its transitive
dependency resolution can be identified and reproduced; direct pins alone do not
meet that requirement.

## Inspect and verify

The existing `npm distribution checks` workflow now supports a manual run with
`reuse-verification=true` and an optional exact SHA. It only verifies; it cannot
publish or deploy. Leave SHA empty to use the dispatch commit. After this change
is merged and a green main run uploads manifests, dispatching the same commit can
exercise a real reuse hit without creating a release. A different workflow/image,
missing evidence, or mismatched inputs will run the checks instead. Job summaries
link to the main run used as evidence.

For coding agents, a successful required check for the current commit is already
verification evidence. Read its summary before repeating equivalent local tests.
Changed inputs and unverified local edits still need their applicable checks.
This does not add a review requirement or reuse human approval for changed work.

Local checks:

```sh
node --test scripts/verification-evidence.test.cjs
python scripts/release-smoke.py archive tsguard v1.2.3 linux amd64 dist
python scripts/release-smoke.py npm v1.2.3 dist/npm
```

Archive smoke checks verify the published-file checksum, binary GOOS/GOARCH, and
Python helper inclusion on every build target. On the native Linux x64 target,
they also execute the extracted CLI's version/help. The npm smoke check extracts
the exact launcher and Linux native tarballs into Node's installed layout, then
runs the launcher against that native payload. It does not repeat registry
installation. Cross-compiled macOS/Windows/ARM payloads receive archive/metadata
checks here; their existing distribution matrix remains fresh.

Validation for this PR: all ten real cross-built release archives passed; native
Linux CLIs and the exact Linux npm tarballs passed; a wrong tag and corrupted
checksum were rejected. Unit tests cover trusted producer restrictions, invalid
states, changed inputs and missing/corrupt payloads. No release was triggered.
End-to-end hosted reuse savings require a new green main manifest and the manual
verification run above; they are not claimed from local tests.
