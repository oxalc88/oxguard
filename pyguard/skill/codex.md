# Python Quality Gates

After editing Python files, run `__BINARY__ check` to validate quality gates.
If checks fail, fix the issues before committing.

Quality gate commands:
- `__BINARY__ check`    — full gate (lint, types, complexity, tests, security)
- `__BINARY__ fix`      — auto-format
- `__BINARY__ ruff`     — lint only
- `__BINARY__ mypy`     — type check only
- `__BINARY__ coverage` — tests + coverage
- `__BINARY__ security` — bandit + pip-audit + detect-secrets
