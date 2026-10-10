---
summary: Complete npm-owned toolchain, version pins, local verification and releases for the Go tsguard CLI.
read_when:
  - Installing tsguard with npm.
  - Maintaining npm packaging or publishing a release.
---

# npm distribution

`npm install -D @oxguard/tsguard` exposes `npx tsguard`. The public command still
runs the Go CLI. npm installation adds the package-owned analyzer dependencies
and the matching native Go/Opengrep package. It does not run setup, modify other
project configuration, or download binaries in a lifecycle hook. Analyzer tools
are dependencies of @oxguard/tsguard, not individual entries added to the consuming
manifest. In npm mode `setup` verifies tools and offers hooks; standalone Go
`setup` retains its existing behavior.

For pnpm projects:

```sh
pnpm add -D @oxguard/tsguard
pnpm exec tsguard check
pnpm exec tsguard audit
```

Only the matching native package is installed. All existing analysis capabilities
are available immediately, including Opengrep. No postinstall hook runs. This
change adds no Level 2 analysis or ts-morph. An installed toolchain does not mean
an arbitrary project's checks pass: findings, missing tests and invalid project
configs still produce failures.

## Versions and resolution

`npm/toolchain.json` pins each direct implementation tool to an exact version.
Upgrade tools deliberately by editing those pins, running the real gate tests,
and publishing a new tsguard version. Consumer npm/pnpm lockfiles fix the resolved
transitive tree; exact direct pins do not freeze upstream transitive ranges.
Publisher npm shrinkwrap files are not a portable lock across npm and pnpm.

The launcher supplies its runtime location and Node executable to Go through
child-process environment variables. Go selects packaged commands; a small Node
adapter resolves their bins relative to the package, including pnpm's isolated
layout. FTA's shipped native executable is called directly to preserve argument
boundaries. Declared project TypeScript and Vitest tools take precedence; other
configured project test runners keep their existing project-owned behavior.

Existing project configs are preserved. Without configs, lint uses the pinned
Ultracite/Biome core preset; types use strict cached defaults; secrets use the
recommended Secretlint preset; coverage uses bundled Vitest/V8 and still requires
tests. Generated configs live under node_modules/.cache/oxguard/defaults, not at
the project root. npm mode setup does not add implementation devDependencies.

Opengrep v1.23.0 is copied into the native packages at build time. Asset and license
SHA-256 digests are pinned in npm/native-tools.json and npm/fetch-native.cjs. A
checksum mismatch fails packaging. Linux packages include glibc and musl engines.
LGPL license/source notices ship alongside them. Missing packaged SAST fails
instead of reporting a successful skip. The standalone missing-engine behavior
is preserved. Default p/javascript and p/typescript rules are still retrieved
from their registry when scanning; these remote rule sets are not version pins.
Projects can supply their existing cached rule directory. CVE auditing likewise
requires registry access. The CLI does not claim that security scans are offline.

## Package structure

Generated packages live under `dist/npm/` (ignored by git):

```text
tsguard/                         @oxguard/tsguard
  package.json                   owned tool dependencies + five native optionalDependencies
  README.md
  bin/tsguard.cjs                 Node launcher
  bin/tool.cjs                    package-relative tool adapter
  config/vitest.config.mjs        default coverage config
tsguard-linux-x64/               @oxguard/tsguard-linux-x64
tsguard-linux-arm64/             @oxguard/tsguard-linux-arm64
tsguard-darwin-x64/              @oxguard/tsguard-darwin-x64
tsguard-darwin-arm64/             @oxguard/tsguard-darwin-arm64
tsguard-win32-x64/               @oxguard/tsguard-win32-x64
  package.json                   os + cpu restrictions
  README.md
  bin/tsguard                     bin/tsguard.exe for Windows
  bin/opengrep                    bin/opengrep.exe for Windows
  licenses/                      upstream Opengrep license and source notices
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

Future TypeScript analyzers can use the same package-owned resolution without
changing the public command. Business logic and gate orchestration remain in Go.

## Local verification without publishing

From the repository root, with Go and Node/npm available:

```sh
node --test npm/distribution.test.cjs
cd tsguard
go test ./...
```

The npm test builds the real host Go binary, generates packages, runs `npm pack`
for the launcher and native package, installs both tarballs into a temporary
consumer using `npm install --ignore-scripts`, and runs
`npx --no-install tsguard --version` and `--help`. It checks the real CLI's exit
code 3, consumer package stability, platform metadata, missing and
mismatched native packages, unsupported platforms, startup errors, arguments,
stdin/stdout/stderr, and exit codes 0, 1, 3, 4, 5 and 37 using a small native fixture. Unix runners
also test signal forwarding and termination. The test now exercises doctor,
formatting, lint, types, FTA, coverage, secrets, audit and the complete check
pipeline with real owned tools, plus type and SAST findings. Installation needs
registry access for the toolchain. A local SAST test rule removes the live rule
registry from test expectations. Set TSGUARD_KEEP_TEST_DIR=1 to retain fixtures
for debugging; otherwise temporary files are removed.
An adapter test separately verifies that a declared project compiler wins over
the bundled compiler, while an undeclared compiler uses the owned version.

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
node npm/fetch-native.cjs dist/binaries darwin-arm64
node npm/prepare.cjs v0.0.0-local dist/binaries dist/npm darwin-arm64
npm pack ./dist/npm/tsguard-darwin-arm64 --pack-destination dist/npm
npm pack ./dist/npm/tsguard --pack-destination dist/npm
```

In a temporary consumer directory, supply the absolute paths to both tarballs:

```sh
npm init -y
npm install -D --ignore-scripts --no-audit --no-fund /absolute/path/oxguard/dist/npm/oxguard-tsguard-darwin-arm64-0.0.0-local.tgz /absolute/path/oxguard/dist/npm/oxguard-tsguard-0.0.0-local.tgz
npx --no-install tsguard --version
npx --no-install tsguard --help
```

Substitute your host platform and executable name. The extra native tarball is
only needed for unpublished local tests. Normal users install just the main
package. npm login is not needed; registry access is needed for owned dependencies.

## Release flow and registry setup

The release workflow checks packed installs on Linux, macOS and Windows first.
It checks out the requested tag (including manual dispatch), cross-compiles with
CGO disabled, and packages the same binary into existing release archives and
npm platform tarballs. npm packaging additionally fetches the pinned, verified
Opengrep assets and licenses. The existing GitHub archives, checksums and installers
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

## Native semantic lint

The package pins Oxlint and oxlint-tsgolint for the separate typed-lint gate. Platform binaries resolve through the backend package's own dependency graph, including pnpm isolation; no project or global executable is substituted. Biome owns ordinary syntax/format policy. TypeScript 7 backend limits and native assessment artifacts are documented in [maintainability](../maintainability.md). Missing optional platform engines require reinstalling with optional dependencies.
