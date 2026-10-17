'use strict';

// Real stable project compilers through the installed launcher. The bundled
// compiler remains 5.9; native 7 is an isolated test dependency, not a fallback.
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const { performance } = require('node:perf_hooks');

function evaluate({ run, temporary, launcher, tool, registry, manager = 'npm' }) {
  const cases = [];
  for (const version of ['5.9.3', '7.0.2']) {
    const root = path.join(temporary, `TypeScript ${version} project`);
    fs.mkdirSync(root);
    fs.mkdirSync(path.join(root, 'src'));
    fs.writeFileSync(path.join(root, 'package.json'), JSON.stringify({ name: 'compiler-compatibility', private: true, devDependencies: { typescript: version } }));
    const install = manager === 'pnpm'
      ? ['install', '--ignore-scripts', '--registry', registry, '--store-dir', path.join(temporary, 'compiler-store')]
      : ['install', '--ignore-scripts', '--no-audit', '--no-fund', '--registry', registry];
    run(manager, install, root, 0, 240000);
    const config = { compilerOptions: { strict: true, target: 'ES2022', module: 'ESNext', moduleResolution: 'Bundler', noEmit: true }, include: ['src'] };
    const writeConfig = value => fs.writeFileSync(path.join(root, 'tsconfig.json'), JSON.stringify(value));
    const source = text => fs.writeFileSync(path.join(root, 'src/index.ts'), text);
    const check = (name, action) => {
      const started = performance.now();
      action();
      cases.push({ id: `typescript.${version}.${name}`, outcome: 'pass', duration_ms: Math.round(performance.now() - started) });
    };
    const invoke = (command, expected = 0, extra = []) => {
      const result = JSON.parse(run(process.execPath, [launcher, command, '--dirs', 'src', '--output', 'json', ...extra], root, expected).stdout);
      assert.equal(result.schema_version, '1');
      assert.equal(result.exit_code, expected);
      assert.ok(result.findings.every(f => f.category !== 'adapter_failure'));
      return result;
    };
    const incomplete = (result, gate) => {
      assert.equal(result.assessment, 'incomplete');
      assert.equal(result.gates.find(g => g.name === gate).normalization, 'partial');
      assert.ok(result.findings.some(f => f.rule === `tsguard.${gate}.not_evaluated`));
      assert.ok(!result.measurements.some(m => /^(?:module\.|function\.|criticality\.|delegation\.)/.test(m.metric)));
    };
    writeConfig(config);
    check('selected_compiler', () => {
      assert.equal(run(process.execPath, [tool, 'tsc', '--version'], root).stdout.trim(), `Version ${version}`);
    });
    check('types_clean', () => {
      source('export const value: number = 1;\n');
      assert.equal(invoke('types').status, 'pass');
    });
    check('types_findings', () => {
      source('export const first: number = "wrong";\nexport const second: boolean = 1;\n');
      const result = invoke('types', 1);
      assert.equal(result.assessment, 'complete');
      assert.deepEqual(result.findings.map(f => [f.rule, f.status, f.location?.file, f.location?.line]), [
        ['TS2322', 'blocking', 'src/index.ts', 1], ['TS2322', 'blocking', 'src/index.ts', 2],
      ]);
    });
    check('types_without_config', () => {
      fs.rmSync(path.join(root, 'tsconfig.json'));
      source('export const value: number = 1;\n');
      try {
        assert.equal(invoke('types').status, 'pass');
        assert.equal(fs.existsSync(path.join(root, 'tsconfig.json')), false);
      }
      finally { writeConfig(config); }
    });
    check('typed_lint_clean', () => {
      source('export const value: number = 1;\n');
      const result = invoke('typed-lint');
      assert.equal(result.status, 'pass');
      assert.equal(result.assessment, 'complete');
      const artifact = JSON.parse(fs.readFileSync(path.join(root, result.artifacts.find(a => a.kind === 'typed_lint_native').path)));
      assert.equal(artifact.project_compiler, version);
    });
    check('typed_lint_unsafe', () => {
      source('export const value: number = JSON.parse("1");\n');
      const result = invoke('typed-lint', 1);
      assert.equal(result.assessment, 'complete');
      assert.ok(result.findings.some(f => f.rule === 'typescript/no-unsafe-assignment' && f.status === 'blocking' && f.location.file === 'src/index.ts'));
    });
    check('typed_lint_missing_declaration', () => {
      source('import { absent } from "./missing";\nexport const value: number = absent;\n');
      const result = invoke('typed-lint');
      assert.equal(result.assessment, 'incomplete');
      assert.ok(result.findings.some(f => f.rule === 'tsguard.typed-lint.not_evaluated'));
      assert.ok(!result.findings.some(f => f.rule === 'typescript/no-unsafe-assignment'));
    });
    check('strict_policy_preserved', () => {
      source('export const value: number = 1;\n');
      writeConfig({ ...config, compilerOptions: { ...config.compilerOptions, strictNullChecks: false } });
      const before = fs.readFileSync(path.join(root, 'tsconfig.json'), 'utf8');
      try {
        assert.equal(invoke('typed-lint').assessment, 'incomplete');
        assert.equal(fs.readFileSync(path.join(root, 'tsconfig.json'), 'utf8'), before);
      } finally { writeConfig(config); }
    });
    check('inherited_config', () => {
      source('export const value: number = 1;\n');
      fs.writeFileSync(path.join(root, 'tsconfig.base.json'), JSON.stringify(config));
      writeConfig({ extends: './tsconfig.base.json' });
      try {
        assert.equal(invoke('types').status, 'pass');
        assert.equal(invoke('typed-lint').assessment, 'complete');
      } finally { writeConfig(config); }
    });
    check('removed_native_option', () => {
      source('export const value: number = 1;\n');
      writeConfig({ ...config, compilerOptions: { ...config.compilerOptions, baseUrl: '.' } });
      try {
        const result = invoke('types', version.startsWith('7.') ? 1 : 0);
        if (version.startsWith('7.')) assert.ok(result.findings.some(f => f.category === 'invalid_configuration'));
        assert.equal(invoke('typed-lint').assessment, 'incomplete');
      } finally { writeConfig(config); }
    });
    check('structural_capability', () => {
      source('export function target(x: number) { return x * 2; }\nexport function caller(x: number) { return target(x); }\n');
      for (const gate of ['criticality', 'maintainability', 'structure', 'smells']) {
        const result = invoke(gate);
        if (version.startsWith('7.')) {
          incomplete(result, gate);
          assert.ok(result.findings.some(f => f.evidence.includes('no older compiler was substituted')));
          assert.ok(!result.artifacts.some(a => ['criticality', 'maintainability_graph'].includes(a.kind)));
        } else {
          assert.equal(result.assessment, 'complete');
          const metric = gate === 'smells' ? 'function.branches' : 'criticality.in_degree';
          assert.ok(result.measurements.some(m => m.metric === metric && m.location.symbol === 'target' && m.value === (gate === 'smells' ? 0 : 1)));
        }
      }
    });
    if (version.startsWith('7.')) check('change_not_evaluated', () => {
      run('git', ['init', '--quiet'], root);
      run('git', ['add', 'src', 'tsconfig.json', 'package.json'], root);
      run('git', ['-c', 'user.name=OxGuard compatibility', '-c', 'user.email=compatibility@example.invalid', '-c', 'commit.gpgsign=false', 'commit', '--quiet', '-m', 'Known compiler fixture'], root);
      const result = invoke('change', 0, ['--baseline', 'HEAD']);
      assert.equal(result.assessment, 'incomplete');
      assert.ok(result.findings.some(f => f.rule === 'tsguard.change.INCOMPLETE_INPUT'));
      assert.ok(!result.findings.some(f => f.status === 'execution_error'));
    });
    if (version.startsWith('7.')) check('missing_native_compiler_is_error', () => {
      const executable = path.join(root, 'node_modules/typescript/bin/tsc');
      fs.renameSync(executable, executable + '.missing');
      try {
        const result = invoke('typed-lint', 1);
        assert.equal(result.assessment, 'incomplete');
        assert.ok(result.findings.some(f => f.status === 'execution_error' && f.category === 'analyzer_failure'));
        assert.ok(!result.findings.some(f => f.rule === 'tsguard.typed-lint.not_evaluated'));
      } finally { fs.renameSync(executable + '.missing', executable); }
    });
  }
  return { schema_version: '1', suite: 'typescript.installed', package_manager: manager, versions: ['5.9.3', '7.0.2'], passed: cases.length, cases,
    support: { '5.9.3': { types: true, typed_lint: true, compiler_api_analysis: true }, '7.0.2': { types: true, typed_lint: true, compiler_api_analysis: false } } };
}

module.exports = { evaluate };
