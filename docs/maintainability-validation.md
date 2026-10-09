# Tsguard Biome maintainability validation

Three-level single-linter local evidence collected on 2026-10-09, based on main `94ef5cfaf2056c53775e2cf39719f4c571e116fe`. Environment: Linux x64, Node 24.19.0, Go 1.26.1, pnpm 10.34.5; Python uses the existing hash-locked eval environment. Tsguard readiness is assessed for the selected Ultracite/Biome scope independently of later Python capabilities. No release is published by this work.

## Native and installed checks

| Check | Observed result |
|---|---|
| Tsguard Go unit tests, race detector and vet | Pass |
| Evaluator/observer/agent action and scope-readiness tests | 33 pass |
| Packed npm distribution on Linux x64 | 2 integration tests pass; fresh installed manifest has no eslint/typescript-eslint dependency; installed npm tree has no ESLint package |
| Existing TypeScript native capability corpus | 30 pass, 0 failed/unsupported/skipped; 30 retained against main, 0 regressions |
| Packed npm new Biome maintainability corpus | 28 pass, including single-linter and existing-gate wrapper controls |
| Packed pnpm isolated dependency layout | 2 integration tests pass; existing 30 native cases and new 28 cases pass |
| Retained Python native corpus | Previously verified 15 behavior + 22 contract/parity cases pass, 0 failed/unsupported/skipped |
| Retained Level 1 shared readiness | True; both observed skill traces pass |
| Selected Tsguard maintainability readiness | `tsguard_scope_ready: true`; all 28 required native cases present and passing |
| Native cross-builds with the comparison helper | linux/amd64, linux/arm64, darwin/amd64, darwin/arm64, windows/amd64 build successfully |

The final evaluator SHA-256 is `86f748e57b8526e7c49c1802b43d3d59b4a433d6e9e90f9842864fdbb995a10c`. Both installed package-manager reports record it. Fresh local native reports use revision label `levels-local`; uncommitted validation is not relabelled as a future commit SHA. Existing PR CI records its actual SHA, runs macOS/Windows and both supported pnpm versions, and uploads the full reports. Its final result is linked in the PR description.

Local pnpm verification uses a populated reusable test store to avoid download contention. The existing 120-second install timeout is unchanged. Default CI still uses a new temporary store. Download time is not analyzer runtime.

The 28-case corpus additionally checks regular function caller/callee depth and recursion, joint coupling thresholds, native clone multiplicity, moved-file/function controls, native cognitive growth, suppressed-score partial assessment, large graph input through a private file, generated exclusions and use of the same current project compiler for both revisions. Native measurement profiles record the pinned tools and policy without running application code or loading historical dependencies.

The forwarding-chain fixture passes pinned lint, FTA and types while producing LONG_DELEGATION_CHAIN and FRAGMENTED_DELEGATION advisory findings. The direct equivalent `x * 2` implementation produces neither. Other controls cover necessary validation, dependency injection, distinct failure policies, runtime versus type-only cycles, exact inert Git blobs, artificial splitting and reduced branch observations. Passing fixtures do not establish precision across arbitrary production code.

The single-linter control rejects the removed typed-lint command and demonstrates that TypeScript compilation alone does not establish unsafe-operation lint coverage. Project lint policies remain authoritative; a consumer requesting an external engine must supply it. The compatibility fixture preserves that policy and reports incomplete normalization rather than substituting the owned Biome baseline.

## Agent guidance

A fresh independent agent read the revised Biome skill and chose observed native/offline npm commands. It retrieved all 12 type findings after the bounded report omitted two, restored missing TypeScript offline without editing source, and interpreted advisory caller counts without score-driven edits. It also ran the native baseline comparison and distinguished actual FTA/cognitive/clone measurements from structural branch observations. The split raised native FTA maximum and cyclomatic work; cognitive bounds and unmatched identities did not establish simplification. Its response explicitly refused to claim proof from smaller files. The temporary runtime's full check stopped on a real coverage adapter failure because its Vitest configuration was missing; later gates remained not_run. The capture records that failure, not a passing full check. Installed distribution coverage checks separately pass with the complete packaged runtime.

The prior TypeScript captures remain under `evals/agent-traces/history/`. The current capture records the updated skill hash and actual process/source fingerprints. Skill changes require fresh observed evidence through the existing harness.

## Runtime observations

Fresh process measurements include CLI and Node startup. Peak RSS uses Linux `RUSAGE_CHILDREN`. The reported child high-water mark is cumulative across the two sequential commands. This is a single-run observation, not a controlled before/after or whole-check benchmark.

| Input | Command | Wall time | Peak child RSS | Assessment |
|---|---|---|---|---|
| Synthetic 200 independent TypeScript modules/functions, no module edges | maintainability | 1.029 s | 184.3 MiB | Complete; 0 findings |
| Same sources against an identical committed baseline | change | 3.062 s | 184.3 MiB | Complete; 0 findings |

The default check has no ESLint process or extra typed-lint compiler pass. The smell/structure/change path reuses candidate facts and one compiler pass; an explicit baseline requires another pass. Larger independent repositories and controlled whole-check measurements remain useful follow-up evaluation without adding a mandatory release ceremony.

## Scope and later capabilities

New PyGuard maintainability parity is deferred independently: `pyguard_maintainability_ready: false` and `maintainability_parity_ready: false` do not block Tsguard's selected scope. The report deliberately has no ambiguous global `release_ready` field. Tsguard scope readiness still requires native cases; incomplete, unsupported, duplicate or failing evidence cannot produce a ready result. Platform checks and explicit publication authorization remain separate requirements.

Compiler-aware unsafe-operation/narrowing-assertion/unnecessary-condition lint beyond Biome is deferred to retain the single default lint engine. Historical native FTA/cognitive/duplication deltas and unique exact move matching are implemented. Arbitrary semantic rename tracking, custom branch-expression complexity and runtime/domain responsibility inference remain outside this supported comparison. See [implemented scope](maintainability.md). A branch-count reduction does not establish behavioral equivalence or prove easier maintenance.
