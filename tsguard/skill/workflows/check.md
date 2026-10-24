# Check

1. Use the local CLI (`pnpm exec tsguard` with pnpm). Never let plain `npx` fetch a version.
2. Run with the requested root and scope:

   ```sh
   npx --no-install tsguard check --root /absolute/project/path --output agent
   ```

3. If findings are omitted, rerun the same command and scope with `--output json` before any edit.
4. Record status and actual exit code. Exit 0 alone is not success.
5. Name every partial or not-run gate in `gates`; `check` stops at the first blocker, and skipped gates never pass.
6. Split blocking findings, advisories and execution errors. Never invent findings from logs. For execution errors, name the finding `category` (for example `tool_missing`).

Done: report command, status, exit code, open findings, incomplete gates and next step.
