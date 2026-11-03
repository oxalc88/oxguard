# Check

1. Stop if `command -v pyguard` fails.
2. Use the requested root/scope:

   ```sh
   pyguard check --root /absolute/project/path --output agent
   ```

3. Omitted findings: rerun identically with `--output json` before editing.
4. Record status, assessment and exit code; 0 alone is insufficient.
5. Name partial/not-run gates; `check` stops at the first blocker. Skipped gates never pass.
6. Separate blockers, advisories and execution errors (include `category`, e.g. `tool_missing`). Never invent findings from logs.

Report command, findings, incomplete gates and next step.
