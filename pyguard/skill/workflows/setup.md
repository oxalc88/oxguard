# Setup

1. Check the toolchain without changing anything:

   ```sh
   pyguard doctor
   ```

2. Only when authorized, run `pyguard setup`. `setup` installs uv, runs `uv sync` and configures AI tool hooks.
3. Rerun `doctor`.

Done: `doctor` reports a ready toolchain, or the missing pieces are listed.
