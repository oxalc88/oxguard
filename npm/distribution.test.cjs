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

test(`${manager} packed distribution runs the Go CLI and forwards native process behavior`, { timeout: process.env.OXGUARD_EVAL_REPORT_DIR ? 600000 : 360000 }, async t => {
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
    // Isolate this negative install from the unpublished optional versions in
    // the real consumer. npm otherwise fails in optional-dependency deduplication
    // before it reaches the platform check (Invalid Version / edgesOut errors).
    const unsupportedConsumer = path.join(temporary, 'unsupported consumer');
    fs.mkdirSync(unsupportedConsumer);
    fs.writeFileSync(path.join(unsupportedConsumer, 'package.json'), JSON.stringify({ private: true, version: '1.0.0' }));
    assert.match(run('npm', ['install', '--offline', '--ignore-scripts', '--no-audit', '--no-fund', '--no-save', path.join(temporary, otherPack.filename)], unsupportedConsumer, 1).stderr, /EBADPLATFORM/);
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

  // Evaluate through the packed, installed launcher, before fixtures replace
  // the real binary later in this integration test. Keep scoring separate from
  // packaging assertions and record the exact revision/corpus/toolchain.
  if (process.env.OXGUARD_EVAL_REPORT_DIR) {
    const { evaluate } = require('../evals/run.cjs');
    const { compare, markdown } = require('../evals/compare.cjs');
    const reportDirectory = path.resolve(root, process.env.OXGUARD_EVAL_REPORT_DIR, `${manager}-${host}`);
    fs.mkdirSync(reportDirectory, { recursive: true });
    const candidate = evaluate({ command: [process.execPath, launcher], revision: process.env.OXGUARD_EVAL_REVISION || run('git', ['rev-parse', 'HEAD'], root).stdout.trim() });
    assert.deepEqual(candidate.toolchain, require('./toolchain.json'));
    fs.writeFileSync(path.join(reportDirectory, 'candidate.json'), JSON.stringify(candidate, null, 2) + '\n');
    const baselineRef = process.env.OXGUARD_EVAL_BASELINE_REF;
    if (baselineRef) {
      assert.match(baselineRef, /^[a-f0-9]{40}$/, 'Eval baseline must be a full commit SHA');
      // Same launcher/toolchain isolates native CLI capability changes. The
      // existing integration assertions independently protect package behavior.
      run('git', ['fetch', '--no-tags', 'origin', baselineRef], root);
      const baselineSource = path.join(temporary, 'baseline-source');
      run('git', ['worktree', 'add', '--detach', baselineSource, baselineRef], root);
      try {
        const baselineBinary = path.join(temporary, binaryName);
        run('go', ['build', '-trimpath', '-ldflags', `-X main.version=v${version}`, '-o', baselineBinary, '.'], path.join(baselineSource, 'tsguard'));
        fs.copyFileSync(baselineBinary, installedBinary);
        let baseline;
        try { baseline = evaluate({ command: [process.execPath, launcher], suite: 'baseline', revision: run('git', ['rev-parse', 'HEAD'], baselineSource).stdout.trim() }); }
        finally { fs.copyFileSync(nativeBinary, installedBinary); }
        fs.writeFileSync(path.join(reportDirectory, 'baseline.json'), JSON.stringify(baseline, null, 2) + '\n');
        const comparison = compare(baseline, candidate);
        fs.writeFileSync(path.join(reportDirectory, 'comparison.json'), JSON.stringify(comparison, null, 2) + '\n');
        fs.writeFileSync(path.join(reportDirectory, 'comparison.md'), markdown(comparison));
        assert.equal(comparison.candidate_ready, true, JSON.stringify(comparison));
      } finally { run('git', ['worktree', 'remove', '--force', baselineSource], root); }
    }
    assert.equal(candidate.totals.fail + candidate.totals.unsupported, 0, JSON.stringify(candidate.cases.filter(c => c.outcome !== 'pass' && c.outcome !== 'skipped')));
    t.diagnostic(`Capability evals: ${JSON.stringify(candidate.totals)}; detection: ${JSON.stringify(candidate.detection)}`);
  }

  // Exercise the actual gates with no analyzer listed as a consumer dependency.
  fs.mkdirSync(path.join(consumer, 'src'));
  fs.writeFileSync(path.join(consumer, 'src/add.ts'), 'export function add(a: number, b: number): number { return a + b; }\n');
  fs.writeFileSync(path.join(consumer, 'src/add.test.ts'), 'import { expect, test } from "vitest";\nimport { add } from "./add";\ntest("adds", () => { expect(add(1, 2)).toBe(3); });\n');
  const execute = (args, expected = 0) => run(executor, [...execArgs, 'tsguard', ...args, '--allow-pipe', ...(args.includes('--dirs') ? [] : ['--dirs', 'src'])], consumer, expected);
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
  // Use local providers so live rule/advisory changes do not block real SAST.
  // Installation above still uses the PM registry; only audit responses are fixed.
  const auditRegistry = spawn(process.execPath, [path.join(__dirname, 'testdata/audit-registry.cjs')], { stdio: ['ignore', 'ignore', 'inherit', 'ipc'] });
  t.after(() => auditRegistry.kill());
  const [auditProvider] = await once(auditRegistry, 'message');
  // pnpm 12 does not apply npm_config_registry to audit. Use the shared
  // project config so both PMs and audit-ci's child PM see this provider.
  fs.writeFileSync(path.join(consumer, '.npmrc'), `registry=${auditProvider.registry}\n`);
  const originalRegistry = process.env.npm_config_registry;
  process.env.npm_config_registry = auditProvider.registry;
  t.after(() => {
    if (originalRegistry === undefined) delete process.env.npm_config_registry;
    else process.env.npm_config_registry = originalRegistry;
  });
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
  // Semantic contracts are exercised through the packed, installed launcher.
  execute(['fix']);
  const structured = (args, expected = 0) => {
    const value = execute([...args, '--output', 'json'], expected);
    const result = JSON.parse(value.stdout);
    assert.equal(result.schema_version, '1');
    assert.equal(result.exit_code, expected);
    for (const key of ['findings', 'measurements', 'artifacts', 'diagnostics']) assert.ok(Array.isArray(result[key]), key);
    assert.equal(Object.hasOwn(result, 'output'), false);
    for (const diagnostic of result.diagnostics) assert.ok(fs.existsSync(path.join(consumer, diagnostic.path)));
    return result;
  };
  // Real compiler-backed call resolution: imports, methods, aliases, nested
  // functions, repeated call sites, recursion, exclusions and zero callers.
  fs.mkdirSync(path.join(consumer, 'calls/ignored'), { recursive: true });
  fs.writeFileSync(path.join(consumer, 'calls/core.ts'), 'export function target() { return 1; }\nexport class Service { method() { return target(); } }\nexport function unused() { return 0; }\nexport function recursive() { return recursive(); }\n');
  fs.writeFileSync(path.join(consumer, 'calls/use.ts'), 'import { target as alias, Service } from "./core";\nexport function first() { alias(); alias(); new Service().method(); }\nexport const second = () => alias();\nexport function outer() { function nested() { alias(); } return nested(); }\nalias();\n');
  fs.writeFileSync(path.join(consumer, 'calls/ignored/skip.ts'), 'import { target } from "../core"; export function excluded() { target(); }\n');
  const criticalArgs = ['criticality', '--dirs', 'calls', '--exclude', 'calls/ignored'];
  fs.writeFileSync(path.join(consumer, 'tsconfig.json'), JSON.stringify({ compilerOptions: { module: 'NodeNext', moduleResolution: 'NodeNext', paths: { '@core': ['./calls/core.ts'] } }, include: ['calls/**/*.ts'] }));
  fs.writeFileSync(path.join(consumer, 'calls/use.ts'), fs.readFileSync(path.join(consumer, 'calls/use.ts'), 'utf8').replace('"./core"', '"@core"'));
  const critical = structured(criticalArgs);
  assert.equal(critical.status, 'advisory');
  const inDegree = (symbol) => critical.measurements.find(m => m.metric === 'criticality.in_degree' && m.location.symbol === symbol)?.value;
  assert.equal(inDegree('target'), 4); // method, first, second, nested; no module caller
  assert.equal(inDegree('Service.method'), 1);
  assert.equal(inDegree('unused'), 0);
  assert.equal(inDegree('recursive'), 1);
  assert.equal(inDegree('outer.nested'), 1);
  assert.ok(critical.findings.every(f => f.rule === 'tsguard.criticality.ranked' && f.level === 'structure' && f.status === 'advisory'));
  assert.ok(critical.artifacts.some(a => a.kind === 'criticality' && a.path === 'CRITICALITY.md'));
  const criticalReport = fs.readFileSync(path.join(consumer, 'CRITICALITY.md'), 'utf8');
  assert.match(criticalReport, /\| 1 \| `calls\/core.ts:1 target` \| 4 \|/);
  assert.deepEqual(structured(criticalArgs).findings.map(f => f.id), critical.findings.map(f => f.id));
  assert.equal(fs.readFileSync(path.join(consumer, 'CRITICALITY.md'), 'utf8'), criticalReport);
  const criticalAgent = execute([...criticalArgs, '--output', 'agent']).stdout;
  assert.ok(Buffer.byteLength(criticalAgent) <= 6144 && criticalAgent.split('\n').length <= 27);
  assert.match(execute(criticalArgs).stdout, /\[OK\].*criticality.*CRITICALITY.md/);
  assert.ok(structured(['audit', '--dirs', 'calls', '--exclude', 'calls/ignored']).artifacts.some(a => a.kind === 'criticality'));
  fs.writeFileSync(path.join(consumer, 'tsconfig.json'), '{broken');
  const invalidCritical = structured(criticalArgs);
  assert.equal(invalidCritical.status, 'error');
  assert.equal(invalidCritical.findings[0].category, 'invalid_configuration', JSON.stringify(invalidCritical));
  fs.rmSync(path.join(consumer, 'tsconfig.json'));
  fs.rmSync(path.join(consumer, 'calls'), { recursive: true });
  assert.equal(structured(['types']).status, 'pass');
  const completeCheck = structured(['check']);
  assert.ok(['pass', 'advisory'].includes(completeCheck.status));
  assert.equal(completeCheck.assessment, 'complete');
  assert.deepEqual(completeCheck.gates.map(g => g.name), ['lint', 'fta', 'types', 'coverage', 'secrets', 'dependencies', 'security']);
  assert.ok(completeCheck.gates.every(g => g.normalization === 'complete' && ['passed', 'advisory'].includes(g.status)));
  assert.equal(completeCheck.measurements.filter(m => m.metric.startsWith('coverage.') && m.location.file === '').length, 4);
  const pipedAgent = run(executor, [...execArgs, 'tsguard', 'check', '--output', 'agent', '--dirs', 'src'], consumer);
  assert.match(pipedAgent.stdout, /^(?:PASS|ADVISORY)\n/);
  const ftaPass = structured(['fta']);
  assert.equal(ftaPass.status, 'pass');
  // FTA omits tiny files; this fixture has enough lines for a real score.
  fs.mkdirSync(path.join(consumer, 'src/score'));
  fs.writeFileSync(path.join(consumer, 'src/score/score.ts'), 'export function score(x: number) {\n  if (x === 1) return 1;\n  if (x === 2) return 2;\n  if (x === 3) return 3;\n  if (x === 4) return 4;\n  if (x === 5) return 5;\n  return 0;\n}\n');
  assert.ok(structured(['fta']).measurements.some(m => m.metric === 'fta.score' && m.location.file === 'src/score/score.ts'));
  const ftaFailed = structured(['fta', '--dirs', 'src/score', '--max-fta-score', '1'], 1);
  const scoreFinding = ftaFailed.findings.find(f => f.rule === 'tsguard.fta.score_exceeded');
  assert.ok(scoreFinding);
  assert.equal(scoreFinding.location.file, 'src/score/score.ts');
  assert.equal(scoreFinding.threshold, 1);
  assert.ok(scoreFinding.observed > scoreFinding.threshold);
  assert.ok(ftaFailed.measurements.some(m => m.metric === 'fta.score'));
  fs.rmSync(path.join(consumer, 'src/score'), { recursive: true });
  fs.writeFileSync(path.join(consumer, 'src/add.ts'), 'export const broken: number = "not a number";\nexport const other: boolean = 123;\n');
  const typesFailed = structured(['types', '--tail', '1'], 1);
  assert.equal(typesFailed.status, 'fail');
  assert.equal(typesFailed.findings.length, 2);
  assert.ok(typesFailed.findings.every(f => f.rule === 'TS2322' && f.category === 'quality' && f.location.file === 'src/add.ts'));
  assert.deepEqual(structured(['types'], 1).findings.map(f => f.id), typesFailed.findings.map(f => f.id));
  const agentTypes = execute(['types', '--output', 'agent'], 1).stdout;
  assert.match(agentTypes, /^FAIL\n/);
  assert.match(agentTypes, /findings: 2/);
  assert.match(agentTypes, /TS2322 src\/add.ts:/);
  assert.ok(Buffer.byteLength(agentTypes) <= 6144);
  fs.writeFileSync(path.join(consumer, 'src/add.ts'), 'export const bad:number=1;\n');
  const lintFailed = structured(['lint'], 1);
  assert.ok(lintFailed.findings.some(f => f.gate === 'lint' && f.category === 'quality' && f.rule === 'format'));
  const failFast = structured(['check'], 1);
  assert.ok(failFast.findings.every(f => f.gate === 'lint'));
  assert.ok(failFast.diagnostics.every(d => d.gate === 'lint'));
  fs.writeFileSync(path.join(consumer, 'src/add.ts'), '// first\nexport function lintLocation() {\n  debugger;\n}\n');
  const positionedLint = structured(['lint'], 1).findings.find(f => f.rule === 'lint/suspicious/noDebugger');
  assert.ok(positionedLint);
  assert.equal(positionedLint.location.line, 3);
  fs.writeFileSync(path.join(consumer, 'src/add.ts'), 'export const unsafe = eval("1+1");\n');
  const sastFailed = structured(['security'], 1);
  assert.ok(sastFailed.findings.some(f => f.rule.endsWith('tsguard-test-eval') && f.category === 'quality' && f.location.file === 'src/add.ts'));
  fs.writeFileSync(path.join(consumer, 'src/add.ts'), 'export function add(a: number, b: number): number { return a + b; }\n');
  for (const invalid of [['--timout', '30'], ['--timeout'], ['--output', 'xml']]) {
    const result = structured(['types', ...invalid], 3);
    assert.equal(result.status, 'error');
    assert.equal(result.findings[0].category, 'invalid_configuration');
  }
  // Explicit root is forwarded intact from a nested cwd through Node to Go.
  fs.mkdirSync(path.join(consumer, 'nested'));
  const rooted = run(executor, [...execArgs, 'tsguard', 'types', '--root', consumer, '--dirs', 'src', '--output', 'json', '--log-file', 'project log.txt'], path.join(consumer, 'nested'));
  assert.equal(JSON.parse(rooted.stdout).status, 'pass');
  assert.equal(JSON.parse(rooted.stdout).artifacts[0].path, 'nested/project log.txt');
  assert.ok(fs.existsSync(path.join(consumer, JSON.parse(rooted.stdout).artifacts[0].path)));
  // Missing package-owned Node executable is an execution failure, not a TS finding.
  // Spawn Go directly: Node is the deliberately missing analyzer dependency.
  const missingDependency = spawnSync(installedBinary, ['types', '--output', 'json', '--dirs', 'src'], {
    cwd: consumer, encoding: 'utf8', env: { ...process.env, TSGUARD_RUNTIME: path.dirname(path.dirname(launcher)), TSGUARD_NODE: path.join(temporary, 'absent-node') },
  });
  assert.ifError(missingDependency.error);
  assert.equal(missingDependency.status, 1);
  assert.equal(JSON.parse(missingDependency.stdout).findings[0].category, 'tool_missing');
  run('go', ['build', '-o', installedBinary, path.join(__dirname, 'testdata/cli.go')], root);
  const contextProcess = spawnSync(process.execPath, [launcher, 'context', '--output', 'json', 'argument with spaces'], {
    cwd: consumer, encoding: 'utf8', env: { ...process.env, TSGUARD_FIXTURE_VALUE: 'forwarded-value' },
  });
  assert.ifError(contextProcess.error);
  assert.equal(contextProcess.status, 0);
  const context = JSON.parse(contextProcess.stdout);
  assert.equal(fs.realpathSync(context.cwd), fs.realpathSync(consumer));
  assert.deepEqual(context.argv, ['--output', 'json', 'argument with spaces']);
  assert.equal(context.value, 'forwarded-value');
  assert.ok(context.runtime && context.node && context.opengrep);
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
