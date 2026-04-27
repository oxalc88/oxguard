# oxguard

Centralized quality-gate CLIs for Python and TypeScript projects. Both tools
are Go binaries derived from
[tortastudios/python-ai-guardrails-template](https://github.com/tortastudios/python-ai-guardrails-template).

## Tools

### `pkn` — Python quality gate

`pkn/` is the CLI for Python projects. Runs:

| Gate | Tool | What it enforces |
|---|---|---|
| Lint + format | ruff | Style, imports, common bugs |
| Types | mypy (strict) | Full annotation coverage |
| Cyclomatic complexity | radon cc | CC grade ≤ C (max 10) per function |
| Maintainability index | radon mi | MI grade ≥ B per file |
| Halstead complexity | radon hal + check_halstead.py | effort ≤ 50k, difficulty ≤ 30, bugs ≤ 0.4 per function |
| Type annotation complexity | check_type_complexity.py | nesting depth ≤ 2, length ≤ 40 chars |
| Coverage | pytest + coverage | Threshold in pyproject.toml |
| Security — static | bandit | eval(), shell=True, weak crypto, etc. |
| Security — CVEs | pip-audit | Known vulnerabilities in deps |
| Security — secrets | detect-secrets | Credential leaks vs baseline |

Informational (advisory, never fails gate):

| Audit | Tool |
|---|---|
| Dead code | vulture |
| Dependency hygiene | deptry |
| Criticality analysis | pyan3 + networkx → CRITICALITY.md |

```bash
cd pkn && go build -o pkn .

pkn check        # full gate (ruff → mypy → radon → types → coverage → security)
pkn fix          # auto-format (ruff format + lint --fix)
pkn audit        # informational: criticality + dead code + deps
pkn security     # security gates only
pkn types        # type-annotation complexity only
```

The `pkn/analysis/` directory holds the Python helper scripts invoked by pkn at
runtime. These scripts must be present in the consuming project's
`tools/analysis/` directory — pkn calls them project-relative, e.g.
`uv run python tools/analysis/check_halstead.py`.

### `kps` — TypeScript quality gate

`kps/` is the CLI for TypeScript projects. Runs:

| Gate | Tool | What it enforces |
|---|---|---|
| Lint + format | ultracite (biome) | Style, common bugs |
| Cyclomatic complexity | biome cognitive complexity | Per-file |
| Halstead / FTA score | fta-cli | FTA score ≤ 60 per file |
| Types | tsc --noEmit | Full TypeScript type checking |
| Coverage | vitest + coverage | Threshold in vitest.config.ts |
| Security — static | semgrep | XSS, eval, path traversal, etc. (skips if not installed) |
| Security — CVEs | npm audit | Moderate+ vulnerabilities |
| Security — secrets | detect-secrets | Credential leaks vs baseline (skips if not installed) |

Informational:

| Audit | Tool |
|---|---|
| Dead code + unused deps | knip |
| Duplicate code | jscpd |

```bash
cd kps && go build -o kps .

kps check        # full gate (lint → complexity → fta → types → coverage → security)
kps fix          # auto-format (ultracite fix)
kps audit        # informational: dead code + duplicates
kps security     # security gates only
kps fta          # FTA score gate only
```

## Per-language idiomatic asymmetry

pkn and kps enforce the same quality goals but reach them with different idioms:

- **Complexity**: pkn uses radon (per-function CC + Halstead — three thresholds).
  kps uses fta-cli (per-file FTA score — single normalized threshold). Both bound
  complexity; the granularity and metric differ because the ecosystems differ.

- **Security**: pkn uses bandit (Python-specific). kps uses semgrep
  (language-agnostic, JS/TS rules). Same class of problems caught.

- **Dead code**: pkn uses vulture (Python AST). kps uses knip (TypeScript-aware,
  also catches unused deps). kps additionally runs jscpd for copy-paste detection.

## Status

This repo is currently a **snapshot** of the vendored copies. Both pakatnamu and
kapso still ship their own vendored CLIs (`pakatnamu/tools/pkn/`,
`kapso/tools/kps/`). Future work:

- Embed the Python analysis scripts in the pkn binary (`go:embed`) so consumers
  don't need to vendor `tools/analysis/` separately.
- Wire oxguard into oxStack's install verb so `oxstack install-quality python|typescript`
  builds and places the binary in `~/.local/bin/`.
- Delete the vendored copies from pakatnamu and kapso once oxguard is the sole
  development home.
