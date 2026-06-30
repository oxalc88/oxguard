## TypeScript quality gates (tsguard)

After editing TypeScript files, run `tsguard check` to validate all quality gates.
Fix any failures before committing. Key commands:

- `tsguard check`    — full gate: lint → fta → types → coverage → security (secretlint + audit-ci + opengrep)
- `tsguard fix`      — auto-format (ultracite fix)
- `tsguard security` — secrets + CVE + SAST only
- `tsguard setup`    — install deps + download opengrep binary

Project config: `oxguard.toml` at repo root overrides default dirs and excludes.

FTA gate skips `*.test.*` and `*.spec.*` files by default. Configure in `oxguard.toml`:

```toml
fta-exclude-tests = false    # re-enable FTA on test files
fta-exclude = ["*.pbt.ts"]   # add project-specific exclusion patterns
```
