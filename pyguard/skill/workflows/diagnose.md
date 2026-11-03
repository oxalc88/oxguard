# Diagnose

Collect every independent finding before repairing.

1. Confirm `pyguard --help` lists `diagnose`; otherwise run checks separately and disclose gaps.
2. Keep the failing run's root, `--dirs` and exclusions; never widen scope:

   ```sh
   pyguard diagnose --root /absolute/project/path --dirs src --output json
   ```

3. It never installs or syncs; select the intended synchronized uv environment first. Missing tools are `error` analyses.
4. Read `findings` and name every `error`, `not_run` or `partial` entry in `analyses`.
5. Plan repairs from the full list, then [Fix](fix.md).

Done: all findings plus named incomplete analyses.
