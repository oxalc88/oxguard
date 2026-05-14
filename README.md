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
pyguard check        # full gate (ruff → mypy → radon → types → coverage → security)
pyguard fix          # auto-format (ruff format + lint --fix)
pyguard audit        # informational: criticality + dead code + deps
pyguard security     # security gates only
pyguard types        # type-annotation complexity only
```

pyguard ships Python helper scripts (in `pyguard/analysis/`) that are invoked at
runtime. `pyguard setup` deploys them automatically to the consuming project's
`tools/analysis/` directory.

### `tsguard` — TypeScript quality gate

`tsguard/` is the CLI for TypeScript projects. Runs:

| Gate | Tool | What it catches |
|---|---|---|
| Lint + format + cognitive complexity | ultracite (`ultracite/biome/core`) | Style drift, common JS/TS mistake patterns, excessive cognitive complexity |
| Maintainability (FTA) | `fta` (from `fta-cli`) | Files too complex to maintain — catches what cyclomatic alone misses |
| Types | tsc --noEmit | Type errors |
| Coverage | vitest + coverage | Untested code paths |
| Security — static | semgrep | XSS, `eval()`, path traversal, insecure patterns |
| Security — CVEs | npm audit | Known vulnerabilities in dependencies |
| Security — secrets | detect-secrets | Credentials accidentally committed |

Informational:

| Audit | Tool | What it finds |
|---|---|---|
| Dead code + unused deps | knip | Unreachable exports, unused dependencies |
| Duplicate code | jscpd | Copy-pasted blocks that should be abstracted |

```bash
tsguard check        # full gate (lint → fta → types → coverage → security)
tsguard fix          # auto-format (ultracite fix)
tsguard audit        # informational: dead code + duplicates
tsguard security     # security gates only
tsguard fta          # FTA score gate only
tsguard complexity   # compatibility alias: complexity is enforced by ultracite check
```

The security gate (`semgrep`, `detect-secrets`) requires Python tooling. `tsguard setup`
installs both automatically via `uv tool install` (falling back to `pipx`). If the
tools are missing when `tsguard check` runs, tsguard will attempt to install them
on-the-fly before running the gate. If neither `uv` nor `pipx` is available, the gate
prints `[SKIP]` and continues — install them manually with
`uv tool install semgrep detect-secrets` to make the gate hard.

## Install

```bash
# TypeScript project
curl -fsSL https://github.com/oxalc88/oxguard/releases/latest/download/install.sh | sh -s -- tsguard

# Python project
curl -fsSL https://github.com/oxalc88/oxguard/releases/latest/download/install.sh | sh -s -- pyguard
```

Windows (PowerShell):

```powershell
& ([scriptblock]::Create((iwr -useb https://github.com/oxalc88/oxguard/releases/latest/download/install.ps1))) tsguard
& ([scriptblock]::Create((iwr -useb https://github.com/oxalc88/oxguard/releases/latest/download/install.ps1))) pyguard
```

The install script detects OS and architecture, downloads the right binary from
GitHub Releases, verifies the SHA256 checksum, and installs to `~/.local/bin`
(or `%LOCALAPPDATA%\Programs\oxguard` on Windows).

## Adding to a project

**1. Run setup — one time**

```bash
tsguard setup    # TypeScript
pyguard setup    # Python
```

`setup` adds all required dev dependencies to `package.json` / `pyproject.toml`,
runs `npm install` / `uv sync`, deploys pyguard's analysis helper scripts to
`tools/analysis/`, creates `.secrets.baseline`, and offers to wire your AI
tool's PostToolUse hook (Claude Code, Cursor, Copilot, Codex, OpenCode, Kiro) so
the gate runs automatically after every file edit.

Pass `--yes` to skip interactive prompts (for CI or non-interactive shells):

```bash
tsguard setup --yes
pyguard setup --yes
```

**2. Verify**

```bash
tsguard doctor   # checks Node.js, npm, tsc, vitest, ultracite, fta, …
pyguard doctor   # checks Python, uv, .venv, ruff, mypy, radon, …
```

**3. Run**

```bash
tsguard check
pyguard check
```

## Install via AI agent

If you use Claude Code, Cursor, Codex, OpenCode, or Kiro, you can paste a one-shot
prompt and let your agent handle the full install + setup flow.

→ See [docs/AI_INSTALL.md](docs/AI_INSTALL.md) for copy-paste prompts.

## Per-language idiomatic asymmetry

pyguard and tsguard enforce the same quality goals but reach them with different idioms:

- **Complexity**: pyguard splits complexity into three separate gates (cyclomatic per-function,
  Halstead per-function, maintainability index per-file). tsguard combines all three into a
  single FTA score per file. See [docs/metrics.md](docs/metrics.md) for what each measures.

- **Security**: pyguard uses bandit (Python-specific). tsguard uses semgrep
  (language-agnostic, JS/TS rules). Same class of problems caught.

- **Dead code**: pyguard uses vulture (Python AST). tsguard uses knip (TypeScript-aware,
  also catches unused deps). tsguard additionally runs jscpd for copy-paste detection.
