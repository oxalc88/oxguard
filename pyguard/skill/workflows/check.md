# Check

1. If `command -v pyguard` fails, stop and report it.
2. Run with the requested root and scope:

   ```sh
   pyguard check --root /absolute/project/path --output agent
   ```

3. If findings are omitted, rerun with `--output json`.
4. Record status and actual exit code. Exit 0 alone is not success.
5. Name every partial or not-run gate in `gates`; `check` stops at the first blocker, and skipped gates never pass.
6. Split blocking findings, advisories and execution errors. Never invent findings from logs.

Done: report command, status, exit code, open findings, incomplete gates and next step.
