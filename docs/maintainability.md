# Maintainability analysis

The v0.8 candidate adds measurable maintainability protection to the existing CLI and schema-1 result contract. No extra workflow or agent handoff is required. Existing Level 1 gates still fail fast and retain their thresholds.

```sh
npm install -D @oxguard/tsguard
npx tsguard check --output agent
npx tsguard maintainability --output json
npx tsguard change --baseline HEAD --output json
```

## Quality policy

Without a lint config, the npm distribution extends pinned Ultracite and applies quality baseline version 1. The baseline explicitly pins cognitive complexity at the existing limit of 20, empty blocks, useless catches, unsafe non-null assertions, unreachable code, constant conditions and other established Biome rules. `lint.baseline_version` records its use. This does not claim those rules were previously missing from Ultracite.

Project Biome configuration, disabled rules and flat ESLint/Oxlint configurations remain authoritative. Legacy ESLint names are recognized so fallback Biome does not silently replace them; support still depends on upstream Ultracite/ESLint, and unsupported execution is an error or partial normalization. The flat ESLint path is tested but its native diagnostics are not yet normalized. Project configs are never overwritten.

Zero-config `check` also runs package-owned ESLint/typescript-eslint against an explicit TypeScript compiler program. It checks unsafe assignment, arguments, calls, member access, returns, narrowing assertions and unnecessary assertions/conditions. Existing compiler options are retained; the fallback compiler program is strict and needs no generated project config. `no-unnecessary-condition` is not evaluated when project strictNullChecks is disabled, and that result is partial. A project lint config leaves the owned typed baseline not_run, with a PROJECT_POLICY notice when execution reaches it. `typed-lint` explicitly runs the owned rules for diagnosis. Compilation success alone never establishes typed-lint coverage.

ESLint 10.12.0 and typescript-eslint 8.71.1 are pinned package dependencies; standalone setup installs their names through the existing toolchain mechanism. This adds dependencies, not a new service or executable installation hook.

The npm Node engine minimum on the 22.x line is now 22.13, matching ESLint 10; Node 24.x and 26+ remain supported. Ultracite's pinned ESLint peer requires version 10. This is an explicit patch-version compatibility change from the previous 22.12 declaration.

## Advisory rules

| Rule | Evidence | False-positive boundary |
|---|---|---|
| LONG_DELEGATION_CHAIN | At least three resolved synchronous layers preserve positional arguments and return type | A single boundary is not flagged; methods, async boundaries, generic/default/rest/optional parameters and transformations are excluded |
| FRAGMENTED_DELEGATION | The forwarding chain crosses at least three modules whose analyzed functions only forward | File counts alone never produce a finding; constants and other module-level responsibilities still require human review |
| REPEATED_ERROR_HANDLER | At least three identical catch token sequences of at least 12 tokens | Identifiers/literals are preserved, so different failure policies are not equated; comments are ignored |
| SILENT_EXCEPTION_FALLBACK | Catch body consists only of a constant/no-value return | Intentional best-effort policies remain valid; review the evidence, do not automatically change propagation or add payload logging |
| CIRCULAR_DEPENDENCY | A runtime import strongly connected component, including a self-loop | Type-only imports do not create runtime cycles |
| HIGH_MODULE_COUPLING | At least three in-scope importers and eight in-scope runtime dependencies | Advisory review cue; counts do not establish poor cohesion |
| POSSIBLE_COMPLEXITY_DISPLACEMENT | Maximum per-file branches decreases, total branches does not, edges increase and depth, wrappers or cycles increases | Smaller files alone do not trigger it; branch reduction is a negative control |
| NEW_DEPENDENCY_CYCLES | Candidate has more cyclic runtime components than baseline | Advisory, not an architecture prescription |

`check` includes established jscpd duplication analysis after the blocking gates. Its existing minimum match policy is retained; repeated validation and general boilerplate use upstream detection instead of another custom clone engine. Duplicate findings remain advisory. Short repeated catch policies have their own AST-backed evidence below typical general clone thresholds.

## Structure and scope

One compiler pass produces function/call facts, catch fingerprints and module edges for the new smell/structure/change path. Existing criticality reuses the compiler-input helper. Typed lint reuses its compiler program across all selected files; invoking a separate Level 1 typed gate remains a separate process/compiler pass.

Module IDs are root-relative paths. Function IDs contain file, source position and symbol; edge IDs identify their endpoint pair. Findings retain the existing stable schema-1 IDs. IDs are deterministic for the same inputs, not rename-tracking identities across arbitrary refactors.

The graph artifact is `node_modules/.cache/oxguard/maintainability-graph.json`. JSON includes fan-in/out, SCC condensation depth, cycle components, branch observations, forwarding depth, external/type-only imports and unresolved references. Depth is the longest path between SCCs, not an invented depth within a cycle. Branch observations are not FTA or cognitive scores.

Source scopes and directory exclusions apply. Dependency directories, `.git`, declaration files and conventional generated files are excluded; conventional test files/directories follow the existing exclude-tests option. Dynamic nonliteral and missing local imports make graph assessment partial. External and out-of-scope dependencies are boundary counts, not inferred graph edges. CommonJS require, dynamic literal imports, re-exports and compiler path aliases are supported. Runtime dispatch, bundler-only resolution and import assertions beyond static compiler resolution are not proven.

## Safe change comparison

Set `--baseline <Git ref>` or root `oxguard.toml` `baseline`. The ref resolves to an immutable commit SHA. Exact Git blobs are read with ls-tree/cat-file into a bounded private source snapshot; export-ignore/export-subst cannot hide or rewrite the input. Symlinks, submodules and dependency trees are not exported. Nested project prefixes are supported. Project source, package scripts, baseline dependencies and hooks are never run to produce measurements.

Comparison reuses candidate structural facts and analyzes the baseline with the trusted current compiler. The snapshot limits are 20,000 copied files, 8 MiB per file, 128 MiB total source and a 4 MiB Git tree listing. Missing compiler/config dependencies, missing scopes and unresolved local imports remain errors or not-evaluated comparisons, not passes.

No baseline means `change` remains not_run, assessment is incomplete and the command is nonblocking. Report the missing optional comparison without inventing a quality failure. Baseline/candidate module, edge, cycle, dependency-depth, wrapper and branch-distribution measurements are emitted separately. No aggregate maintainability score or behavioral-equivalence claim is produced.

Historical FTA-score, cognitive-complexity and duplication deltas, rename matching, branch-expression complexity, full runtime call depth and domain responsibility inference remain outside this implementation. Current FTA, upstream cognitive checks and jscpd still run through their native gates. The supported displacement fixture preserves branch work while distributing it across new modules; these facts cannot prove every possible complexity displacement.

## Evaluation and parity

`evals/maintainability.cjs` runs adversarial cases through the packed installed launcher in the existing npm/pnpm platform matrix. It tests the baseline, project policy, unsafe typed operations, valid DI/boundaries/validation, exact repeated handlers, exception propagation/recording/fallback, runtime/type-only cycles, generated exclusions, deterministic IDs, bounded agent output, artificial splits, real branch reduction, missing refs, nested projects and inert baseline scripts/export attributes. It never runs in an ordinary guard invocation.

The existing 30 TypeScript and 37 Python Level 1 cases remain. Additional not-run gates are explicitly allowed in the historical fail-fast oracle; all original gate states/findings remain exact, and installed tests separately require the new candidate gate list. New Python capabilities are not implemented or scored as passing. `maintainability-capabilities.json` lists appropriate Python providers and explicit gaps; `maintainability-parity.cjs` cannot turn those gaps into release readiness.

The candidate is not a claim of complete v0.8/PyGuard parity. Complete cross-platform runtime verification, broader independent repositories and Python structural/change parity must be assessed before claiming the entire requested release scope is complete.
