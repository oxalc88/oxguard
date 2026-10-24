'use strict';
const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const os = require('node:os');
const path = require('node:path');
const { createHash } = require('node:crypto');
const { spawnSync } = require('node:child_process');

const analyzer = fs.readFileSync(path.join(__dirname, 'project.cjs'), 'utf8') + '\n' +
  fs.readFileSync(path.join(__dirname, 'change-metrics.cjs'), 'utf8');
const cognitive = {
  category: 'lint/complexity/noExcessiveCognitiveComplexity',
  message: 'Excessive complexity of 4 detected (max: 1).',
  location: { path: 'source-00000.ts', start: { line: 2, column: 1 } },
};

function analyze(t, comment, diagnostics) {
  const root = fs.mkdtempSync(path.join(os.tmpdir(), 'tsguard-change-test-'));
  t.after(() => fs.rmSync(root, { recursive: true, force: true }));
  const runtime = path.join(root, 'runtime');
  function write(file, text, executable = false) {
    fs.mkdirSync(path.dirname(file), { recursive: true });
    fs.writeFileSync(file, text);
    if (executable) fs.chmodSync(file, 0o755);
  }
  function pkg(name, script) {
    const directory = path.join(runtime, 'node_modules', name);
    write(path.join(directory, 'package.json'), JSON.stringify({ name, version: '0.0.0', main: 'index.cjs', bin: 'index.cjs' }));
    write(path.join(directory, 'index.cjs'), script);
    return directory;
  }
  // Only comment scanning is needed: the empty duplicate report never requests
  // token fingerprints. Keep TypeScript entirely independent of installed deps.
  pkg('typescript', String.raw`
    exports.ScriptTarget = { Latest: 99 };
    exports.LanguageVariant = { JSX: 1 };
    exports.SyntaxKind = { EndOfFileToken: 0, SingleLineCommentTrivia: 1, MultiLineCommentTrivia: 2 };
    exports.createScanner = (target, skipTrivia, variant, text) => {
      const comments = skipTrivia ? [] : text.match(/\/\/[^\n]*/g) || [];
      let index = -1;
      return { scan: () => ++index < comments.length ? 1 : 0, getTokenText: () => comments[index] };
    };
  `);
  const fta = pkg('fta-cli', '');
  for (const target of ['x86_64-unknown-linux-musl', 'aarch64-unknown-linux-musl', 'x86_64-apple-darwin', 'aarch64-apple-darwin']) {
    write(path.join(fta, 'binaries', `fta-${target}`, 'fta'), `#!/usr/bin/env node
      const fs = require('node:fs');
      const path = require('node:path');
      const directory = process.argv.at(-1);
      console.log(JSON.stringify(fs.readdirSync(directory).map(file => ({
        file_name: path.join(directory, file), fta_score: 1, cyclo: 1, halstead: { volume: 1 }, line_count: 3,
      }))));
    `, true);
  }
  pkg('@biomejs/biome', `
    process.stdout.write(require('node:fs').readFileSync(process.env.TSGUARD_TEST_BIOME_REPORT, 'utf8'));
  `);
  pkg('jscpd', `
    const fs = require('node:fs');
    const path = require('node:path');
    const output = process.argv[process.argv.indexOf('--output') + 1];
    fs.mkdirSync(output, { recursive: true });
    fs.writeFileSync(path.join(output, 'jscpd-report.json'), JSON.stringify({
      duplicates: [], statistics: { total: { clones: 0, duplicatedTokens: 0, duplicatedLines: 0, percentage: 0 } },
    }));
  `);
  const text = `${comment}\nexport function example() { return 1; }\n`;
  const snapshot = {
    modules: [{ id: 'example.ts', source_hash: createHash('sha256').update(text).digest('hex') }],
    functions: [{ id: 'example', location: { file: 'example.ts', line: 2, column: 1, symbol: 'example' },
      fingerprint: 'example-fingerprint', end_line: 2, end_column: 40 }],
  };
  const baselineRoot = path.join(root, 'baseline');
  const candidate = path.join(root, 'candidate');
  for (const directory of [baselineRoot, candidate]) write(path.join(directory, 'example.ts'), text);
  const report = path.join(root, 'biome-report.json');
  write(report, JSON.stringify({ diagnostics, summary: { unchanged: 1, skipped: 0, diagnosticsNotPrinted: 0 } }));
  const input = path.join(root, 'input.json');
  write(input, JSON.stringify({ runtime, baselineRoot, baselineSnapshot: snapshot, candidateSnapshot: snapshot, timeout: 5 }));
  const result = spawnSync(process.execPath, ['-e', analyzer, '@' + input], {
    cwd: candidate, encoding: 'utf8', timeout: 30000,
    env: { ...process.env, TSGUARD_TEST_BIOME_REPORT: report },
  });
  assert.ifError(result.error);
  assert.equal(result.status, 0, result.stderr);
  return JSON.parse(result.stdout);
}

const options = { skip: process.platform === 'win32' };
for (const category of ['suppressions/unused', 'suppressions/parse', 'suppressions/incorrect']) {
  test(`ignores ${category} while measuring cognitive complexity`, options, t => {
    const result = analyze(t, '// biome-ignore lint/performance/noAwaitInLoops: sequential work', [{ category }, cognitive]);
    assert.equal(result.error, undefined);
    assert.equal(result.partial, false);
    for (const profile of [result.comparison_metrics.baseline, result.comparison_metrics.candidate]) {
      assert.equal(profile.cognitive_complete, true);
      assert.equal(profile.functions.length, 1);
      assert.equal(profile.functions[0].cognitive_min, 4);
      assert.equal(profile.functions[0].cognitive_max, 4);
    }
  });
}
for (const category of ['lint/style/useConst', 'parse']) {
  test(`rejects unexpected ${category} diagnostics`, options, t => {
    const result = analyze(t, '// ordinary comment', [{ category }]);
    assert.deepEqual(result, { error: { category: 'analyzer_failure', message: 'Unexpected cognitive diagnostic' } });
  });
}
test('cognitive biome-ignore-start makes the file partial', options, t => {
  const result = analyze(t, '// biome-ignore-start lint/complexity/noExcessiveCognitiveComplexity: legacy logic', []);
  assert.equal(result.error, undefined);
  assert.equal(result.partial, true);
  for (const profile of [result.comparison_metrics.baseline, result.comparison_metrics.candidate]) {
    assert.equal(profile.cognitive_complete, false);
    assert.equal(profile.functions.length, 1);
    assert.equal(profile.functions[0].cognitive_min, 0);
    assert.equal(profile.functions[0].cognitive_max, null);
  }
});
