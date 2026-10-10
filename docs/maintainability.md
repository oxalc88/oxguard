# Maintainability analysis

The v0.8 candidate implements the planned Tsguard code, structure and change levels with measurable maintainability protection to the existing CLI and schema-1 result contract. No extra workflow or agent handoff is required. Existing Level 1 gates still fail fast and retain their thresholds.

```sh
npm install -D @oxguard/tsguard
npx tsguard check --output agent
npx tsguard maintainability --output json
npx tsguard change --baseline HEAD --output json
```

## Implemented Tsguard levels

| Level | Implemented behavior | Execution |
|---|---|---|
| 1 — Code | Ultracite/Biome lint, formatting and cognitive cap; FTA; TypeScript; native typed lint; coverage; secrets, dependency CVEs and SAST; unused code and duplication | Existing blocking check gates; Knip in audit; jscpd advisory in check/audit |
| 2 — Structure | Resolved function/method and runtime module graphs; caller/callee counts, criticality, fan-in/out, SCC depth/cycles, coupling; forwarding chains, fragmented forwarding modules and handler policies | Advisory maintainability in check; focused smells/structure/criticality commands |
| 3 — Change | Explicit inert Git baseline; native file FTA/cyclomatic/Halstead distribution, per-function cognitive bounds, clone/token changes, structural/coupling/delegation deltas; conservative file/symbol matching | Advisory change in check when a baseline is available; focused change command |

Python structural/change parity remains a later milestone. These levels describe supported deterministic analyses, not exhaustive type safety, runtime architecture inference or proof of behavioral equivalence. Single-use functions, recursion and file counts alone are not quality violations. No prescribed architecture/layer framework is required.

## Quality policy

Without a lint config, the npm distribution extends pinned Ultracite and applies quality baseline version 1. The baseline explicitly pins cognitive complexity at the existing limit of 20, empty blocks, useless catches, unsafe non-null assertions, unreachable code, constant conditions and other established Biome rules. `lint.baseline_version` records its use. This does not claim those rules were previously missing from Ultracite.

Project Biome configuration, disabled rules and flat ESLint/Oxlint configurations remain authoritative. Legacy ESLint names are recognized so fallback Biome does not silently replace them; support still depends on upstream Ultracite/ESLint, and unsupported execution is an error or partial normalization. The flat ESLint path is tested but its native diagnostics are not yet normalized. Project configs are never overwritten.

Biome with pinned Ultracite presets owns syntax lint and formatting. A separate `typed-lint` gate runs only five upstream compiler-aware rules through pinned Oxlint 1.87.0 / oxlint-tsgolint 7.0.2003, with all syntax categories disabled. No ESLint engine, JS lint plugin or second formatter is added. `types` and structural analysis retain the selected project TypeScript compiler.

`check` runs lint → FTA → types → typed-lint → coverage → security, then advisory maintainability/duplication/change. Native unsafe assignments, unsafe narrowing assertions, floating promises and misused promises are blocking. Unnecessary conditions are advisory because type assumptions may differ from runtime inputs. Native defaults, including explicit `void` promise opt-out, remain upstream policy; an opt-out does not prove safe error handling.

The backend uses TypeScript 7 semantics, independently of the project compiler version. It requires strictNullChecks and resolved, compiler-valid declarations. Supported-config fixtures use the current 5.9 project/compiler path without claiming semantic equivalence to TypeScript 7. Removed options such as baseUrl, native type/configuration diagnostics, missing declarations and scopes over the portable 24000-character file-argument budget leave typed lint partial and nonblocking; missing installed engines or malformed reports are execution failures. Select a smaller scope or migrate unsupported native options deliberately. Semantic scope follows the configured directories/exclusions and existing exclude-tests default; selected files and that policy are recorded in the native artifact. No root config is overwritten and no executable Oxlint config is loaded. Native reports record tool versions, selected rules, project compiler and native diagnostics in `node_modules/.cache/oxguard/typed-lint/native.json`; stale reports are removed before each invocation.

Existing explicit project ESLint/Oxlint policy is still respected through the compatibility path. Such projects must supply their own engine. The zero-config package invokes Oxlint only for the separate semantic gate, not a competing syntax/format policy. Node support remains 22.12+ on the 22.x line, 24.x, or 26+.

## Advisory rules

| Rule | Evidence | False-positive boundary |
|---|---|---|
| LONG_DELEGATION_CHAIN | At least three resolved single-return sync/async layers preserve positional arguments and return type | A single boundary is not flagged; methods, generic/default/rest/optional parameters and transformations are excluded; async timing/stack boundaries remain review judgments |
| FRAGMENTED_DELEGATION | The forwarding chain crosses at least three modules whose analyzed functions only forward | File counts alone never produce a finding; constants and other module-level responsibilities still require human review |
| REPEATED_ERROR_HANDLER | At least three identical catch token sequences of at least 12 tokens | Identifiers/literals are preserved, so different failure policies are not equated; comments are ignored |
| SILENT_EXCEPTION_FALLBACK | Catch body consists only of a literal, bare-value, empty-array or no-value return | Intentional best-effort policies remain valid; review the evidence, do not automatically change propagation or add payload logging |
| SILENT_EXCEPTION_EXIT | Catch body only continues or breaks | Recorded/propagated recovery is excluded; intentional best-effort policy remains valid |
| SILENT_PROMISE_REJECTION | Native promise catch callback only returns a literal/empty array | Recorded or transformed recovery is excluded; advisory, not a forced rethrow |
| DISCARDED_SETTLED_REJECTION | Native allSettled through const aliases into a flatMap block; explicit status guards expose an empty-array rejected path without a reason reference | Bounded path analysis, not arbitrary data flow or a reachability proof; reason references suppress warnings but do not prove recovery |
| CIRCULAR_DEPENDENCY | A runtime import strongly connected component, including a self-loop | Type-only imports do not create runtime cycles |
| HIGH_MODULE_COUPLING | At least three in-scope importers and eight in-scope runtime dependencies | Advisory review cue; counts do not establish poor cohesion |
| POSSIBLE_COMPLEXITY_DISPLACEMENT | Per-file branch concentration or native FTA maximum decreases, total branches do not, edges increase and depth, wrappers or cycles increases | Smaller files alone do not trigger it; branch reduction is a negative control |
| NEW_DEPENDENCY_CYCLES | Candidate has more cyclic runtime components than baseline | Advisory, not an architecture prescription |
| INCREASED_COUPLING | More modules meet the joint fan-in/fan-out review threshold | Counts do not establish poor cohesion |
| FUNCTION_COMPLEXITY_INCREASE | Matched native cognitive lower bound exceeds the prior upper bound | Required domain complexity remains valid |
| NEW_DUPLICATION | Native clone token fingerprint has additional matches | File moves alone are not new duplication |

`check` includes established jscpd duplication analysis after the blocking gates. Its existing minimum match policy is retained; repeated validation and general boilerplate use upstream detection instead of another custom clone engine. Duplicate findings remain advisory. Short repeated catch policies have their own AST-backed evidence below typical general clone thresholds.

## Structure and scope

One compiler pass produces function/call facts, catch fingerprints and module edges for the new smell/structure/change path. Existing criticality reuses the compiler-input helper. Typed lint has its own native semantic program plus a compatibility preflight; it is not part of the reusable graph pass.

Module IDs are root-relative paths. Function IDs contain file, source position and symbol; edge IDs identify their endpoint pair. Findings retain the existing stable schema-1 IDs. IDs are deterministic for the same inputs, not rename-tracking identities across arbitrary refactors.

The graph artifact is `node_modules/.cache/oxguard/maintainability-graph.json`. JSON includes fan-in/out, SCC condensation depth, cycle components, branch observations, forwarding depth, external/type-only imports and unresolved references. Module and function call depth are the longest path between SCCs, not an invented depth within a cycle. Distinct function callers/callees and recursive components are measured in regular analysis without another compiler pass. Functions also include forwarding_mode, max_branch_nesting and branch_locations. Nested callbacks reset branch nesting at their own boundary. These branch observations are not FTA or cognitive scores. Structured FTA cap failures collect complexity-context for only the first failing file, retaining the original failure and not running later check gates.

Source scopes and directory exclusions apply. Dependency directories, `.git`, declaration files and conventional generated files are excluded; conventional test files/directories follow the existing exclude-tests option. Dynamic nonliteral and missing local imports make graph assessment partial. External and out-of-scope dependencies are boundary counts, not inferred graph edges. CommonJS require, dynamic literal imports, re-exports and compiler path aliases are supported. Runtime dispatch, bundler-only resolution and import assertions beyond static compiler resolution are not proven.

## Safe change comparison

Set `--baseline <Git ref>` or root `oxguard.toml` `baseline`. The ref resolves to an immutable commit SHA. Exact Git blobs are read with ls-tree/cat-file into a bounded private source snapshot; export-ignore/export-subst cannot hide or rewrite the input. Symlinks, submodules and dependency trees are not exported. Nested project prefixes are supported. Project source, package scripts, baseline dependencies and hooks are never run to produce measurements.

Comparison reuses candidate structural facts and analyzes the baseline with the trusted current compiler. The snapshot limits are 20,000 copied files, 8 MiB per file, 128 MiB total source and a 4 MiB Git tree listing. Missing compiler/config dependencies, missing scopes and unresolved local imports remain errors or not-evaluated comparisons, not passes.

No baseline means `change` remains not_run, assessment is incomplete and the command is nonblocking. Report the missing optional comparison without inventing a quality failure. Baseline/candidate module, edge, cycle, dependency-depth, fan-in/out, coupling, function call depth, wrapper and branch-distribution measurements are emitted separately. No aggregate maintainability score or behavioral-equivalence claim is produced.

Native comparison metrics reuse the installed FTA, Biome and jscpd engines. Both revisions are projected into private, bounded directories containing only graph-selected source. No baseline or candidate analyzer configuration, application code, dependency code or package script is loaded there. Identical pinned analysis policy is recorded in `node_modules/.cache/oxguard/maintainability-change.json`; the result links this artifact. Existing project policy and thresholds for regular lint/FTA/duplicates gates remain unchanged.

FTA comparison includes short files with `exclude-under: 0`, per-file scores/cyclomatic/Halstead volume, maximum/p50/p90 score distributions and matched-file deltas. Native jscpd compares exact clones with its established 50-token/5-line minimum; token fingerprints retain identifiers/literals, omit trivia and count multiplicity independently of file location. NEW_DUPLICATION reports additional native clone matches, including independent validation that still needs human review.

Biome's cognitive rule has a minimum diagnostic threshold of 1. An isolated measurement pass records exact scores above 1 and honest [0,1] bounds below it. FUNCTION_COMPLEXITY_INCREASE requires a matched candidate lower bound above the baseline upper bound. Inline cognitive suppressions leave unknown upper bounds and partial assessment; they cannot be turned into zeroes. Source changes during measurement, malformed/native tool failures and missing metrics are explicit execution errors. No lint threshold is changed by these measurement passes.

File identity first uses the existing path, then only unique exact source hashes for moves. Functions first use a unique file/qualified-symbol key, then a unique exact token fingerprint. Line movement does not erase matching evidence. Ambiguous copies and arbitrary semantic renames stay unmatched; the artifact records the matching method and both profiles. A move is not a simplification claim.

Full runtime call depth, arbitrary semantic rename inference, custom branch-expression scores and domain responsibility inference remain outside static analysis. The supported displacement checks combine lower per-file branches or native FTA maximum with unreduced branch work and increased structural cost. These facts do not prove every possible displacement or behavioral equivalence.

## Evaluation and parity

`evals/maintainability.cjs` and `evals/review-gaps.cjs` run adversarial cases through the packed installed launcher in the existing npm/pnpm platform matrix. It tests the baseline, Biome syntax policy plus semantic-only native rules, preserved project policy, valid DI/boundaries/validation, exact repeated handlers, exception propagation/recording/fallback, runtime/type-only cycles, generated exclusions, deterministic IDs, bounded agent output, artificial splits, real branch reduction, missing refs, nested projects and inert baseline scripts/export attributes. It never runs in an ordinary guard invocation.

The existing 30 TypeScript and 37 Python Level 1 cases remain. Additional not-run gates are explicitly allowed in the historical fail-fast oracle; all original gate states/findings remain exact, and installed tests separately require the new candidate gate list. New Python capabilities are deferred independently and are not scored as passing. `maintainability-capabilities.json` lists appropriate Python providers and explicit gaps; `maintainability-parity.cjs` cannot turn those gaps into release readiness.

Tsguard v0.8 readiness is assessed for the selected Ultracite/Biome maintainability scope with installed evals, retained Level 1 checks and the existing platform matrix. New Python maintainability parity is a later milestone and does not block that scope. `tsguard_scope_ready` and `pyguard_maintainability_ready` are separate report fields; a passing scope report alone does not replace platform checks or authorize publication. Broader independent repository evaluation remains a useful follow-up, not a new mandatory release ceremony.

## Agent review responsibilities

The bundled Tsguard skill reads the target project's applicable AGENTS.md and referenced policies. It reviews helper purpose, shared policy, recovery visibility, event ownership and whole-workflow complexity. Those judgments cite code/project policy and remain separate from native findings. The skill does not impose a new project architecture document, require every catch to rethrow, or infer duplicate runtime logs from event names alone. Read-only review can run focused smells/maintainability after fail-fast; it cannot turn unexecuted gates into passes.
