# @oxguard/tsguard

Install the Go CLI as a project development dependency (Node.js 18+):

```sh
npm install -D @oxguard/tsguard
npx tsguard check
npx tsguard audit
```

The small Node launcher runs the matching Go binary with the caller's arguments,
working directory, environment and terminal streams. The five native packages are
optional dependencies pinned to the exact launcher version, restricted by npm
`os` and `cpu`: linux-x64, linux-arm64, darwin-x64, darwin-arm64 and win32-x64.
Linux binaries are built with CGO disabled to avoid a glibc dependency.

There are no installation scripts, binary downloads, or analyzer dependencies.
Installation only adds the requested npm dependency; it does not run `tsguard setup`.
Existing Go commands (including explicit `setup`) retain their existing behavior.
Future owned analyzers can live inside this package without changing the command.

Source and distribution guide: https://github.com/oxalc88/oxguard
