#!/usr/bin/env node
'use strict';

const { spawn } = require('node:child_process');
const { constants } = require('node:os');
const path = require('node:path');
const { version, optionalDependencies } = require('../package.json');

const name = `@oxguard/tsguard-${process.platform}-${process.arch}`;
function fail(message) {
  console.error(`tsguard: ${message}`);
  process.exit(1);
}
if (!Object.hasOwn(optionalDependencies, name)) {
  fail(`unsupported platform ${process.platform}/${process.arch}. Use a supported platform or build the Go CLI from source.`);
}
let binary;
try {
  const native = require(`${name}/package.json`);
  if (native.version !== version) {
    fail(`native package ${name} has version ${native.version}; expected ${version}. Reinstall @oxguard/tsguard.`);
  }
  binary = require.resolve(`${name}/bin/tsguard${process.platform === 'win32' ? '.exe' : ''}`);
} catch {
  fail(`missing native package ${name}@${version}. Reinstall with npm install -D @oxguard/tsguard --include=optional (do not use --omit=optional).`);
}

const nativeRoot = path.dirname(path.dirname(binary));
const musl = process.platform === 'linux' && !process.report.getReport().header.glibcVersionRuntime;
const child = spawn(binary, process.argv.slice(2), {
  stdio: 'inherit',
  env: { ...process.env,
    TSGUARD_RUNTIME: path.resolve(__dirname, '..'),
    TSGUARD_NODE: process.execPath,
    TSGUARD_OPENGREP: path.join(nativeRoot, 'bin', `opengrep${musl ? '-musl' : ''}${process.platform === 'win32' ? '.exe' : ''}`),
  },
});
const handlers = new Map();
for (const signal of ['SIGINT', 'SIGTERM', ...(process.platform === 'win32' ? [] : ['SIGHUP'])]) {
  const handler = () => child.kill(signal);
  handlers.set(signal, handler);
  process.on(signal, handler);
}
child.on('error', error => fail(`could not start ${name}: ${error.message}`));
child.on('exit', (code, signal) => {
  for (const [name, handler] of handlers) process.removeListener(name, handler);
  if (signal) {
    if (process.platform === 'win32') process.exit(128 + (constants.signals[signal] || 1));
    process.kill(process.pid, signal);
  } else {
    process.exit(code ?? 1);
  }
});
