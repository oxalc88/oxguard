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

---

## FTA Score (tsguard)

**Tool:** fta-cli  
**Threshold:** ≤ 60 per file

FTA (Fast TypeScript Analyzer) is the TypeScript equivalent of pyguard's three complexity
gates combined into one normalized per-file score. It computes:

- Halstead volume
- Cyclomatic complexity
- Lines of code

…and normalizes them into a single 0–100+ score. The tiers:

| Score | Label |
|-------|-------|
| 0–24 | Low complexity |
| 25–49 | Moderate |
| 50–74 | High — gate fails at 60 |
| 75+ | Very high |

The threshold sits at 60 rather than 50 to allow real-world TypeScript files (which tend
toward more boilerplate than Python) a little more headroom, while still blocking files
that have grown genuinely hard to change safely.

Because FTA operates per-file, it catches the same class of problem as pyguard's MI — a
file that accumulates too much responsibility over time.

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
