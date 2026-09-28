---
summary: npm package generation, local verification and release configuration for the Go tsguard CLI.
read_when:
  - Installing tsguard with npm.
  - Maintaining npm packaging or publishing a release.
---

# npm distribution

`npm install -D @oxguard/tsguard` exposes `npx tsguard`. The public command still
runs the Go CLI; npm installation does not run setup, modify other project
configuration, download binaries in a lifecycle hook, or add analyzers.
Explicit Go `setup` behavior is unchanged.

For pnpm projects:

```sh
pnpm add -D @oxguard/tsguard
pnpm exec tsguard check
pnpm exec tsguard audit
```

Only the launcher and the matching platform's Go binary are installed. Opengrep,
ts-morph and the other analyzer tools are not dependencies of these packages.
No setup command or postinstall hook runs. Explicit `tsguard setup` still has
its existing tool-installation behavior; it is not needed to install the CLI.

## Package structure

Generated packages live under `dist/npm/` (ignored by git):

```text
tsguard/                         @oxguard/tsguard
  package.json                   bin + five exact-version optionalDependencies
  README.md
  bin/tsguard.cjs                 Node launcher, Node.js >=18
tsguard-linux-x64/               @oxguard/tsguard-linux-x64
tsguard-linux-arm64/             @oxguard/tsguard-linux-arm64
tsguard-darwin-x64/              @oxguard/tsguard-darwin-x64
tsguard-darwin-arm64/             @oxguard/tsguard-darwin-arm64
tsguard-win32-x64/               @oxguard/tsguard-win32-x64
  package.json                   os + cpu restrictions
  README.md
  bin/tsguard                     bin/tsguard.exe for Windows
```

`npm/prepare.cjs` is the single manifest/version source: `v1.2.3` becomes npm
`1.2.3` in all six packages. The Go binary continues reporting `v1.2.3` through
`-X main.version=v1.2.3`. Supported tags are stable or prerelease SemVer without
build metadata. The launcher checks the selected native package's version.
Missing optional dependencies produce a reinstall instruction; unsupported
OS/CPU combinations report a source-build option.

The launcher preserves arguments, working directory, environment, stdin,
stdout/stderr and native exit codes. It forwards SIGINT/SIGTERM and, on Unix,
SIGHUP. On Unix it re-emits a native terminating signal; on Windows, where
signals have limited semantics, it uses a conventional 128 + signal exit code.

Future tsguard-owned TypeScript analyzers can be added within the main package
and addressed relative to it, without changing the public command. This change
does not add any analyzers or change Go dependency resolution.

## Local verification without publishing

From the repository root, with Go and Node/npm available:

```sh
node --test npm/distribution.test.cjs
cd tsguard
go test ./...
```

The npm test builds the real host Go binary, generates packages, runs `npm pack`
for the launcher and native package, installs both tarballs into a temporary
consumer using `npm install --offline --ignore-scripts`, and runs
`npx --no-install tsguard --version` and `--help`. It checks the real CLI's exit
code 3, consumer package stability, platform metadata, missing and
mismatched native packages, unsupported platforms, startup errors, arguments,
stdin/stdout/stderr, and exit codes 0, 1, 3, 4, 5 and 37 using a small native fixture. Unix runners
also test signal forwarding and termination. Temporary files are removed.

To run the same verification with pnpm installed:

```sh
TSGUARD_PACKAGE_MANAGER=pnpm node --test npm/distribution.test.cjs
```

In PowerShell, set `$env:TSGUARD_PACKAGE_MANAGER = 'pnpm'` before running the test.
The pnpm test adds only the main tarball as a direct devDependency. A local
override in the temporary test consumer supplies its unpublished native optional
dependency. This verifies resolution under pnpm's isolated dependency layout
rather than relying on a native dependency installed at the project root.
Consumer install scripts are disabled. No local override is needed for published
packages. CI exercises npm and pnpm 10.34.5/12.6.0 on Linux, macOS and Windows.

To retain tarballs for manual testing, for example on macOS arm64:

```sh
mkdir -p dist/binaries/darwin-arm64
(cd tsguard && CGO_ENABLED=0 go build -trimpath -ldflags '-s -w -X main.version=v0.0.0-local' -o ../dist/binaries/darwin-arm64/tsguard .)
node npm/prepare.cjs v0.0.0-local dist/binaries dist/npm darwin-arm64
npm pack ./dist/npm/tsguard-darwin-arm64 --pack-destination dist/npm
npm pack ./dist/npm/tsguard --pack-destination dist/npm
```

In a temporary consumer directory, supply the absolute paths to both tarballs:

```sh
npm init -y
npm install -D --offline --ignore-scripts --no-audit --no-fund /absolute/path/oxguard/dist/npm/oxguard-tsguard-darwin-arm64-0.0.0-local.tgz /absolute/path/oxguard/dist/npm/oxguard-tsguard-0.0.0-local.tgz
npx --no-install tsguard --version
npx --no-install tsguard --help
```

Substitute your host platform and executable name. The extra native tarball is
only needed for unpublished local tests. Normal users install just the main
package. No registry or npm login is needed for these checks.

## Release flow and registry setup

The release workflow checks packed installs on Linux, macOS and Windows first.
It checks out the requested tag (including manual dispatch), cross-compiles with
CGO disabled, and packages the same binary into existing release archives and
npm platform tarballs. The existing GitHub archives, checksums and installers
are published as before. npm publishing follows, with all five native packages
published before the launcher. Stable tags use npm `latest`; prereleases use
`next`. The source tag must include this distribution implementation.

Before the first release:

1. Create/control the npm `@oxguard` scope and grant the publishing account
   access to all six package names. All packages are published publicly.
2. Create a granular npm access token with package/scope read-and-write publish
   permissions. Include scope access for creating the new packages. For
   unattended publishing with 2FA enabled, enable the token's Bypass 2FA option
   and ensure package/organization policy permits token publishing.
3. Add the token as the repository Actions secret `MAEZSAEN_TOKEN`. The workflow
   uses setup-node's registry configuration and `NODE_AUTH_TOKEN`; no env files
   need editing. Rotate the token before its configured expiration.
4. Push a new version tag, or run Release manually with that tag.

See npm's [token setup documentation](https://docs.npmjs.com/creating-and-viewing-access-tokens/).
Organization-management token permissions alone do not grant package publish
rights. GitHub Releases can succeed independently of a later npm publish failure.
Published npm versions are immutable: if publication partially succeeds, inspect
which packages exist and publish the remaining tarballs in native-first order;
do not blindly rerun publication of already published versions.

Nothing has been published by this implementation work. The repository currently
has no LICENSE file; packaging does not invent a license declaration.
