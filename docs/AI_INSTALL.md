---
summary: Copy-paste prompts for installing and bootstrapping oxguard through AI coding agents.
read_when:
  - Guiding users to install tsguard or pyguard with Claude Code, Codex, Cursor, OpenCode, or Kiro.
  - Updating agent-facing setup instructions for oxguard.
---

# Installing oxguard via AI agent

Paste one of the prompts below into Claude Code, Cursor, Codex, OpenCode, Kiro, or
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

3. From the project root, run setup (--yes skips interactive prompts for CI):
     tsguard setup --yes

   Setup adds any missing devDependencies to package.json, runs npm install,
   and downloads the Opengrep SAST binary into node_modules/.cache/oxguard/.
   Setup is idempotent — safe to re-run on an existing project.
   Commit the package.json diff afterward.

4. Run the doctor and show me the full output:
     tsguard doctor

5. Run a check and summarize which gates passed and which failed.
   Do NOT attempt to fix any code yet — just report:
     tsguard check

   Behavior notes for interpreting the output:
   - tsguard scans from the project root (.) by default. node_modules, dist,
     .next, build, coverage, and AI tool dirs (.claude, .opencode, .kiro, .agents)
     are always excluded. If you need to restrict to specific subdirectories,
     pass --dirs src,lib. To add more exclusions, pass --exclude extra,dirs.
   - If the SAST gate prints [SKIP], the Opengrep binary was not downloaded yet —
     that is not a failure. Run `tsguard setup` again to fetch it, then re-run check.
   - You can persist custom dirs/excludes/thresholds in an oxguard.toml file at the
     project root instead of passing CLI flags every time. CLI flags always win.

6. Tell the user that to configure their AI tool hooks and skill files they should
   run `tsguard hooks` interactively and select their tools from the menu.
   (setup --yes skips this step because stdin is not a TTY in agent mode.)

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

3. From the project root, run setup (--yes skips interactive prompts for CI):
     pyguard setup --yes

   Setup will:
   - Add missing dev-group packages to pyproject.toml and run uv sync
   - Deploy analysis helper scripts to tools/analysis/ (commit these — pyguard
     invokes them at runtime; they must live in the project repo)
   - Create .secrets.baseline

   Setup is idempotent — safe to re-run on an existing project.
   Commit the pyproject.toml diff and the new tools/analysis/ files afterward.

4. Run the doctor and show me the full output:
     pyguard doctor

5. Run a check and summarize which gates passed and which failed.
   Do NOT attempt to fix any code yet — just report:
     pyguard check

   Behavior notes for interpreting the output:
   - pyguard scans from the project root (.) by default. __pycache__, .venv,
     node_modules, and common build dirs are always excluded. If you need to
     restrict to specific subdirectories, pass --dirs src,lib.

6. Tell the user that to configure their AI tool hooks and skill files they should
   run `pyguard hooks` interactively and select their tools from the menu.
   (setup --yes skips this step because stdin is not a TTY in agent mode.)

Report a concise summary: gates passed, gates failed, any install issues.
```

---

## Updating an existing installation

To upgrade to the latest version, re-run the install script — it overwrites the binary in place:

```bash
curl -fsSL https://github.com/oxalc88/oxguard/releases/latest/download/install.sh | sh -s -- tsguard
curl -fsSL https://github.com/oxalc88/oxguard/releases/latest/download/install.sh | sh -s -- pyguard
```

Then run setup again to pick up any new devDependencies or updated Opengrep binary:

```bash
tsguard setup --yes   # idempotent — only adds what's missing
pyguard setup --yes
```

To redeploy updated skill files to your AI tools (e.g. after a version bump ships new SKILL.md content):

```bash
tsguard hooks   # interactive — re-select your tools to overwrite stale skill files
pyguard hooks
```

**AI agent prompt for updating:**

```
tsguard is already installed on this project. Update to the latest version:

1. Re-install the binary:
     curl -fsSL https://github.com/oxalc88/oxguard/releases/latest/download/install.sh | sh -s -- tsguard

2. Re-run setup to sync devDependencies and update the Opengrep binary:
     tsguard setup --yes
   (setup is idempotent — only adds what is missing, never removes existing config)

3. Run doctor to confirm everything is healthy:
     tsguard doctor

4. Tell the user to run `tsguard hooks` interactively if they want to redeploy
   updated AI tool skill files. (setup --yes skips the hook/skill installer
   because stdin is not a TTY in agent mode.)

Report the new version and any changes to the doctor output.
```

---

## Notes for the agent

- **setup is idempotent** — it adds only what's missing. Re-running it on an
  existing project is safe and produces no diff if everything is already in place.
- **Opengrep SAST binary** — `tsguard setup` downloads a self-contained binary
  (~50 MB) into `node_modules/.cache/oxguard/opengrep`. No Python, no pip, no
  global writes. If the binary is absent when `tsguard check` runs, the SAST gate
  prints `[SKIP]` and continues — run `tsguard setup` to download it.
- **pyguard's analysis scripts** (`tools/analysis/*.py`) should be committed to
  your project repo alongside pyproject.toml — they are runtime helpers that
  pyguard invokes during checks.
- **AI tool hooks and skills** — `tsguard setup --yes` and `pyguard setup --yes`
  skip the interactive AI tool picker (stdin is not a TTY in agent mode). To deploy
  hook configs and skill files, the user must run `tsguard hooks` / `pyguard hooks`
  interactively. Informing the user of this is step 6 in the install prompts above.
- **Default scan scope** — both tools scan from the project root (`.`) by default.
  `node_modules`, `dist`, `.next`, `build`, `coverage`, and AI tool dirs are always
  excluded. Pass `--dirs src,lib` to restrict to specific subdirectories.
- **`oxguard.toml`** (tsguard only) — projects can create an `oxguard.toml` at the
  repo root to set default `dirs`, `exclude`, `fta-score-cap`, and `timeout` without
  passing CLI flags every time. CLI flags always win over the file.
- **Resuming on a project with partial setup**: the prompt is identical. Setup
  detects what's already present and only adds the gap. Review the diff before
  committing to confirm it only adds what's expected.
