'use strict';

const assert = require('node:assert/strict');
const fs = require('node:fs');
const os = require('node:os');
const path = require('node:path');
const { spawnSync, spawn } = require('node:child_process');
const { test } = require('node:test');
const { once } = require('node:events');
const { prepare, platforms } = require('./prepare.cjs');

const root = path.resolve(__dirname, '..');
const host = `${process.platform}-${process.arch}`;
const binaryName = process.platform === 'win32' ? 'tsguard.exe' : 'tsguard';
const version = '0.0.0-npm-test';

function run(command, args, cwd, expected = 0) {
  // Windows npm/npx are .cmd scripts; quote paths when going through cmd.exe.
  const shell = process.platform === 'win32' && ['npm', 'npx'].includes(command);
  const result = spawnSync(command, shell ? args.map(a => `"${a}"`) : args, {
    cwd, encoding: 'utf8', shell, timeout: 120000,
  });
  assert.ifError(result.error);
  assert.equal(result.status, expected, `${command}: ${result.stdout}\n${result.stderr}`);
  return result;
}

test('packed distribution runs the Go CLI and forwards native process behavior', { timeout: 240000 }, async t => {
  const temporary = fs.mkdtempSync(path.join(os.tmpdir(), 'tsguard-npm-'));
  t.after(() => fs.rmSync(temporary, { recursive: true, force: true }));
  const binaries = path.join(temporary, 'binaries');
  const packages = path.join(temporary, 'packages');
  const consumer = path.join(temporary, 'consumer with spaces');
  fs.mkdirSync(path.join(binaries, host), { recursive: true });
  fs.mkdirSync(consumer);
  const nativeBinary = path.join(binaries, host, binaryName);
  run('go', ['build', '-trimpath', '-ldflags', `-X main.version=v${version}`, '-o', nativeBinary, '.'], path.join(root, 'tsguard'));
  prepare(`v${version}`, binaries, packages, [host]);
  assert.throws(() => prepare('v01.2.3', binaries, packages), /Invalid release version/);
  assert.throws(() => prepare('v1.2.3-01', binaries, packages), /Invalid release version/);
  assert.throws(() => prepare('v1.2.3', binaries, packages, ['freebsd-x64']), /Unsupported platform/);
  assert.throws(() => prepare('v1.2.3', binaries, packages), /ENOENT/);

  // Verify every generated platform manifest, without pretending these copies are cross-builds.
  for (const platform of platforms) {
    fs.mkdirSync(path.join(binaries, platform), { recursive: true });
    fs.copyFileSync(nativeBinary, path.join(binaries, platform, platform.startsWith('win32') ? 'tsguard.exe' : 'tsguard'));
  }
  prepare(`v${version}`, binaries, packages);
  const manifest = JSON.parse(fs.readFileSync(path.join(packages, 'tsguard/package.json')));
  assert.equal(Object.keys(manifest.optionalDependencies).length, 5);
  assert.equal(manifest.scripts, undefined);
  assert.equal(manifest.dependencies, undefined);
  for (const platform of platforms) {
    const native = JSON.parse(fs.readFileSync(path.join(packages, `tsguard-${platform}/package.json`)));
    assert.equal(native.version, manifest.version);
    assert.equal(manifest.optionalDependencies[native.name], native.version);
    assert.deepEqual(native.os, [platform.split('-')[0]]);
    assert.deepEqual(native.cpu, [platform.split('-')[1]]);
  }
  const tarballs = ['tsguard', `tsguard-${host}`].map(name => {
    const packed = JSON.parse(run('npm', ['pack', '--json', '--pack-destination', temporary], path.join(packages, name)).stdout)[0];
    assert.ok(packed.files.some(file => file.path.startsWith('bin/')));
    assert.ok(!packed.files.some(file => file.path.endsWith('.go') || file.path.includes('test')));
    return path.join(temporary, packed.filename);
  });
  fs.writeFileSync(path.join(consumer, 'package.json'), JSON.stringify({ name: 'consumer', private: true, version: '1.0.0' }));
  // Both tarballs are supplied locally: unpublished optional versions never need the registry.
  run('npm', ['install', '--offline', '--ignore-scripts', '--no-audit', '--no-fund', '-D', ...tarballs], consumer);
  const other = platforms.find(platform => platform !== host);
  const otherPack = JSON.parse(run('npm', ['pack', '--json', '--pack-destination', temporary], path.join(packages, `tsguard-${other}`)).stdout)[0];
  assert.match(run('npm', ['install', '--offline', '--ignore-scripts', '--no-audit', '--no-fund', '--no-save', path.join(temporary, otherPack.filename)], consumer, 1).stderr, /EBADPLATFORM/);
  const packageBefore = fs.readFileSync(path.join(consumer, 'package.json'), 'utf8');
  assert.equal(run('npx', ['--no-install', 'tsguard', '--version'], consumer).stdout.trim(), `v${version}`);
  assert.match(run('npx', ['--no-install', 'tsguard', '--help'], consumer).stdout, /tsguard check/);
  assert.match(run('npx', ['--no-install', 'tsguard', 'not-a-command'], consumer, 3).stderr, /unknown command/);
  assert.equal(fs.readFileSync(path.join(consumer, 'package.json'), 'utf8'), packageBefore);
  const installed = path.join(consumer, 'node_modules/@oxguard');
  assert.deepEqual(fs.readdirSync(installed).sort(), ['tsguard', `tsguard-${host}`].sort());
  const launcher = path.join(installed, 'tsguard/bin/tsguard.cjs');
  const nativeDirectory = path.join(installed, `tsguard-${host}`);
  const installedBinary = path.join(nativeDirectory, 'bin', binaryName);
  run('go', ['build', '-o', installedBinary, path.join(__dirname, 'testdata/cli.go')], root);
  for (const code of [0, 1, 3, 4, 5]) {
    run(process.execPath, [launcher, 'exit', String(code), 'ok'], consumer, code);
  }
  assert.equal(run(process.execPath, [launcher, 'exit', '37', 'argument with spaces'], consumer, 37).stdout.trim(), 'argument with spaces');
  assert.match(run(process.execPath, [launcher, 'exit', '0', 'ok'], consumer).stderr, /fixture stderr/);
  const input = spawnSync(process.execPath, [launcher, 'stdin'], { cwd: consumer, input: 'hello\n', encoding: 'utf8' });
  assert.equal(input.status, 0);
  assert.equal(input.stdout.trim(), 'hello');
  if (process.platform !== 'win32') {
    for (const signal of ['SIGTERM', 'SIGINT', 'SIGHUP']) {
      const child = spawn(process.execPath, [launcher, 'wait'], { cwd: consumer });
      t.after(() => { if (child.exitCode === null) child.kill('SIGKILL'); });
      const exited = once(child, 'exit');
      await new Promise((resolve, reject) => {
        let output = '';
        child.stdout.on('data', data => { output += data; if (output.includes('ready')) resolve(); });
        child.on('error', reject);
        child.on('exit', () => reject(new Error('Fixture exited before readiness')));
      });
      child.kill(signal);
      assert.deepEqual(await exited, [42, null]);
    }
    const signaled = spawnSync(process.execPath, [launcher, 'signal'], { cwd: consumer, timeout: 10000 });
    assert.equal(signaled.signal, 'SIGTERM');
  }
  const nativeManifest = path.join(nativeDirectory, 'package.json');
  const native = JSON.parse(fs.readFileSync(nativeManifest));
  fs.writeFileSync(nativeManifest, JSON.stringify({ ...native, version: '0.0.1' }));
  assert.match(run(process.execPath, [launcher, '--version'], consumer, 1).stderr, /expected 0.0.0-npm-test/);
  fs.writeFileSync(nativeManifest, JSON.stringify(native));
  if (process.platform !== 'win32') {
    fs.chmodSync(installedBinary, 0o644);
    assert.match(run(process.execPath, [launcher], consumer, 1).stderr, /could not start/);
  }
  fs.renameSync(nativeDirectory, `${nativeDirectory}-missing`);
  assert.match(run(process.execPath, [launcher], consumer, 1).stderr, /missing native package.*include=optional/);
  const preload = path.join(temporary, 'unsupported.cjs');
  fs.writeFileSync(preload, "Object.defineProperty(process, 'platform', { value: 'freebsd' });\n");
  assert.match(run(process.execPath, ['--require', preload, launcher], consumer, 1).stderr, /unsupported platform freebsd/);
});
