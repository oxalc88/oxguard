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
directory are excluded from the radon gates (cc, mi, hal), the Halstead script and
complexipy by default. For complexipy, which has no directory `--ignore`, the exclusion
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
