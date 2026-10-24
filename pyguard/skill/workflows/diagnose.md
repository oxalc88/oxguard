# Diagnose

Collect every independent finding before repairing a failure.

1. Confirm `pyguard --help` lists `diagnose`. If not, run the checks separately and disclose gaps.
2. Run with the same root, directories, exclusions as the failing run:

   ```sh
   pyguard diagnose --root /absolute/project/path --output json
   ```

3. It never installs tools. It uses the current uv environment without syncing; select the intended synchronized environment first. Missing tools show up as `error` analyses.
4. Read `findings` and `analyses`. Name every analysis that is `error`, `not_run` or `partial`.
5. Plan repairs from the full list, then go to [Fix](fix.md).

Done: all findings plus named incomplete analyses.
