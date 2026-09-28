'use strict';

const assert = require('node:assert/strict');
const fs = require('node:fs');
const os = require('node:os');
const path = require('node:path');
const { spawnSync, spawn } = require('node:child_process');
const { test } = require('node:test');
const { once } = require('node:events');
const { createRequire } = require('node:module');
const { prepare, platforms } = require('./prepare.cjs');
const { fetchNative } = require('./fetch-native.cjs');

const root = path.resolve(__dirname, '..');
const host = `${process.platform}-${process.arch}`;
const binaryName = process.platform === 'win32' ? 'tsguard.exe' : 'tsguard';
const version = '0.0.0-npm-test';
const manager = process.env.TSGUARD_PACKAGE_MANAGER || 'npm';
assert.ok(['npm', 'pnpm'].includes(manager), `Unsupported test package manager: ${manager}`);

function run(command, args, cwd, expected = 0, timeout = 120000) {
  // Windows package managers are .cmd scripts; quote paths when going through cmd.exe.
  const shell = process.platform === 'win32' && ['npm', 'npx', 'pnpm'].includes(command);
  const result = spawnSync(command, shell ? args.map(a => `"${a}"`) : args, {
    cwd, encoding: 'utf8', shell, timeout,
  });
  assert.ifError(result.error);
  assert.equal(result.status, expected, `${command}: ${result.stdout}\n${result.stderr}`);
  return result;
}

test('packaged adapter preserves a declared project compiler', t => {
  const temporary = fs.mkdtempSync(path.join(os.tmpdir(), 'tsguard-compiler-'));
  t.after(() => fs.rmSync(temporary, { recursive: true, force: true }));
  const owned = path.join(temporary, 'owned');
  const project = path.join(temporary, 'project');
  fs.mkdirSync(path.join(owned, 'bin'), { recursive: true });
  fs.mkdirSync(project);
  const adapter = path.join(owned, 'bin/tool.cjs');
  fs.copyFileSync(path.join(__dirname, 'tsguard/bin/tool.cjs'), adapter);
  function compiler(directory, label) {
    const packageDirectory = path.join(directory, 'node_modules/typescript');
    fs.mkdirSync(path.join(packageDirectory, 'bin'), { recursive: true });
    fs.writeFileSync(path.join(packageDirectory, 'package.json'), JSON.stringify({ name: 'typescript', bin: { tsc: 'bin/tsc.cjs' } }));
    fs.writeFileSync(path.join(packageDirectory, 'bin/tsc.cjs'), `console.log(${JSON.stringify(label)});\n`);
  }
  compiler(owned, 'owned compiler');
  compiler(project, 'project compiler');
  const manifest = path.join(project, 'package.json');
  fs.writeFileSync(manifest, '{}');
  assert.equal(run(process.execPath, [adapter, 'tsc', '--version'], project).stdout.trim(), 'owned compiler');
  fs.writeFileSync(manifest, JSON.stringify({ devDependencies: { typescript: '5.8.3' } }));
  assert.equal(run(process.execPath, [adapter, 'tsc', '--version'], project).stdout.trim(), 'project compiler');
});

test(`${manager} packed distribution runs the Go CLI and forwards native process behavior`, { timeout: 360000 }, async t => {
  const temporary = fs.mkdtempSync(path.join(os.tmpdir(), 'tsguard-npm-'));
  t.after(() => {
    if (process.env.TSGUARD_KEEP_TEST_DIR) t.diagnostic(`Test artifacts: ${temporary}`);
    else fs.rmSync(temporary, { recursive: true, force: true });
  });
  const binaries = path.join(temporary, 'binaries');
  const packages = path.join(temporary, 'packages');
  const consumer = path.join(temporary, 'consumer with spaces');
  fs.mkdirSync(path.join(binaries, host), { recursive: true });
  fs.mkdirSync(consumer);
  const nativeBinary = path.join(binaries, host, binaryName);
  run('go', ['build', '-trimpath', '-ldflags', `-X main.version=v${version}`, '-o', nativeBinary, '.'], path.join(root, 'tsguard'));
  // Reuse verified build inputs across test runs; consumer installation stays script-free.
  const engines = path.join(root, 'dist/binaries');
  await fetchNative(engines, host);
  for (const file of fs.readdirSync(path.join(engines, host))) {
    if (file.startsWith('opengrep') || file === 'licenses') fs.cpSync(path.join(engines, host, file), path.join(binaries, host, file), { recursive: true });
  }
  prepare(`v${version}`, binaries, packages, [host]);
  assert.throws(() => prepare('v01.2.3', binaries, packages), /Invalid release version/);
  assert.throws(() => prepare('v1.2.3-01', binaries, packages), /Invalid release version/);
  assert.throws(() => prepare('v1.2.3', binaries, packages, ['freebsd-x64']), /Unsupported platform/);
  assert.throws(() => prepare('v1.2.3', binaries, packages), /ENOENT/);

  // Verify every generated platform manifest, without pretending these copies are cross-builds.
  for (const platform of platforms) {
    fs.mkdirSync(path.join(binaries, platform), { recursive: true });
    fs.copyFileSync(nativeBinary, path.join(binaries, platform, platform.startsWith('win32') ? 'tsguard.exe' : 'tsguard'));
    if (platform !== host) {
      fs.copyFileSync(path.join(binaries, host, process.platform === 'win32' ? 'opengrep.exe' : 'opengrep'), path.join(binaries, platform, platform.startsWith('win32') ? 'opengrep.exe' : 'opengrep'));
      if (platform.startsWith('linux')) fs.copyFileSync(nativeBinary, path.join(binaries, platform, 'opengrep-musl'));
      fs.cpSync(path.join(binaries, host, 'licenses'), path.join(binaries, platform, 'licenses'), { recursive: true });
    }
  }
  prepare(`v${version}`, binaries, packages);
  const manifest = JSON.parse(fs.readFileSync(path.join(packages, 'tsguard/package.json')));
  assert.equal(Object.keys(manifest.optionalDependencies).length, 5);
  assert.equal(manifest.scripts, undefined);
  for (const version of Object.values(manifest.dependencies)) assert.match(version, /^\d+\.\d+\.\d+$/);
  for (const platform of platforms) {
    const native = JSON.parse(fs.readFileSync(path.join(packages, `tsguard-${platform}/package.json`)));
    assert.equal(native.version, manifest.version);
    assert.equal(manifest.optionalDependencies[native.name], native.version);
    assert.deepEqual(native.os, [platform.split('-')[0]]);
    assert.deepEqual(native.cpu, [platform.split('-')[1]]);
    assert.equal(native.dependencies, undefined);
    assert.equal(native.scripts, undefined);
  }
  const tarballs = ['tsguard', `tsguard-${host}`].map(name => {
    const packed = JSON.parse(run('npm', ['pack', '--json', '--pack-destination', temporary], path.join(packages, name)).stdout)[0];
    assert.ok(packed.files.some(file => file.path.startsWith('bin/')));
    assert.ok(!packed.files.some(file => file.path.endsWith('.go') || file.path.endsWith('.test.cjs') || file.path.startsWith('testdata/')));
    return path.join(temporary, packed.filename);
  });
  fs.writeFileSync(path.join(consumer, 'package.json'), JSON.stringify({ name: 'consumer', private: true, version: '1.0.0' }));
  if (manager === 'pnpm') {
    // Keep the native package transitive to verify pnpm's isolated dependency layout.
    // Only this temporary consumer overrides the unpublished native version.
    fs.writeFileSync(path.join(consumer, 'pnpm-workspace.yaml'),
      `overrides:\n  ${JSON.stringify(`@oxguard/tsguard-${host}`)}: ${JSON.stringify(`file:${tarballs[1].replaceAll('\\', '/')}`)}\n`);
    run('pnpm', ['add', '-D', '--ignore-scripts', '--store-dir', path.join(temporary, 'store'), tarballs[0]], consumer);
    const consumerManifest = JSON.parse(fs.readFileSync(path.join(consumer, 'package.json')));
    assert.deepEqual(Object.keys(consumerManifest.devDependencies), ['@oxguard/tsguard']);
  } else {
  // Both tarballs are supplied locally; owned tools still resolve from the registry.
    run('npm', ['install', '--ignore-scripts', '--no-audit', '--no-fund', '-D', ...tarballs], consumer, 0, 240000);
    const other = platforms.find(platform => platform !== host);
    const otherPack = JSON.parse(run('npm', ['pack', '--json', '--pack-destination', temporary], path.join(packages, `tsguard-${other}`)).stdout)[0];
    assert.match(run('npm', ['install', '--offline', '--ignore-scripts', '--no-audit', '--no-fund', '--no-save', path.join(temporary, otherPack.filename)], consumer, 1).stderr, /EBADPLATFORM/);
  }
  const packageBefore = fs.readFileSync(path.join(consumer, 'package.json'), 'utf8');
  const executor = manager === 'pnpm' ? 'pnpm' : 'npx';
  const execArgs = manager === 'pnpm' ? ['exec'] : ['--no-install'];
  assert.equal(run(executor, [...execArgs, 'tsguard', '--version'], consumer).stdout.trim(), `v${version}`);
  assert.match(run(executor, [...execArgs, 'tsguard', '--help'], consumer).stdout, /tsguard check/);
  assert.match(run(executor, [...execArgs, 'tsguard', 'not-a-command'], consumer, 3).stderr, /unknown command/);
  assert.equal(fs.readFileSync(path.join(consumer, 'package.json'), 'utf8'), packageBefore);
  assert.equal(fs.existsSync(path.join(consumer, 'node_modules/.cache/oxguard')), false);
  const installed = path.join(consumer, 'node_modules/@oxguard');
  assert.deepEqual(fs.readdirSync(installed).sort(), (manager === 'pnpm' ? ['tsguard'] : ['tsguard', `tsguard-${host}`]).sort());
  const launcher = path.join(installed, 'tsguard/bin/tsguard.cjs');
  const nativeDirectory = path.dirname(createRequire(fs.realpathSync(launcher)).resolve(`@oxguard/tsguard-${host}/package.json`));
  const installedBinary = path.join(nativeDirectory, 'bin', binaryName);

  // Exercise the actual gates with no analyzer listed as a consumer dependency.
  fs.mkdirSync(path.join(consumer, 'src'));
  fs.writeFileSync(path.join(consumer, 'src/add.ts'), 'export function add(a: number, b: number): number { return a + b; }\n');
  fs.writeFileSync(path.join(consumer, 'src/add.test.ts'), 'import { expect, test } from "vitest";\nimport { add } from "./add";\ntest("adds", () => { expect(add(1, 2)).toBe(3); });\n');
  const execute = (args, expected = 0) => run(executor, [...execArgs, 'tsguard', ...args, '--allow-pipe', '--dirs', 'src'], consumer, expected);
  execute(['doctor']);
  execute(['fix']);
  execute(['lint']);
  execute(['types']);
  execute(['fta']);
  execute(['coverage']);
  execute(['secrets']);
  const audit = execute(['audit']).stdout;
  assert.match(audit, /\[(?:OK|FAIL)\].*knip/);
  assert.match(audit, /\[OK\].*jscpd/);
  // Use a local test rule to exercise the real SAST engine without depending on
  // the live rule registry. Vulnerability auditing still uses the PM registry.
  const rules = path.join(consumer, 'node_modules/.cache/oxguard/rules');
  fs.mkdirSync(rules, { recursive: true });
  fs.writeFileSync(path.join(rules, 'test.yaml'), 'rules:\n  - id: tsguard-test-eval\n    languages: [typescript, javascript]\n    message: Avoid eval\n    severity: ERROR\n    pattern: eval($X)\n');
  assert.match(execute(['check']).stdout, /All checks passed/);
  fs.writeFileSync(path.join(consumer, 'src/add.ts'), 'export const unsafe = eval("1+1");\n');
  assert.match(execute(['security'], 1).stdout, /\[FAIL\].*opengrep SAST/);
  assert.equal(fs.readFileSync(path.join(consumer, 'package.json'), 'utf8'), packageBefore);
  assert.equal(fs.existsSync(path.join(consumer, 'biome.json')), false);
  assert.equal(fs.existsSync(path.join(consumer, 'tsconfig.json')), false);
  fs.writeFileSync(path.join(consumer, 'src/add.ts'), 'export const broken: number = "not a number";\n');
  execute(['types'], 1);
  fs.writeFileSync(path.join(consumer, 'src/add.ts'), 'export function add(a: number, b: number): number { return a + b; }\n');
  run('go', ['build', '-o', installedBinary, path.join(__dirname, 'testdata/cli.go')], root);
  for (const code of [0, 1, 3, 4, 5]) {
    run(process.execPath, [launcher, 'exit', String(code), 'ok'], consumer, code);
  }
  assert.equal(run(executor, [...execArgs, 'tsguard', 'exit', '37', 'argument with spaces'], consumer, 37).stdout.trim(), 'argument with spaces');
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
