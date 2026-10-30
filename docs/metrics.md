---
summary: Reference guide for the complexity and maintainability metrics enforced by pyguard and tsguard.
read_when:
  - Explaining why a complexity or maintainability gate failed.
  - Adjusting thresholds or gate semantics for pyguard or tsguard.
---

# Complexity Metrics

oxguard uses several complementary metrics to catch code that has grown too complex to
safely modify or reason about. This document explains what each metric measures, why it
matters, and what the thresholds mean in practice.

---

## Cyclomatic Complexity (pyguard + tsguard)

**Tool:** radon cc (Python) / biome cognitive complexity (TypeScript)
**Threshold:** max 10 per function (radon CC grade ≤ C)

Counts the number of independent paths through a function — roughly, the number of
`if`, `for`, `while`, `and`, `or`, and `except` branches.

A function with CC 1 has a single straight path. CC 10 means there are 10 paths to test
and reason about. Beyond that, the function is statistically likely to contain bugs and is
hard to modify safely.

**Why 10?** It's the threshold where research shows defect rates increase sharply. Functions
above 10 should be split into smaller, named pieces.

---

## Maintainability Index (pyguard)

**Tool:** radon mi  
**Threshold:** grade ≥ B per file (score ≥ 65 out of 100)

A composite 0–100 score computed from three inputs:

- Halstead volume (how many concepts are in play)
- Cyclomatic complexity (how many paths exist)
- Lines of code (sheer size)

A score of 100 is perfectly maintainable. The grade scale:

| Grade | Score | Meaning |
|-------|-------|---------|
| A | 85–100 | Easy to maintain |
| B | 65–84 | Acceptable — gate passes here |
| C | 35–64 | Difficult — refactor recommended |
| D | 0–34 | Very hard to maintain |

A file can have individually simple functions (low CC) but still score low on MI if it has
hundreds of functions and a huge vocabulary — MI catches the file-level picture that
per-function checks miss.

---

## Halstead Complexity (pyguard)

**Tool:** radon hal + check_halstead.py  
**Thresholds:** effort ≤ 50,000 · difficulty ≤ 30 · bugs ≤ 0.4 per function

Halstead treats code as a vocabulary problem. It counts:

- **Distinct operators** (keywords, symbols: `+`, `if`, `return`, …)
- **Distinct operands** (variables, literals)
- **Total uses** of each

From those counts it derives:

| Metric | What it means | Threshold |
|--------|--------------|-----------|
| **Difficulty** | How hard the function is to write correctly — grows with operator variety and operand reuse | ≤ 30 |
| **Effort** | Mental effort required to understand the function (difficulty × volume) | ≤ 50,000 |
| **Delivered bugs** | Estimated latent bugs based on effort (Halstead's empirical formula: effort^⅔ / 3000) | ≤ 0.4 |

A function that passes cyclomatic complexity but fails Halstead has too many distinct
concepts crammed into one place — even if the control flow is linear, the cognitive load
makes mistakes likely.

**Test file exclusion:** `test_*.py`, `*_test.py`, `conftest.py`, and the `tests/`
directory are excluded from the radon gates (cc, mi, hal), the Halstead script,
complexipy, the runtime module graph and the function graph by default. For complexipy, which has no directory `--ignore`, the exclusion
globs also cover root-level matches and whole `tests/` or `test/` directories at any
depth. Test files are intentionally repetitive — many similar cases, parameterized
fixtures — so complexity metrics don't apply meaningfully. Ruff and coverage still gate
test code. Configure in `pyproject.toml`:

```toml
[tool.pyguard]
exclude-tests = false          # re-enable complexity gating on test files
exclude = ["*/fixtures/*"]     # add project-specific patterns without touching the default set
```

---

## Cognitive Complexity (pyguard, advisory)

**Tool:** complexipy 8.3.0  
**Threshold:** 20 per function (advisory in this release)

Cognitive complexity counts how much a reader has to keep in mind while following a
function: nesting, breaks in the linear flow, boolean operator sequences, recursion and
nested callback boundaries. Unlike cyclomatic complexity it penalizes deeply nested code
more than a flat sequence of independent branches, because nesting makes the same number
of paths harder to hold in working memory.

Each function produces one `complexipy.cognitive` measurement at level `code` with unit
`score`. A score above 20 yields the warning `pyguard.complexipy.cognitive_exceeded`;
exactly 20 is clean. In this release the warning never changes the exit code or the
gate/overall pass–fail status, even under strict gate expectations. The next release makes
it blocking, so extract helpers or flatten nesting now for functions above 20.

complexipy folds nested functions into their enclosing function: a nested `def` has no
separate report row, and its complexity is counted toward the outer function's score.
PyGuard keeps exactly one measurement per complexipy-reported function (normally the outer
symbol), so a complex nested helper warns against the function that contains it rather
than disappearing or appearing twice. Extract the nested definition to a top-level
function when the folded score exceeds 20.

The analyzer is pinned exactly at 8.3.0 and is managed like radon and pyan3: it is in the
PyGuard dev-dependency manifest, the hash-locked Python evaluation toolchain, and
`pyguard doctor` reports its version. `pyguard diagnose` exposes it as a separate
`complexipy.cognitive` analysis.

---

## Runtime module graph (pyguard, advisory)

**Tool:** grimp 3.18 + NetworkX 3.7
**Command:** `pyguard structure` (also `pyguard audit` and the `structure.module_graph` `pyguard diagnose` analysis; not `pyguard check`)

PyGuard builds a graph of in-scope runtime imports between Python modules. External and third-party imports are not edges, and imports outside the configured scope are not inferred. The graph emits the same project-level measurements as Tsguard:

| Metric | Meaning | Unit |
|---|---|---|
| `module.count` | In-scope Python modules, including package `__init__.py` files | `modules` |
| `module.edges` | Unique in-scope runtime import edges after type-only/deferred imports are removed | `edges` |
| `module.cycles` | Runtime strongly connected components with more than one module, plus self-loops | `components` |
| `module.type_imports` | Import statements under `if TYPE_CHECKING:` / `typing.TYPE_CHECKING` | `references` |
| `module.deferred_imports` | Import statements inside a function or method body | `references` |
| `module.dependency_depth` | Per module: longest path in the SCC condensation DAG, measured in edges; internal cycle edges add no depth | `edges` |
| `module.fan_in` | Per module: distinct in-scope runtime modules that import it; a self-import is not counted | `modules` |
| `module.fan_out` | Per module: distinct in-scope runtime modules it imports; a self-import is not counted | `modules` |

A runtime cycle yields one advisory `pyguard.structure.CIRCULAR_DEPENDENCY` warning at level `structure`, with the first module as its location and the remaining modules as `related`. A self-import is a one-module cycle. A module with at least 3 in-scope runtime importers (`module.fan_in`) and at least 8 in-scope runtime dependencies (`module.fan_out`) also yields one advisory `pyguard.structure.HIGH_MODULE_COUPLING` warning at level `structure`, with the module as its location. Both thresholds are fixed constants with no configuration, and the warning is a review cue: counts alone do not prove poor cohesion or prescribe an architecture. Neither warning changes the exit code, blocking outcome or strict expectations; `pyguard structure` exits 0 for advisory findings. As in Tsguard, a project whose only findings are advisory reports overall status `advisory` instead of `pass` and an advisory gate instead of `passed`. Fan-in and fan-out use distinct in-scope runtime modules only: `TYPE_CHECKING`-only, deferred function/method-local and external imports do not count, out-of-scope/excluded/test modules are boundaries, and a self-import remains one runtime edge and one cycle but is not counted as its own importer or dependency. This release adds coupling; layer and boundary rules are not added.

Python-specific controls are explicit. Imports guarded by `TYPE_CHECKING` are excluded from runtime edges and counted as type imports. Imports inside a function or method body are deferred: they execute only when that callable runs, so they are excluded from edges/cycles and counted separately. Class-body imports execute at import time and remain runtime imports. An import of an out-of-scope package is external, not unresolved. Import roots come from the project layout, independently of the requested graph scope: files under `<project>/src` use `src` as their root (including the default `.` scope and a narrowed `--dirs` path inside `src`), and every other file uses the project root. Module identities are the full dotted path beneath those fixed roots, including implicit namespace directories; files are never renamed or promoted to match an import name. Resolution inventories every `.py` file under the roots, so an existing out-of-scope target is a scope boundary: it creates no edge, is not missing and gets no parent-package fallback edge. `from X import name` validates `name` against the full inventory (a submodule, subpackage, or an AST-defined/exported name in X's `__init__.py`) before the boundary applies, so an out-of-scope parent never hides a missing child. Export validation is a deliberate over-approximation: a name counts as exported when any statement in the package `__init__` AST can bind it, at any nesting level, including ordinary `if`/`elif`/`else` branches, `try`, `while`, `match`, `with`, function bodies (including `global` declarations), `Store` names, `def`/`class` names, import aliases and except/match targets. Only bodies of `if TYPE_CHECKING:` blocks and annotation-only `AnnAssign` statements are excluded. It also accepts Python-provided module attributes such as `__path__`, `__name__`, `__spec__` and `__file__`, and any name when the package `__init__` defines module-level `__getattr__`, performs a runtime `from ... import *`, or assigns through `globals()`/`setattr()`. Because exports are over-approximated, only a clearly missing name makes the result incomplete. Only imports that resolve nowhere locally are unresolved. `from . import x` and `from pkg import x` validate `x` as a child of a namespace package. Unresolved modules, including invalid relative imports beyond the top-level package, are withheld from grimp; a grimp resolver panic becomes incomplete `not_evaluated` evidence rather than an execution failure. If two files map to one identity, the first deterministic file is kept and the assessment becomes incomplete with `pyguard.structure.not_evaluated`. A missing module inside a project package root or an unparsable source file also leaves the assessment incomplete; it is not a crash or a clean pass. Source parsing honors a UTF-8 BOM and PEP 263 encoding declarations by reading bytes, matching grimp's own decoding and line numbers. Missing/broken grimp or NetworkX is an execution error.

Source scope follows `--dirs`, `[tool.pyguard] exclude` and the default test-file/directory exclusion. `--dirs`, exclusion globs and test filters choose graph nodes only; they do not remove files from local import resolution. grimp is pinned exactly at 3.18 in the dev-dependency manifest and hash-locked evaluation toolchain; `pyguard doctor` reports its version.

The graph is static evidence, not proof of runtime reachability. Dynamic imports, import hooks and conditional package availability remain outside the graph; the missing-local/unparsable controls keep incomplete evidence visible rather than turning it into a pass.

---

## Function call graph (pyguard, advisory)

**Tool:** pyan3 2.9.0 + NetworkX 3.7
**Command:** `pyguard structure` (also `pyguard audit` and the `structure.module_graph` `pyguard diagnose` analysis; not `pyguard check`)

The same command builds a graph of resolved calls between defined in-scope Python functions. Nodes are `FunctionDef`/`AsyncFunctionDef` definitions in the selected non-test files, including methods and nested functions; call edges come from pyan3's function-use edges restricted to those definitions. External, out-of-scope and module-level calls are not edges. Repeated calls from one function remain one edge and one caller/callee. Every function produces these measurements at location file, line and symbol:

| Metric | Meaning | Unit |
|---|---|---|
| `function.fan_in` | Distinct in-scope defined functions that call it; a self-call is not counted | `callers` |
| `function.fan_out` | Distinct in-scope defined functions it calls; a self-call is not counted | `callees` |
| `function.call_depth` | Longest path in the call SCC condensation DAG, measured in edges; internal cycle edges add no depth | `edges` |

Two project measurements use location `.`:

| Metric | Meaning | Unit |
|---|---|---|
| `function.recursive_components` | Call SCCs with more than one function, plus self-recursive functions | `components` |
| `calls.unresolved` | Own-scope syntactic call sites that are not a resolved in-scope callee, builtin or imported name/attribute chain | `calls` |

`calls.unresolved` is a deterministic count of syntactic call sites, not a pyan3 placeholder/wildcard edge count. For every defined in-scope function node it visits `ast.Call` expressions in that function's own scope only: nested `FunctionDef`/`AsyncFunctionDef`/`ClassDef` bodies are not descended into because they are their own callers, while lambda and comprehension bodies stay with the enclosing function and a nested definition's decorators/defaults (and annotations) belong to the enclosing scope. A call site is resolved when its target is a `Name` or `Attribute` whose terminal name (`Name.id` or `Attribute.attr`) equals the short name of a callee in that caller's resolved in-scope function edges (including a self-call), or a `Name` that is a Python builtin from the `builtins` module, or a `Name`/`Attribute` chain rooted at a name bound by an import statement in that module (an external or imported API boundary). Every other call site is unresolved, including subscript or call-result targets such as `items[0]()` and `f()()`, local aliases such as `saved = callback; saved()`, parameters and closure variables, and attribute receivers whose name is not a resolved in-scope callee or imported root such as `obj.run()`. Each syntactic call site counts once and a receiver is never a separate call site. There is no alias or dataflow tracking; this is a syntactic approximation and may over- or under-count dynamic dispatch. The measurement is evidence only: unresolved calls never make the assessment incomplete and the result remains complete unless a source file or module resolution limitation is separately reported.

A self-call is one call edge and one recursive component, but a function is neither its own caller nor its own callee. Therefore `function.fan_in` equals the unchanged `pyguard criticality` `criticality.in_degree` except for self-recursive functions, where criticality counts the self-call once and function fan-in counts zero. Both metrics are retained; `pyguard criticality` and its in-degree semantics are unchanged.

The function graph emits no findings and never changes the exit code, status or blocking outcome. An SCC, recursion, high fan-in or unresolved call is not by itself a quality violation. `tokenize.detect_encoding` decides pyan3 normalization: a source whose declared or BOM encoding is not plain UTF-8, including non-UTF-8 bytes that also decode as UTF-8, is copied into a temporary UTF-8 mirror for pyan3 while reported locations stay the original paths and line numbers and pyan3's original inferred import root is preserved, including when an ancestor `__init__.py` marker is outside the requested scope; a file pyan3 cannot parse leaves the assessment incomplete with `pyguard.structure.not_evaluated`, as in the module graph. Missing/broken pyan3 or NetworkX is an execution error. Source scope follows `--dirs`, `[tool.pyguard] exclude` and the default test-file/directory exclusion exactly like the module graph.

---

## Zero-config exception handling (pyguard)

**Tool:** Ruff S110 + B012

**Policy:** blocking only when the project root has no Ruff configuration

When the project has no Ruff configuration, pyguard adds `--extend-select S110,B012`
to the Ruff lint invocations it owns and records one `ruff.baseline_version`
measurement at level `code`, location `.`, value `1`, unit `version`. The two rules
are blocking through the normal Ruff adapter, so they fail the ruff gate and change
the exit code.

| Rule | Catches | Typical fix |
|---|---|---|
| S110 | A `pass`-only handler for bare `except:`, `except Exception:` or `except BaseException:`. Ruff's default `check-typed-exception = false` means typed handlers such as `except ValueError: pass` are not flagged | Log the failure, re-raise it, or narrow the handler and document why it is safe to ignore |
| B012 | `return`, `break` or `continue` in a `finally` block | Move the control-flow statement out of `finally`; it can silently discard a pending exception |

Configuration detection covers `[tool.ruff]` and every `[tool.ruff.*]` table in
`pyproject.toml`, plus `ruff.toml` and `.ruff.toml` at the project root. A configured
project is run exactly as before: no extra flags and no baseline measurement. This
matters because Ruff's CLI `--extend-select` overrides project `ignore` entries, so a
project that ignores S110 deliberately must keep that ignore. Unreadable or malformed
configuration is treated conservatively as configured, preserving the existing Ruff
invocation rather than injecting CLI overrides. Nested Ruff config files do not count
for this root decision; Ruff still discovers them itself.

Ruff's native semantics are retained: `# noqa: S110` is respected, S112 is not added,
and an ellipsis handler (`except Exception: ...`) is not reported by S110. S110 also
keeps Ruff's default `check-typed-exception = false`, so only bare and
`Exception`/`BaseException` handlers with a `pass`-only body are reported;
`except ValueError: pass` is a documented limitation and the harness exercises it as a
boundary. The baseline does not exclude test files; unlike the complexity analyzers,
Ruff continues to gate conventional test code.

---

## FTA Score (tsguard)

**Tool:** `fta` (from the `fta-cli` package)  
**Threshold:** ≤ 60 per file

FTA (Fast TypeScript Analyzer) is the TypeScript equivalent of pyguard's blocking complexity
gates combined into one normalized per-file score. It computes:

- Halstead volume
- Cyclomatic complexity
- Lines of code

…and normalizes them into a single 0–100+ score. The official
[FTA scoring](https://ftaproject.dev/docs/scoring) assessments:

| FTA Score | Assessment |
|-----------|------------|
| < 50 | OK — considered maintainable |
| 50–60 | Could be better — reasonably maintainable |
| > 60 | Needs improvement — difficult to maintain |

The default gate fails when a file scores above 60, matching the official
"Needs improvement" boundary. It sits at 60 rather than 50 to allow real-world TypeScript files (which tend
toward more boilerplate than Python) a little more headroom, while still blocking files
that have grown genuinely hard to change safely. Tune it per project with
`--max-fta-score <n>` or `fta-score-cap = <n>` in `oxguard.toml`.

Because FTA operates per-file, it catches the same class of problem as pyguard's MI — a
file that accumulates too much responsibility over time. The score is per file, so
splitting a file only to lower its score does not reduce complexity; smaller files or
lower scores alone do not prove simplification.

**Test file exclusion:** `*.test.*` and `*.spec.*` files are excluded from FTA scoring by
default — test files are intentionally repetitive (many similar test cases, property
generators, fixtures) so FTA's maintainability index doesn't apply meaningfully to them.
Lint and coverage gates still enforce quality on test code. To add project-specific
patterns or re-enable test scoring, use `oxguard.toml`:

```toml
fta-exclude-tests = false          # re-enable FTA on all test files
fta-exclude = ["*.pbt.ts"]         # add project-specific patterns without touching the default set
```

---

## Type Annotation Complexity (pyguard)

**Tool:** check_type_complexity.py  
**Thresholds:** nesting depth ≤ 2 · annotation length ≤ 40 characters

Catches type hints that technically pass mypy but hide data structure behind complexity:

```python
# fails — depth 3, length >> 40
results: dict[str, list[dict[str, str]]]

# passes — or better, name the shape
class UserRecord(TypedDict):
    name: str
    tags: list[str]

results: dict[str, UserRecord]
```

Deep nesting means the type hint is doing the job that a named type should do. Enforcing
a depth limit pushes toward explicit, readable data models.

The same test-file exclusion that applies to the radon gates (`test_*.py`, `*_test.py`,
`conftest.py`, `tests/`) also applies here — test fixtures and parametrize helpers often
use complex type annotations by design.

## Tsguard function context

`function.branches` counts compiler branch constructs. `function.max_branch_nesting` measures their maximum structural nesting; callbacks start a new function boundary. `branch_locations` in the maintainability graph identifies the source paths. These are observations, not per-function FTA or cognitive scores. Structured FTA failures attach this context for the first failing file while preserving the native score/cap failure.

Native Level 3 Biome cognitive measurements remain exact above 1 and bounds [0,1] below it. Compare the whole affected graph and native distributions before describing a refactor as simplification. Async forwarding mode records return versus return-await; it does not prove equivalent timing or stack behavior.
