# Maintainability candidate validation

Local evidence collected on 2026-10-09, based on main `94ef5cfaf2056c53775e2cf39719f4c571e116fe`. Environment: Linux x64, Node 24.19.0, Go 1.26.1, pnpm 10.34.5; Python uses the existing hash-locked eval environment. The implementation is an unreleased candidate, not complete v0.8 release approval.

## Native and installed checks

| Check | Observed result |
|---|---|
| Tsguard Go unit tests, race detector and vet | Pass |
| Shared contract and PyGuard Go regression tests | Pass |
| Existing evaluator/observer/agent action tests plus new parity tests | 31 pass |
| Repository identity hooks | 6 pass |
| Owned Python helper source-location tests | 2 pass |
| Packed npm distribution on Linux x64 | 2 integration tests pass; real native forwarding, scopes, errors, policy and packaging |
| TypeScript native capability corpus | 30 pass, 0 failed/unsupported/skipped; 30 retained against main, 0 regressions |
| Packed npm new maintainability corpus | Initial integrated 22 pass; final installed-launcher rerun 23 pass, including existing-gate wrapper control |
| Packed pnpm isolated dependency layout | 2 integration tests pass, existing 30 native cases and final 23 maintainability cases pass |
| Python native corpus | 15 behavior + 22 contract/parity cases pass, 0 failed/unsupported/skipped |
| Retained Level 1 shared readiness | True; both observed skill traces pass |
| Native cross-builds | linux/amd64, linux/arm64, darwin/amd64, darwin/arm64, windows/amd64 build successfully |

The final maintainability evaluator SHA-256 is `8644a9108cb77d6dca1603604de9a8ad604467062e0f835faacc87a9fb4db092`. Both installed npm rerun and pnpm reports record it. Capability reports use revision label `v08-local`; they do not pretend that uncommitted validation was run against a future commit SHA. Existing PR CI records its actual SHA and uploads the full reports.

The initial cold pnpm install hit the existing 120-second test timeout in this environment. After populating a reusable test store, the full isolated consumer install and native tests passed without increasing that timeout. Default CI still uses a new temporary store. Download time is not analyzer runtime.

The forwarding-chain fixture passes pinned lint, FTA and types while producing LONG_DELEGATION_CHAIN and FRAGMENTED_DELEGATION advisory findings. The direct equivalent `x * 2` implementation produces neither. Other controls cover unsafe typed operations, legitimate dependency injection, necessary validation, distinct failure policies, runtime versus type-only cycles, exact inert baseline blobs, artificial splitting and reduced branch observations. Passing fixtures do not establish precision across arbitrary production code.

## Agent guidance

An independent agent read the updated skill and invoked observed commands through the existing command observer. It retrieved 12 findings through JSON after the bounded report omitted two, repaired missing TypeScript via offline npm without editing source, and interpreted advisory criticality without demanding score-driven edits. Its full check stopped on an actual coverage adapter failure because the temporary owned runtime lacked its Vitest configuration. The trace records that failure and later not_run gates; it does not claim that fixture's full check passed. Distribution coverage checks separately pass with the complete packaged runtime.

The prior TypeScript trace is preserved under `evals/agent-traces/history/`; the new capture records the revised skill hash and actual process/source fingerprints. Skill changes require a new capture through the existing harness.

## Runtime observations

Fresh child-process measurements include CLI and Node startup. Peak RSS uses Linux `RUSAGE_CHILDREN` in a separate worker for each command. These are one-run observations, not controlled before/after benchmarks or whole-check overhead estimates.

| Input | Command | Wall time | Peak child RSS | Assessment |
|---|---|---|---|---|
| Synthetic 200 TypeScript modules/functions, 199 edges, transformations rather than forwarding wrappers | maintainability | 1.413 s | 197.5 MiB | Complete; 0 findings |
| Same input | typed-lint | 2.483 s | 384.1 MiB | Complete; 0 findings |
| Copied real repository npm/eval Node tooling, fixtures omitted, 15 selected modules | maintainability | 2.105 s | 256.5 MiB | Incomplete; unresolved copied dependencies disclosed |

Typed lint adds the pinned ESLint/typescript-eslint dependencies and a separate compiler pass. The smell/structure/change path reuses candidate facts and one compiler pass; an explicit baseline requires another pass. These observations expose a material memory cost. Larger independent repositories, cross-platform runtime measurements and controlled whole-check comparisons remain work before a broad performance claim.

## Outstanding verification and scope

The existing PR matrix performs macOS/Windows runtime checks and both supported pnpm versions; cross-compilation alone is not runtime verification. Release smoke tests are unchanged and no release is published.

New PyGuard maintainability capabilities are **not implemented/evaluated**; the separate capability matrix identifies candidate Python providers and returns `maintainability_parity_ready: false` and `release_ready: false`. Historical FTA/cognitive/duplication deltas, rename tracking, branch-expression complexity and runtime/domain responsibility inference are also outside the supported change comparison. See [implemented scope](maintainability.md). A branch reduction control does not establish behavioral equivalence or prove easier maintenance.
