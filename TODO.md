# oxguard — Distribution + Auto-Install TODO

## Goal
Make oxguard adoptable with a single `curl | sh` command plus one `setup` call —
no cloning, no manual dependency wiring.

---

## Tasks

### Repo cleanup
- [ ] `git rm --cached` stale tracked binaries (`pyguard/pkn`, `pyguard/tsguard`, `pyguard/pyguard`, `tsguard/kps`, `tsguard/tsguard`)
- [ ] Expand `.gitignore` to block legacy names (`pkn`, `kps`), cross-builds, and `*.exe` patterns

### Core binary improvements
- [ ] Add `--version` flag to `tsguard/main.go` and `pyguard/main.go` (required by install.sh verification step)
- [ ] Add `--yes` / `-y` flag to both `main.go` files (CI auto-skip for prompts)
- [ ] Fix `tsguard/hooks.go:229` — append `.exe` on Windows (mirrors pyguard pattern)
- [ ] Fix `tsguard/hooks.go:245` — use PowerShell-aware Claude hook command on Windows

### tsguard: auto-install devDependencies
- [ ] Write `tsguard/deps.go`
  - `requiredNpmDevDeps` manifest (ultracite, @biomejs/biome, typescript, vitest, @vitest/coverage-v8, fta-cli, knip, jscpd)
  - `missingNpmDevDeps(root, manifest)` — detect via package.json parse
  - `ensureNpmDevDeps(root, cfg)` — prompt + `npm install --save-dev`
  - `ensurePythonHelperTools(cfg)` — install semgrep + detect-secrets via `uv tool install` → `pipx install` → hint
- [ ] Update `tsguard/setup.go` — renumber steps 1–5, insert manifest step (before npm install) and Python tools step (after)

### pyguard: auto-install dev-group + embed scripts
- [ ] Write `pyguard/deps.go`
  - `requiredUvDevDeps` manifest (ruff, mypy, pytest, pytest-cov, radon, bandit, pip-audit, detect-secrets, vulture, deptry, pyan3, networkx, semgrep)
  - `missingUvDevDeps(root, manifest)` — detect via pyproject.toml parse (BurntSushi/toml)
  - `ensureUvDevDeps(root, cfg)` — prompt + `uv add --group dev`
- [ ] Write `pyguard/scripts.go` — `//go:embed analysis/*.py`, `ensureAnalysisScripts(root, cfg)` (SHA256 diff, overwrite or prompt)
- [ ] Add `github.com/BurntSushi/toml` to `pyguard/go.mod`
- [ ] Update `pyguard/setup.go` — renumber steps 1–8, insert manifest step (before uv sync) and scripts step (after)

### Distribution
- [ ] Write `install.sh` — OS/arch detection, GitHub API version resolution, SHA256 verify, install to `~/.local/bin`, PATH hint, `--version` verification
- [ ] Write `install.ps1` — PowerShell mirror (zip, `Get-FileHash`, `$env:LOCALAPPDATA\Programs\oxguard`)

### CI / GitHub Actions
- [ ] Write `.github/workflows/release.yml` — 5-platform build matrix, tarball packaging, checksums.txt, GitHub Release publish
- [ ] Write `.github/workflows/smoke.yml` — post-release install + setup + doctor smoke test

### Docs
- [ ] Write `docs/AI_INSTALL.md` — two variants (tsguard / pyguard), copy-paste agent prompts with install → setup → doctor → check flow
- [ ] Update `README.md` — add curl install one-liners + "Install via AI agent" section linking to AI_INSTALL.md

---

## Verification checklist (run before tagging a release)

- [ ] Fresh TS project on Linux → `curl … | sh -s -- tsguard` → `setup --yes` → all 8 devDeps in package.json → `doctor` passes
- [ ] Fresh Python project on Linux → `curl … | sh -s -- pyguard` → `setup --yes` → 13 dev-group entries → 5 analysis scripts in `tools/analysis/` → `doctor` passes
- [ ] macOS Apple Silicon — repeat above with `darwin-arm64` tarball
- [ ] Windows — `install.ps1` → `.exe` hook paths correct → `doctor` passes
- [ ] Partial TS project (some deps present) → `setup` detects only the gap → `n` aborts cleanly, `y` installs only missing
- [ ] Idempotency — re-run `setup --yes` → no changes, no npm/uv invocation
- [ ] Analysis script update — edit a script in `tools/analysis/`, re-run `pyguard setup --yes` → overwritten
- [ ] AI-agent prompt — paste tsguard variant into Claude Code → agent installs + reports gate summary
- [ ] CI=true → `setup --yes` skips all prompts, clean exit
