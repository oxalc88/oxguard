# v0.8 maintainability implementation

Delivery: atomic commits in one PR, based on main `94ef5cf`, following the user's updated delivery instruction. No merge or release authorization is assumed.

## Commit checklist

- [x] Quality baseline: inspect v0.7.2 and PR #5; pin zero-config Biome policy; preserve project lint configs; retain the single Biome lint engine; report deferred typed-lint capabilities.
- [x] Code smells: compiler AST evidence for pass-through chains and repeated handlers; upstream jscpd for repeated validation; advisory false-positive policy; legitimate abstractions and failure-policy controls.
- [x] Structure: module and function/method graphs, cycles, distinct fan-in/out, criticality and SCC dependency/call depth, coupling and fragmentation evidence; deterministic module/symbol identities; reuse compiler parse.
- [x] Change: explicit inert Git snapshot comparison; native FTA/cognitive/duplication distributions and deltas; conservative identity matching; multiple-evidence displacement findings; absent baseline remains not evaluated; no baseline code/config execution.
- [x] Evals and guidance: installed npm adversarial controls, preserved Level 1 checks, explicit Python capability gaps, resource observations and independently exercised skill.
- [x] Preserve cross-platform installed-distribution verification in the existing PR matrix, including pnpm; record the final run in the PR description.
- [x] Separate selected Tsguard readiness from deferred Python structural/change parity.
- [ ] Later milestone: new PyGuard maintainability parity and broader independent repository evaluation.

The five planned delivery scopes are retained in one PR, with separate atomic production, evaluation and guidance commits. The final integration fix preserves native fail-fast behavior and replaces Git archive export with exact source blobs. See [implemented scope](../maintainability.md) and [validation evidence](../maintainability-validation.md).

## Inspected behavior

v0.7.2 and main generate a cached Biome config extending package-owned Ultracite when no lint config exists. Ultracite 7.12.0 already enables cognitive complexity at 20, empty blocks, non-null assertions, useless catches, unreachable code and many redundant constructs. A TypeScript compilation success does not establish type-aware lint coverage. Duplication uses jscpd only in audit.

Project Biome configs are used unchanged, including disabled rules. ESLint/Oxlint configs use Ultracite; native structured normalization for that path remains partial. Legacy ESLint config names must be recognized instead of silently applying fallback Biome policy. The baseline applies only without a lint config. No package-owned ESLint or separate typed-lint gate is added. Compiler-aware unsafe-operation lint is a disclosed later capability, not part of the selected Biome release scope.

## Implementation boundaries

Do not block subjective architecture smells. Do not use file counts as quality judgments. Source-only graphs cannot prove runtime dispatch or business equivalence. Preserve explicit exclusions and report unresolved references. Evals and benchmarks are development-only; do not add workflows or agent handoffs.
