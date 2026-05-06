# oxguard

Centralized quality-gate CLIs for Python and TypeScript projects. Both tools
are Go binaries derived from
[tortastudios/python-ai-guardrails-template](https://github.com/tortastudios/python-ai-guardrails-template).

The core problem: AI generates code faster than humans can review it. oxguard
enforces quality standards on every edit — human or machine — so nothing
slips through.

## Tools

### `pyguard` — Python quality gate

`pyguard/` is the CLI for Python projects. Runs:

| Gate | Tool | What it catches |
|---|---|---|
| Lint + format | ruff | Style drift, bad imports, common mistake patterns |
| Types | mypy (strict) | Missing or wrong type annotations |
| Cyclomatic complexity | radon cc | Functions too branchy to safely modify |
| Maintainability index | radon mi | Files that have grown too tangled to maintain |
| Halstead complexity | radon hal + check_halstead.py | Functions with too many distinct concepts — error-prone to reason about |
| Type annotation complexity | check_type_complexity.py | Type hints that hide structure behind deep nesting |
| Coverage | pytest + coverage | Untested code paths |
| Security — static | bandit | `eval()`, shell injection, weak crypto, insecure defaults |
| Security — CVEs | pip-audit | Known CVEs in dependencies |
| Security — secrets | detect-secrets | Credentials accidentally committed |

Informational (advisory, never fails gate):

| Audit | Tool | What it finds |
|---|---|---|
| Dead code | vulture | Unreachable functions, unused variables |
| Dependency hygiene | deptry | Unused or missing package dependencies |
| Criticality analysis | pyan3 + networkx → CRITICALITY.md | Load-bearing functions (high fan-in, bottlenecks) — tells AI where to apply stricter standards |

```bash
cd pyguard && go build -o pyguard .

pyguard check        # full gate (ruff → mypy → radon → types → coverage → security)
pyguard fix          # auto-format (ruff format + lint --fix)
pyguard audit        # informational: criticality + dead code + deps
pyguard security     # security gates only
pyguard types        # type-annotation complexity only
```

The `pyguard/analysis/` directory holds the Python helper scripts invoked by pyguard at
runtime. These scripts must be present in the consuming project's
`tools/analysis/` directory — pyguard calls them project-relative, e.g.
`uv run python tools/analysis/check_halstead.py`.

### `tsguard` — TypeScript quality gate

`tsguard/` is the CLI for TypeScript projects. Runs:

| Gate | Tool | What it catches |
|---|---|---|
| Lint + format | ultracite (biome) | Style drift, common JS/TS mistake patterns |
| Cyclomatic complexity | biome cognitive complexity | Functions too complex to safely modify |
| Maintainability (FTA) | fta-cli | Files too complex to maintain — catches what cyclomatic alone misses |
| Types | tsc --noEmit | Type errors |
| Coverage | vitest + coverage | Untested code paths |
| Security — static | semgrep | XSS, `eval()`, path traversal, insecure patterns (skips if not installed) |
| Security — CVEs | npm audit | Known vulnerabilities in dependencies |
| Security — secrets | detect-secrets | Credentials accidentally committed (skips if not installed) |

Informational:

| Audit | Tool | What it finds |
|---|---|---|
| Dead code + unused deps | knip | Unreachable exports, unused dependencies |
| Duplicate code | jscpd | Copy-pasted blocks that should be abstracted |

```bash
cd tsguard && go build -o tsguard .

tsguard check        # full gate (lint → complexity → fta → types → coverage → security)
tsguard fix          # auto-format (ultracite fix)
tsguard audit        # informational: dead code + duplicates
tsguard security     # security gates only
tsguard fta          # FTA score gate only
```

## Per-language idiomatic asymmetry

pyguard and tsguard enforce the same quality goals but reach them with different idioms:

- **Complexity**: pyguard splits complexity into three separate gates (cyclomatic per-function,
  Halstead per-function, maintainability index per-file). tsguard combines all three into a
  single FTA score per file. See [docs/metrics.md](docs/metrics.md) for what each measures.

- **Security**: pyguard uses bandit (Python-specific). tsguard uses semgrep
  (language-agnostic, JS/TS rules). Same class of problems caught.

- **Dead code**: pyguard uses vulture (Python AST). tsguard uses knip (TypeScript-aware,
  also catches unused deps). tsguard additionally runs jscpd for copy-paste detection.
