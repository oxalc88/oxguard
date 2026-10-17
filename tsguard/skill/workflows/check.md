# Run checks and read results

Use the project-local npm CLI; use `pnpm exec tsguard` for pnpm. Do not let plain `npx` fetch a version. Confirm `--help` supports `--root` and `--output`.

```sh
npx --no-install tsguard check --root /absolute/project/path --output agent
```

Use the requested command and flags. For supplied results, interpret without rerunning unless needed and authorized. Reuse earlier checks only when source, tests, configuration, dependencies, toolchain, environment and scope match. Changed lockfiles invalidate applicable checks.

Record semantic status and actual exit code. Exit zero alone is not success. Unknown, contradictory or malformed statuses are invalid evidence.

If findings are omitted, retrieve the same invocation with `--output json` before edits or complete-inventory claims. Read schema-1 `findings`, `assessment` and `gates`. Name partial and not-run gates. Complete assessment means requested analysis ran, not that findings passed. Never invent findings from logs.

Separate blocking quality findings, advisories and execution errors. Resolve diagnostic IDs through `diagnostics`; inspect only relevant streams. `check` stops after a blocking failure. Never report skipped gates as passed.

Report command/scope, status, exit code, unresolved findings, incomplete gates and next action.
