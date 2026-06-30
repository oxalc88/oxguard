## Python quality gates (pyguard)

After editing Python files, run `pyguard check` to validate all quality gates.
Fix any failures before committing. Key commands:

- `pyguard check`    — full gate: ruff → mypy → radon → types → coverage → security (bandit + pip-audit + detect-secrets)
- `pyguard fix`      — auto-format (ruff fix)
- `pyguard security` — secrets + CVE + SAST only
- `pyguard setup`    — install dev deps via uv

Default dirs: `functions,cdk` — override with `--dirs` if the project structure differs.
