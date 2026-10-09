# @oxguard/tsguard

Install the complete Go-powered toolchain as a project development dependency
(Node.js 22.13+ on the 22.x line, 24.x, or 26+):

```sh
npm install -D @oxguard/tsguard
npx tsguard check
npx tsguard audit
```

With pnpm:

```sh
pnpm add -D @oxguard/tsguard
pnpm exec tsguard check
pnpm exec tsguard audit
```

The small Node launcher runs the matching Go binary with the caller's arguments,
working directory, environment and terminal streams. The package owns pinned
lint, type, test/coverage, FTA, secrets, vulnerability-audit, dead-code and duplicate
analysis tools. The five native packages ship Go and Opengrep binaries and are
optional dependencies pinned to the exact launcher version, restricted by npm
`os` and `cpu`: linux-x64, linux-arm64, darwin-x64, darwin-arm64 and win32-x64.
Linux Go binaries are built with CGO disabled. Linux packages contain glibc and
musl Opengrep builds; the launcher selects the appropriate engine.

There are no installation scripts or install-time binary downloads. The project
manifest lists the tsguard package; its implementation tools are transitive
dependencies. Commands work immediately, without running `setup`. Missing lint,
type and secrets configs use cache-local defaults; existing project configs and
declared TypeScript/test tools take precedence. Coverage still requires actual
project tests; findings and vulnerabilities still fail the applicable gates.

`setup` in npm mode verifies the owned tools and offers project hooks. Standalone
Go installations retain their existing setup behavior. CVE auditing and Opengrep's
default remote rule sets require network access when scans run. Opengrep license
and source notices are included in the native package's `licenses/` directory.

Source and distribution guide: https://github.com/oxalc88/oxguard

The unreleased maintainability candidate adds zero-config typed lint and advisory
smell/module analysis to `check`. Focused commands are `typed-lint`,
`maintainability`, `smells`, `structure`, and `change --baseline <Git ref>`.
Project lint configurations remain authoritative. Missing optional baselines are
not evaluated and do not fail the command. Scope and limits:
https://github.com/oxalc88/oxguard/blob/main/docs/maintainability.md
