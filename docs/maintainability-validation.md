# Tsguard Biome maintainability validation

Final single-linter local evidence collected on 2026-10-09, based on main `94ef5cfaf2056c53775e2cf39719f4c571e116fe`. Environment: Linux x64, Node 24.19.0, Go 1.26.1, pnpm 10.34.5; Python uses the existing hash-locked eval environment. Tsguard readiness is assessed for the selected Ultracite/Biome scope independently of later Python capabilities. No release is published by this work.

## Native and installed checks

| Check | Observed result |
|---|---|
| Tsguard Go unit tests, race detector and vet | Pass |
| Evaluator/observer/agent action and scope-readiness tests | 32 pass |
| Packed npm distribution on Linux x64 | 2 integration tests pass; fresh installed manifest has no eslint/typescript-eslint dependency; installed npm tree has no ESLint package |
| Existing TypeScript native capability corpus | 30 pass, 0 failed/unsupported/skipped; 30 retained against main, 0 regressions |
| Packed npm new Biome maintainability corpus | 21 pass, including single-linter and existing-gate wrapper controls |
| Packed pnpm isolated dependency layout | 2 integration tests pass; existing 30 native cases and new 21 cases pass |
| Retained Python native corpus | Fresh 15 behavior + 22 contract/parity cases pass, 0 failed/unsupported/skipped |
| Retained Level 1 shared readiness | True; both observed skill traces pass |
| Selected Tsguard maintainability readiness | `tsguard_scope_ready: true`; all 21 required native cases present and passing |
| Native cross-builds after removal | linux/amd64, linux/arm64, darwin/amd64, darwin/arm64, windows/amd64 build successfully |

The final evaluator SHA-256 is `a769d000de36e89482b45a8b586538611cfb23b9ce5e143f1c2b6234c32fd96b`. Both installed package-manager reports record it. Fresh local native reports use revision label `biome-local`; uncommitted validation is not relabelled as a future commit SHA. Existing PR CI records its actual SHA, runs macOS/Windows and both supported pnpm versions, and uploads the full reports. Its final result is linked in the PR description.

Local pnpm verification uses a populated reusable test store to avoid download contention. The existing 120-second install timeout is unchanged. Default CI still uses a new temporary store. Download time is not analyzer runtime.

The forwarding-chain fixture passes pinned lint, FTA and types while producing LONG_DELEGATION_CHAIN and FRAGMENTED_DELEGATION advisory findings. The direct equivalent `x * 2` implementation produces neither. Other controls cover necessary validation, dependency injection, distinct failure policies, runtime versus type-only cycles, exact inert Git blobs, artificial splitting and reduced branch observations. Passing fixtures do not establish precision across arbitrary production code.

The single-linter control rejects the removed typed-lint command and demonstrates that TypeScript compilation alone does not establish unsafe-operation lint coverage. Project lint policies remain authoritative; a consumer requesting an external engine must supply it. The compatibility fixture preserves that policy and reports incomplete normalization rather than substituting the owned Biome baseline.

## Agent guidance

A fresh independent agent read the revised Biome skill and chose observed native/offline npm commands. It retrieved all 12 type findings after the bounded report omitted two, restored missing TypeScript offline without editing source, and interpreted advisory caller counts without score-driven edits. The temporary runtime's full check stopped on a real coverage adapter failure because its Vitest configuration was missing; later gates remained not_run. The capture records that failure, not a passing full check. Installed distribution coverage checks separately pass with the complete packaged runtime.

The prior TypeScript captures remain under `evals/agent-traces/history/`. The current capture records the updated skill hash and actual process/source fingerprints. Skill changes require fresh observed evidence through the existing harness.

## Runtime observations

Fresh process measurements include CLI and Node startup. Peak RSS uses Linux `RUSAGE_CHILDREN`. This is a single-run observation, not a controlled before/after or whole-check benchmark.

| Input | Command | Wall time | Peak child RSS | Assessment |
|---|---|---|---|---|
| Synthetic 200 TypeScript modules/functions, 199 edges, transformations rather than forwarding wrappers | maintainability | 1.331 s | 194.2 MiB | Complete; 0 findings |

The default check has no ESLint process or extra typed-lint compiler pass. The smell/structure/change path reuses candidate facts and one compiler pass; an explicit baseline requires another pass. Larger independent repositories and controlled whole-check measurements remain useful follow-up evaluation without adding a mandatory release ceremony.

## Scope and later capabilities

New PyGuard maintainability parity is deferred independently: `pyguard_maintainability_ready: false` and `maintainability_parity_ready: false` do not block Tsguard's selected scope. The report deliberately has no ambiguous global `release_ready` field. Tsguard scope readiness still requires native cases; incomplete, unsupported, duplicate or failing evidence cannot produce a ready result. Platform checks and explicit publication authorization remain separate requirements.

Compiler-aware unsafe-operation/narrowing-assertion/unnecessary-condition lint beyond Biome is deferred to retain the single default lint engine. Historical FTA/cognitive/duplication deltas, rename tracking, branch-expression complexity and runtime/domain responsibility inference also remain outside this supported comparison. See [implemented scope](maintainability.md). A branch-count reduction does not establish behavioral equivalence or prove easier maintenance.
