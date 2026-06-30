# TypeScript Quality Gates

After editing TypeScript files, run `tsguard check` to validate quality gates.
If checks fail, fix the issues before committing.

Quality gate commands:
- `tsguard check`      — full gate (lint, types, coverage, security)
- `tsguard fix`        — auto-format
- `tsguard lint`       — lint only (ultracite check)
- `tsguard types`      — type check only
- `tsguard coverage`   — tests + coverage
- `tsguard security`   — secretlint + audit-ci + opengrep SAST
