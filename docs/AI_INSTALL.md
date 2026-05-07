# Installing oxguard via AI agent

Paste one of the prompts below into Claude Code, Cursor, Codex, OpenCode, or
any agentic CLI from the root of your project. Pick the variant that matches
your project type.

---

## TypeScript project → install tsguard

```
You are setting up tsguard (TypeScript quality gate) for this project.
Follow these steps exactly, stopping and reporting if any step fails:

1. Install tsguard by running:
     curl -fsSL https://github.com/oxalc88/oxguard/releases/latest/download/install.sh | sh -s -- tsguard

   If curl is unavailable on Windows, use instead:
     & ([scriptblock]::Create((iwr -useb https://github.com/oxalc88/oxguard/releases/latest/download/install.ps1))) tsguard

2. Verify the install:
     tsguard --version

3. From the project root, run setup (--yes skips interactive prompts):
     tsguard setup --yes

   Setup will add any missing devDependencies to package.json and install them.
   It will also attempt to install semgrep and detect-secrets system-wide.
   Commit the package.json diff afterward.

4. Run the doctor and show me the full output:
     tsguard doctor

5. Run a check and summarize which gates passed and which failed.
   Do NOT attempt to fix any code yet — just report:
     tsguard check

Report a concise summary: gates passed, gates failed, any install issues.
```

---

## Python project → install pyguard

```
You are setting up pyguard (Python quality gate) for this project.
Follow these steps exactly, stopping and reporting if any step fails:

1. Install pyguard by running:
     curl -fsSL https://github.com/oxalc88/oxguard/releases/latest/download/install.sh | sh -s -- pyguard

   If curl is unavailable on Windows, use instead:
     & ([scriptblock]::Create((iwr -useb https://github.com/oxalc88/oxguard/releases/latest/download/install.ps1))) pyguard

2. Verify the install:
     pyguard --version

3. From the project root, run setup (--yes skips interactive prompts):
     pyguard setup --yes

   Setup will:
   - Add missing dev-group packages to pyproject.toml and run uv sync
   - Deploy analysis helper scripts to tools/analysis/
   - Create .secrets.baseline

   Commit the pyproject.toml diff and the new tools/analysis/ files afterward.

4. Run the doctor and show me the full output:
     pyguard doctor

5. Run a check and summarize which gates passed and which failed.
   Do NOT attempt to fix any code yet — just report:
     pyguard check

Report a concise summary: gates passed, gates failed, any install issues.
```

---

## Notes for the agent

- **setup is idempotent** — it adds only what's missing. Re-running it on an
  existing project is safe and produces no diff if everything is already in place.
- **semgrep and detect-secrets** are Python tools. For tsguard, setup installs
  them via `uv tool install` or `pipx install`. If neither is available, those
  two security gates will skip gracefully — the rest of the gate still runs.
- **pyguard's analysis scripts** (`tools/analysis/*.py`) should be committed to
  your project repo alongside pyproject.toml — they are runtime helpers that
  pyguard invokes during checks.
- **Resuming on a project with partial setup**: the prompt is identical. Setup
  detects what's already present and only adds the gap. Review the diff before
  committing to confirm it only adds what's expected.
