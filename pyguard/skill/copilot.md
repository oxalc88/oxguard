## Python quality gates (pyguard)

After editing Python files, run `pyguard check` to validate all quality gates.
Fix any failures before committing. Key commands:

- `pyguard check`    — full gate: ruff → mypy → radon → types → coverage → security (bandit + pip-audit + detect-secrets)
- `pyguard fix`      — auto-format (ruff fix)
- `pyguard security` — secrets + CVE + SAST only
- `pyguard setup`    — install dev deps via uv

Default dirs: `.` (project root) — use `--dirs` to restrict to specific subdirectories.

Complexity gates (radon cc/mi/hal, Halstead, type annotations) skip `test_*.py`, `*_test.py`, `conftest.py`, and `tests/` by default. Configure in `pyproject.toml`:

```toml
[tool.pyguard]
exclude-tests = false          # re-enable complexity gating on test files
exclude = ["*/fixtures/*"]     # add project-specific exclusion patterns
```
