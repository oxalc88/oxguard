# Setup

1. Check the toolchain without changing anything:

   ```sh
   npx --no-install tsguard doctor
   ```

2. Only when authorized, run `npx --no-install tsguard setup`. `setup` runs the package manager install and configures AI tool hooks.
3. Rerun `doctor`.

Done: `doctor` reports a ready toolchain, or the missing pieces are listed.
