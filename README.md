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
| Cognitive complexity (advisory) | complexipy 8.3.0 | Functions whose nesting and callback boundaries exceed a reader's working memory — warning only above 20 |
| Type annotation complexity | check_type_complexity.py | Type hints that hide structure behind deep nesting |
| Coverage | pytest (or unittest via pytest) — 80% floor | Untested code paths |
| Security — static | bandit | `eval()`, shell injection, weak crypto, insecure defaults |
| Security — CVEs | pip-audit | Known CVEs in dependencies |
| Security — secrets | detect-secrets | Credentials accidentally committed |

Informational (advisory, never fails gate):

| Audit | Tool | What it finds |
|---|---|---|
| Dead code | vulture | Unreachable functions, unused variables |
| Dependency hygiene | deptry | Unused or missing package dependencies |
| Criticality analysis | pyan3 + networkx → CRITICALITY.md | Load-bearing functions (high fan-in, bottlenecks) — tells AI where to apply stricter standards |
| Structure analysis | grimp 3.18 + networkx | Runtime import cycles, TYPE_CHECKING/deferred-import controls and SCC dependency depth — advisory |

```bash
pyguard check        # full gate (ruff → mypy → radon → types → coverage → security)
pyguard fix          # auto-format (ruff format + lint --fix)
pyguard audit        # informational: criticality + structure + dead code + deps
pyguard structure    # advisory runtime module graph: cycles and dependency depth
pyguard security     # security gates only
pyguard types        # type-annotation complexity only
pyguard diagnose --output json  # collect all independent analyses before repair, no installs
```

**Cognitive complexity — advisory:** `pyguard radon` also runs complexipy 8.3.0 and emits one `complexipy.cognitive` measurement per function with a threshold of 20. A function scoring above 20 produces the warning `pyguard.complexipy.cognitive_exceeded`; exactly 20 is clean. In this release the warning never changes the exit code or the gate/overall pass–fail status, but the next release makes it blocking, so review such functions now. `pyguard doctor` reports the pinned version. See [docs/metrics.md](docs/metrics.md).

**Zero-config exception handling:** when the project root has no Ruff configuration, PyGuard adds Ruff's blocking S110 and B012 rules to lint (`--extend-select S110,B012`) and records `ruff.baseline_version`. A `pass`-only handler on a bare `except:`, `except Exception:` or `except BaseException:`, or a `return`/`break`/`continue` in `finally`, therefore fails the ruff gate. Ruff's default `check-typed-exception = false` means a typed handler such as `except ValueError: pass` is not flagged. Add `[tool.ruff]` (or `ruff.toml`/`.ruff.toml`) to own lint policy, including ignores; configured projects run exactly as before. `# noqa: S110` remains effective. See [docs/metrics.md](docs/metrics.md).

**Runtime module graph — advisory:** `pyguard structure` builds the in-scope runtime import graph with grimp 3.18 and NetworkX, emits `module.count`, `module.edges`, `module.cycles`, `module.type_imports`, `module.deferred_imports` and per-module `module.dependency_depth`, and reports one advisory `pyguard.structure.CIRCULAR_DEPENDENCY` per runtime SCC (including self-loops). Imports under `if TYPE_CHECKING:` are type-only and never create runtime edges; imports inside function or method bodies are deferred and also excluded from edges/cycles, while remaining counted. External imports are not edges. Import roots come from the project layout, not the requested graph scope: files under `<project>/src` use `src` as their root (including the default `.` scope and a narrowed `--dirs` path inside `src`), and every other file uses the project root. Identities are full dotted paths below those fixed roots (namespace directories included) and are never renamed from import names. Resolution sees every Python file under those roots, so an existing out-of-scope target is a boundary with no edge rather than a missing module; scope filters only choose graph nodes. `from X import name` validates `name` against the full inventory before that boundary applies; besides real submodules and subpackages, export validation is a deliberate over-approximation: a name counts as exported when any statement in the package `__init__` AST can bind it, at any nesting level, including both branches of ordinary `if`/`elif`/`else`, `try`, `while`, `match`, `with`, function bodies (including `global` declarations), `Store` names, `def`/`class` names, import aliases and except/match targets, while only `if TYPE_CHECKING:` bodies and annotation-only `AnnAssign` statements are excluded. It also accepts Python-provided module attributes such as `__path__`, `__name__`, `__spec__` and `__file__`, and any name when the package `__init__` defines module-level `__getattr__`, performs a runtime `from ... import *`, or assigns through `globals()`/`setattr()`. Because exports are over-approximated, only a clearly missing name makes the result incomplete. Unresolved local imports (including relative imports beyond the top-level package) are withheld from grimp and stay incomplete evidence rather than becoming execution failures. A missing local module, duplicate module identity or unparsable source leaves the assessment incomplete with `pyguard.structure.not_evaluated`; source parsing honors a UTF-8 BOM and PEP 263 encoding declarations by reading bytes, so valid non-UTF-8 modules keep their imports and grimp line numbers; missing grimp/NetworkX is an execution error. Cycles never change the exit code or blocking outcome; like Tsguard, they can change an otherwise passing overall/gate status to `advisory`. The command also runs in `pyguard audit` and `pyguard diagnose`; it is not part of `pyguard check`. `pyguard doctor` reports the pinned grimp version. See [docs/metrics.md](docs/metrics.md).

**Complexity gates — test file exclusion:** `test_*.py`, `*_test.py`, `conftest.py`, and
the `tests/` directory are excluded from radon cc/mi/hal, complexipy cognitive, Halstead,
and type-annotation complexity gates by default. For complexipy, which has no directory
`--ignore`, path globs also cover root-level matches and whole `tests/` or `test/`
directories at any depth. Test files are intentionally repetitive; complexity metrics
don't apply meaningfully to them. Ruff and coverage gates still cover test code.

Configure per-project in `pyproject.toml`:

```toml
[tool.pyguard]
exclude-tests = false          # re-enable complexity gating on test files
exclude = ["*/fixtures/*"]     # add project-specific patterns (e.g. fixtures, stubs)
```

pyguard embeds Python helper scripts (from `pyguard/analysis/`) in its binary
and executes private temporary copies. `pyguard setup` also deploys reference
copies to the project's `tools/analysis/`; these are not executed during checks.

### `tsguard` — TypeScript quality gate

The current implementation provides three Tsguard levels: **code** (Ultracite/Biome, native typed lint and existing quality gates), **structure** (function/module graphs, cycles, coupling and delegation), and **change** (native FTA, cognitive complexity, duplication and graph comparisons against an inert Git baseline). Structure/change findings are advisory. Python runtime module-graph parity is implemented as an advisory subset; remaining Python structural/change parity is deferred independently. See [implemented levels and limits](docs/maintainability.md) and [validation evidence](docs/maintainability-validation.md). Project lint policies remain authoritative; missing capabilities are reported explicitly.

`tsguard/` is the CLI for TypeScript projects. Runs:

| Gate | Tool | What it catches |
|---|---|---|
| Lint + format + cognitive complexity | ultracite (`ultracite/biome/core`) | Style drift, common JS/TS mistake patterns, excessive cognitive complexity |
| Maintainability (FTA) | `fta` (from `fta-cli`) | Files too complex to maintain — catches what cyclomatic alone misses |
| Types | tsc --noEmit | Type errors |
| Compiler-aware lint | Oxlint/tsgolint, five semantic rules only | Unsafe assignments/assertions and promise misuse; unnecessary conditions are advisory; unsupported native configurations remain incomplete |
| Coverage | vitest / jest / mocha+c8 / ava+c8 — 80% floor | Untested code paths |
| Security — static | Opengrep (project-local binary, LGPL) | XSS, `eval()`, path traversal, weak crypto, injection, framework patterns — bandit-class coverage |
| Security — CVEs | npm/pnpm/yarn audit + audit-ci | Known vulnerabilities in dependencies |
| Security — secrets | secretlint | Credentials accidentally committed |

Informational:

| Audit | Tool | What it finds |
|---|---|---|
| Dead code + unused deps | knip | Unreachable exports, unused dependencies |
| Duplicate code | jscpd | Copy-pasted blocks that should be abstracted |

```bash
tsguard check        # blocking gates, then advisory maintainability and duplication
tsguard check --baseline HEAD~1  # also compare change quality with a Git baseline
tsguard maintainability --output json  # focused function/module structure and smells
tsguard change --baseline HEAD~1 --output json  # focused native baseline comparison
tsguard fix          # auto-format (ultracite fix)
tsguard audit        # informational: criticality + dead code + duplicates
tsguard security     # security: secretlint + audit/audit-ci + opengrep SAST
tsguard typed-lint   # focused native compiler-aware lint
tsguard fta          # FTA score gate, with function context on structured failures
tsguard complexity   # compatibility alias: complexity is enforced by ultracite check
tsguard diagnose --output json  # collect all independent analyses before repair, no installs
```

Key flags:

```bash
tsguard check --dirs src,lib          # override scanned directories (default: . — project root)
tsguard check --exclude docs,scripts  # exclude additional directories from all scans
                                      # node_modules, dist, .next, build, coverage are always excluded
tsguard check --max-fta-score 50      # tighten FTA complexity cap (default: 60)
tsguard check --tail 30               # show only the last 30 lines of each tool's output
```

**FTA gate — test file exclusion:** `*.test.*` and `*.spec.*` files are excluded from FTA
scoring by default. Test files are intentionally repetitive and the maintainability index
doesn't apply meaningfully to them. Lint and coverage gates still cover test files.

Tune via `oxguard.toml` at the project root:

```toml
fta-exclude-tests = false          # re-enable FTA on test files
fta-exclude = ["*.pbt.ts"]         # add project-specific patterns (e.g. property-based tests)
```

**Coverage gate** — runner detection and threshold enforcement:

tsguard reads `package.json` to detect the test runner (devDependencies and config files).
The 80% floor is enforced regardless of project config — projects with stricter thresholds
in their own config still fail at the higher value.

| Runner detected | How coverage runs |
|---|---|
| `vitest` | `vitest run --coverage` with threshold flags |
| `jest` | `jest --coverage --coverageThreshold={"global":...}` |
| `mocha` / `ava` / `jasmine` | wrapped with `c8` or `nyc` (must be in devDependencies) |
| none found | gate fails with install hint |
| runner found, no wrapper | gate fails — add `c8` to devDependencies |

With an npm installation and no declared project runner, coverage uses the
package-owned Vitest/V8 defaults instead.

pyguard detects pytest from `pyproject.toml` dev-group dependencies and config files.
If only `unittest` test files are found (no pytest markers), pytest is used as the runner
since it discovers and runs unittest tests natively.

The security gate requires **no Python or global installs**. In npm installations,
tools are owned by `@oxguard/tsguard` and Opengrep ships in its native package.
Tsguard's secrets and SAST gates scan the project root even when `--dirs` narrows
other gates, so a restricted analysis scope cannot hide security findings.
Standalone `tsguard setup` installs project dependencies and a verified SAST engine:

- **Secrets** (`secretlint`): added as a npm devDependency by `tsguard setup` → lives in `node_modules`.
- **CVEs** (PM-native `audit` + `audit-ci`): also npm devDependencies.
- **SAST** (Opengrep): `tsguard setup` downloads a self-contained binary
  (~50 MB) into the OS user cache under `oxguard/opengrep/<version>/<os>-<arch>/`,
  outside the scanned project, and verifies its pinned SHA-256 digest before execution.
  The binary bundles its own runtime (Nuitka-compiled). A missing or untrusted engine
  blocks security checks; run `tsguard setup` to install or repair it. npm installations
  use their package-owned engine; reinstall with optional dependencies if it is missing.

## Install

Maintainers: see [Publishing oxguard](docs/guides/releases.md) for release tags,
publication checks and recovery from failed releases.

TypeScript projects can install the complete Go-powered toolchain through npm
(Node.js 22.18+ on the 22.x line, 24.x, or 26+):

```bash
npm install -D @oxguard/tsguard
npx tsguard check
npx tsguard audit
```

The npm package selects the native Go/Opengrep binaries for your OS and architecture
and owns the required analyzer dependencies. No separate setup is needed to run
the commands. Tools are transitive dependencies of `@oxguard/tsguard`, rather than
individual entries added to your project's manifest. Existing project configs take
precedence; missing configs use cache-local defaults. npm/pnpm installation runs
no setup or install hooks. Explicit `tsguard setup` verifies tools and offers hooks
in npm mode; standalone installation retains the setup behavior below. See the
[npm distribution guide](docs/guides/npm-distribution.md) for local tarball tests
and release configuration.

Standalone installation from GitHub Releases remains available:

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

pyguard needs **Python >=3.12 (3.14.1 is not supported)** on the machine that runs the gate; `pyguard doctor` verifies it.

`setup` adds all required dev dependencies to `package.json` / `pyproject.toml`,
runs `npm install` / `uv sync`, deploys pyguard's analysis helper scripts to
`tools/analysis/`, creates `.secrets.baseline`, and offers to configure your AI
tools. For each tool you select, it writes both a **hook** (so the gate runs
automatically after every file edit) and an **AI skill/instructions file** (so
your agent understands how to interpret gate output):

| AI tool | Hook written | Skill / instructions written |
|---|---|---|
| Claude Code | `.claude/settings.local.json` | `.claude/skills/tsguard/` (entry and workflows) |
| GitHub Copilot | `.github/hooks/tsguard-check.json` + `.vscode/tasks.json` | `.github/copilot-instructions.md` → `.agents/skills/tsguard/` |
| Cursor | `.cursor/hooks.json` | No separate Cursor rules; use the canonical skill |
| OpenAI Codex CLI | `.codex/hooks.json` | `AGENTS.md` → `.agents/skills/tsguard/` |
| OpenCode | `.opencode/plugins/tsguard/index.ts` + `opencode.json` | (plugin is the skill) |

*(pyguard writes the same set with `pyguard` names in each path.)*

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

If you use Claude Code, Cursor, Codex, or OpenCode, you can paste a one-shot
prompt and let your agent handle the full install + setup flow.

→ See [docs/AI_INSTALL.md](docs/AI_INSTALL.md) for copy-paste prompts.

## Per-language idiomatic asymmetry

pyguard and tsguard enforce the same quality goals but reach them with different idioms:

- **Complexity**: pyguard splits complexity into four separate gates (cyclomatic per-function,
  Halstead per-function, maintainability index per-file, and advisory cognitive complexity
  per-function). tsguard combines the first three into a single FTA score per file and adds
  its own native cognitive comparison. See [docs/metrics.md](docs/metrics.md) for what each measures.

- **Security**: pyguard runs Bandit, pip-audit and detect-secrets. Python has no implemented Opengrep `--deep` command; strict flag validation rejects it. tsguard uses Opengrep as its blocking SAST engine through a project-local binary.

- **Dead code**: pyguard uses vulture (Python AST). tsguard uses knip (TypeScript-aware,
  also catches unused deps). tsguard additionally runs jscpd for copy-paste detection.

### Tsguard machine-facing results

Use `npx --no-install tsguard check --output agent` for a bounded summary, or
`--output json` for the complete normalized result. Both support explicit
`--root` and keep raw diagnostics for drill-down. See the
[agent and JSON contract](docs/guides/tsguard-result-contract.md).

`npx --no-install tsguard criticality --output agent` ranks TypeScript functions
and methods by distinct callers and writes the top 30 to `CRITICALITY.md`.
`audit` includes this advisory analysis; blocking `check` gates are unchanged.

### Capability evals

OxGuard has known-answer corpora for Tsguard and PyGuard, baseline/candidate comparison reports and a shared capability matrix. Both CLIs share a normalized Go result contract with bounded agent and complete JSON outputs; Python retains all 15 baseline behavior checks, passes all 16 behavior checks and all 26 current parity checks. The Python maintainability parity suite (`evals/python/maintainability.cjs`) seeds criticality and complexipy cognitive-complexity cases through the built binary; the `quality.cognitive_complexity` capability is implemented, other `planned` capabilities report `not_implemented` and P8 native duplication stays `deferred`, so none of those counts as a pass. See [capability evals](docs/guides/oxguard-evals.md), [Python outputs](docs/guides/pyguard-result-contract.md) and [TypeScript outputs](docs/guides/tsguard-result-contract.md) for execution paths, adapter limits and remaining evaluation gaps.
