# Diagnose

Collect every independent finding before repairing a failure.

1. Confirm `npx --no-install tsguard --help` lists `diagnose`. If not, run the checks separately and disclose gaps.
2. Keep the same root, `--dirs` and exclusions (and `--baseline`) as the failing run; never widen the scope:

   ```sh
   npx --no-install tsguard diagnose --root /absolute/project/path --dirs src --output json
   ```

3. It never installs tools. Without a usable baseline, `change` stays `not_run`.
4. Read `findings` and `analyses`. Name every analysis that is `error`, `not_run` or `partial`.
5. Plan repairs from the full list, then go to [Fix](fix.md).

Done: all findings plus named incomplete analyses.
