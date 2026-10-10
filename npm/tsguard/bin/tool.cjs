'use strict';

// Package-relative executable resolution only. Gate orchestration stays in Go.
const fs = require('node:fs');
const path = require('node:path');
const { createRequire } = require('node:module');
const { spawnSync } = require('node:child_process');
const tools = {
  oxlint: 'oxlint', ultracite: 'ultracite', biome: '@biomejs/biome', tsc: 'typescript',
  vitest: 'vitest', fta: 'fta-cli', knip: 'knip', jscpd: 'jscpd',
  secretlint: 'secretlint', 'audit-ci': 'audit-ci',
};
const [tool, ...args] = process.argv.slice(2);
if (tool === '--resolve') {
  process.stdout.write(require.resolve(args[0]));
  process.exit(0);
}
if (!Object.hasOwn(tools, tool)) throw new Error(`Unknown packaged tool: ${tool}`);
const project = createRequire(path.join(process.cwd(), 'package.json'));
let resolver = require;
if (tool === 'tsc' || tool === 'vitest') {
  const manifest = JSON.parse(fs.readFileSync(path.join(process.cwd(), 'package.json')));
  const name = tools[tool];
  if (manifest.dependencies?.[name] || manifest.devDependencies?.[name]) resolver = project;
}
let packageFile;
try {
  packageFile = resolver.resolve(`${tools[tool]}/package.json`);
} catch (error) {
  if (error.code !== 'ERR_PACKAGE_PATH_NOT_EXPORTED') throw error;
  let directory = path.dirname(resolver.resolve(tool === 'ultracite' ? 'ultracite/biome/core' : tools[tool]));
  while (true) {
    const file = path.join(directory, 'package.json');
    if (fs.existsSync(file) && JSON.parse(fs.readFileSync(file)).name === tools[tool]) { packageFile = file; break; }
    const parent = path.dirname(directory);
    if (parent === directory) throw error;
    directory = parent;
  }
}
const manifest = JSON.parse(fs.readFileSync(packageFile));
let executable = path.resolve(path.dirname(packageFile), typeof manifest.bin === 'string' ? manifest.bin : manifest.bin[tool]);
// FTA's upstream JS wrapper invokes a shell and joins arguments unescaped.
// Invoke its already-shipped native binary directly, preserving spaces/signals.
if (tool === 'fta') {
  const targets = { 'linux-x64': 'x86_64-unknown-linux-musl', 'linux-arm64': 'aarch64-unknown-linux-musl', 'darwin-x64': 'x86_64-apple-darwin', 'darwin-arm64': 'aarch64-apple-darwin', 'win32-x64': 'x86_64-pc-windows-msvc' };
  executable = path.join(path.dirname(packageFile), 'binaries', `fta-${targets[`${process.platform}-${process.arch}`]}`, process.platform === 'win32' ? 'fta.exe' : 'fta');
}
const command = tool === 'fta' ? executable : process.execPath;
const commandArgs = tool === 'fta' ? args : [executable, ...args];
const result = spawnSync(command, commandArgs, { stdio: 'inherit' });
if (result.error) throw result.error;
if (result.signal) process.kill(process.pid, result.signal);
else process.exit(result.status ?? 1);
