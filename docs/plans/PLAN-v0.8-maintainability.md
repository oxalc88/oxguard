# v0.8 maintainability implementation

Delivery: atomic commits in one PR, based on main `94ef5cf`, following the user's updated delivery instruction. No merge or release authorization is assumed.

## Commit checklist

- [x] Quality baseline: inspect v0.7.2 and PR #5; pin zero-config Biome policy; preserve project lint configs; explicit compiler-backed typescript-eslint; test actual resolution and missing capabilities.
- [x] Code smells: compiler AST evidence for pass-through chains and repeated handlers; upstream jscpd for repeated validation; advisory false-positive policy; legitimate abstractions and failure-policy controls.
- [x] Structure: module graph, cycles, fan-in/out, SCC dependency depth, coupling and fragmentation evidence; deterministic module/symbol identities; reuse compiler parse.
- [x] Change: explicit inert Git snapshot comparison; multiple-evidence displacement findings; absent baseline remains not evaluated; no baseline code execution.
- [x] Evals and guidance: installed npm adversarial controls, preserved Level 1 checks, explicit Python capability gaps, resource observations and independently exercised skill.
- [ ] Complete cross-platform runtime verification in the existing PR matrix, including pnpm.
- [ ] New PyGuard structural/change parity and broader independent repository evaluation before declaring the entire v0.8 scope complete.

The five planned delivery scopes are retained in one PR, with separate atomic production, evaluation and guidance commits. The final integration fix preserves native fail-fast behavior and replaces Git archive export with exact source blobs. See [implemented scope](../maintainability.md) and [validation evidence](../maintainability-validation.md).

## Inspected behavior

v0.7.2 and main generate a cached Biome config extending package-owned Ultracite when no lint config exists. Ultracite 7.12.0 already enables cognitive complexity at 20, empty blocks, non-null assertions, useless catches, unreachable code and many redundant constructs. A TypeScript compilation success does not establish type-aware lint coverage. Duplication uses jscpd only in audit.

Project Biome configs are used unchanged, including disabled rules. ESLint/Oxlint configs use Ultracite; native structured normalization for that path remains partial. Legacy ESLint config names must be recognized instead of silently applying fallback Biome policy. The baseline applies only without a lint config. An explicit `typed-lint` command can diagnose typed rules with project compiler options; `check` selects it automatically only on the zero-config path. A project-owned lint policy does not claim the owned typed baseline ran.

## Implementation boundaries

Do not block subjective architecture smells. Do not use file counts as quality judgments. Source-only graphs cannot prove runtime dispatch or business equivalence. Preserve explicit exclusions and report unresolved references. Evals and benchmarks are development-only; do not add workflows or agent handoffs.
