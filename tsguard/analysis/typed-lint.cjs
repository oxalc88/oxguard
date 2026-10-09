'use strict';
// Only upstream semantic rules. Biome owns syntax lint and formatting.
emitAnalysis(input => {
  const { ts, program, sources, relative, owned, root, options } = loadProject(input);
  const fs = require('node:fs');
  const path = require('node:path');
  const { spawnSync } = require('node:child_process');
  const directory = path.join(root, 'node_modules/.cache/oxguard/typed-lint');
  fs.mkdirSync(directory, { recursive: true });
  const report = { findings: [], measurements: [{metric: 'typed-lint.files', level: 'code', location: {file: '.'}, value: sources.length, unit: 'files'}], partial: false, limitation: '' };
  const unsupported = detail => ({ ...report, partial: true, limitation: `Native typed lint was not evaluated: ${detail}` });
  // Unsafe rules need valid types. Do not interpret unresolved imports as any leaks.
  const errors = program.getOptionsDiagnostics().concat(program.getSemanticDiagnostics()).filter(d => d.category === ts.DiagnosticCategory.Error);
  if (errors.length) return unsupported('resolve compiler errors and missing declarations with the types gate first.');
  if (!(options.strict || options.strictNullChecks) || options.strictNullChecks === false) return unsupported('strictNullChecks is required; project compiler options are preserved.');
  if (!sources.length) return unsupported('the requested scope has no analyzable source files.');
  const files = sources.map(s => path.resolve(s.fileName));
  // Keep argv below Windows limits; unsupported scope is visible, never a pass.
  if (files.reduce((total, file) => total + file.length + 3, 0) > 24000) return unsupported('selected file arguments exceed the 24000-character portable invocation budget; select a smaller scope.');
  const selected = new Set(files);
  const rules = {
    'no-unsafe-assignment': 'error', 'no-unsafe-type-assertion': 'error',
    'no-floating-promises': 'error', 'no-misused-promises': 'error',
    'no-unnecessary-condition': 'warn',
  };
  const config = path.join(directory, 'oxlint.json');
  fs.writeFileSync(config, JSON.stringify({ plugins: ['typescript'], rules: {} }));
  const tsconfig = path.join(directory, 'tsconfig.json');
  const original = path.join(root, 'tsconfig.json');
  fs.writeFileSync(tsconfig, JSON.stringify({
    ...(fs.existsSync(original) ? { extends: original } : { compilerOptions: { strict: true, target: 'ES2022', module: 'ESNext', moduleResolution: 'Bundler', jsx: 'preserve', allowJs: true } }),
    compilerOptions: { ...(fs.existsSync(original) ? {} : { strict: true, target: 'ES2022', module: 'ESNext', moduleResolution: 'Bundler', jsx: 'preserve', allowJs: true }), noEmit: true },
    files, include: [], exclude: [],
  }));
  const pkg = owned.resolve('oxlint/package.json');
  const manifest = JSON.parse(fs.readFileSync(pkg, 'utf8'));
  const executable = path.resolve(path.dirname(pkg), manifest.bin.oxlint);
  const backendResolver = require('node:module').createRequire(owned.resolve('oxlint-tsgolint/package.json'));
  const backend = backendResolver.resolve(`@oxlint-tsgolint/${process.platform}-${process.arch}/tsgolint${process.platform === 'win32' ? '.exe' : ''}`);
  const args = [executable, '--type-aware', '--type-check', '--disable-nested-config', '--no-ignore', '--config', config, '--tsconfig', tsconfig, '--format', 'json', '--allow', 'all'];
  for (const [rule, severity] of Object.entries(rules)) args.push(severity === 'error' ? '--deny' : '--warn', `typescript/${rule}`);
  args.push(...files);
  const result = spawnSync(process.execPath, args, { cwd: root, encoding: 'utf8', timeout: input.timeout * 1000,
    maxBuffer: 64 * 1024 * 1024, env: { ...process.env, OXLINT_TSGOLINT_PATH: backend } });
  if (result.error) throw Object.assign(result.error, { category: result.error.code === 'ETIMEDOUT' ? 'timeout' : 'analyzer_failure' });
  process.stderr.write(result.stderr);
  const native = JSON.parse(result.stdout);
  fs.writeFileSync(path.join(directory, 'native.json'), JSON.stringify({ oxlint: manifest.version,
    tsgolint: JSON.parse(fs.readFileSync(owned.resolve('oxlint-tsgolint/package.json'), 'utf8')).version,
    project_compiler: ts.version, compiler_semantics: 'typescript-go / TypeScript 7', selected_files: files.map(relative), exclude_tests: !!input.excludeTests, rules, native }, null, 2));
  if (!Array.isArray(native.diagnostics) || !Number.isInteger(native.number_of_files) || native.number_of_rules !== Object.keys(rules).length) throw new Error('Unsupported native typed-lint report');
  if (native.number_of_files !== files.length) return unsupported('the native backend did not report every selected file.');
  for (const d of native.diagnostics) {
    const match = /^typescript\(([^)]+)\)$/.exec(d.code || '');
    if (!match || typeof d.message !== 'string') throw new Error('Unsupported typed-lint diagnostic');
    if (!Object.hasOwn(rules, match[1])) {
      report.partial = true;
      report.limitation = 'Native TypeScript 7 configuration or type diagnostics prevent a complete semantic assessment; see the typed_lint_native artifact.';
      continue;
    }
    const file = path.resolve(root, d.filename);
    const point = d.labels?.find(l => Number.isInteger(l.span?.line) && Number.isInteger(l.span?.column))?.span;
    if (!selected.has(file) || !point || point.line < 1 || point.column < 1) throw new Error('Invalid typed-lint location');
    report.findings.push({ rule: `typescript/${match[1]}`, level: 'code', severity: rules[match[1]] === 'error' ? 'error' : 'warning',
      status: rules[match[1]] === 'error' ? 'blocking' : 'advisory', category: 'quality',
      location: { file: relative(file), line: point.line, column: point.column }, evidence: d.message });
  }
  if (result.status !== 0 && !native.diagnostics.length) throw new Error('Native typed lint failed without diagnostics');
  return report;
});
