'use strict';

const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const crypto = require('node:crypto');
const { spawnSync } = require('node:child_process');
const { inventory } = require('./observe.cjs');

// Separate expectations for the new command; original P0 oracles stay unchanged.
function verify(language, result, response) {
  assert.equal(response.status, 1);
  assert.equal(result.command, 'diagnose');
  assert.equal(result.exit_code, response.status);
  assert.ok(Array.isArray(result.analyses) && result.analyses.length > 0);
  const analyses = new Map(result.analyses.map(a => [a.name, a]));
  assert.equal(analyses.size, result.analyses.length);
  for (const a of analyses.values()) {
    if (a.status === 'not_run') {
      assert.equal(a.name, 'change', `${a.name} was suppressed`);
      assert.match(a.reason, /baseline/i);
    }
    if (a.status === 'error' || a.normalization !== 'complete') {
      assert.ok(a.reason, `${a.name} hid incomplete coverage`);
      assert.equal(result.assessment, 'incomplete');
    }
  }
  if (language === 'typescript') {
    const findings = result.findings.filter(f => f.rule === 'tsguard.fta.score_exceeded');
    assert.deepEqual(findings.map(f => f.location.file).sort(), ['src/a.ts', 'src/b.ts']);
    const expected = { 'src/a.ts': 24.38161685613889, 'src/b.ts': 24.240152481108808 };
    for (const f of findings) {
      assert.equal(f.threshold, 1);
      assert.ok(Math.abs(f.observed - expected[f.location.file]) < 1e-9);
    }
    assert.equal(analyses.get('fta').normalization, 'complete');
    assert.ok(analyses.has('maintainability') && analyses.has('security') && analyses.has('change'));
  } else {
    const cc = result.findings.find(f => f.rule === 'pyguard.radon.cc_exceeded' && f.location.file === 'src/flow.py');
    const mi = result.findings.find(f => f.rule === 'pyguard.radon.mi_threshold_failed' && f.location.file === 'src/catalog.py');
    assert.ok(cc && mi, 'CC and MI must both be visible');
    assert.equal(cc.observed, 12);
    assert.equal(cc.threshold, 10);
    assert.ok(Math.abs(mi.observed - 16.180813146436567) < 1e-9);
    assert.equal(mi.threshold, 19);
    assert.equal(analyses.get('radon.cc').normalization, 'complete');
    assert.equal(analyses.get('radon.mi').normalization, 'complete');
    assert.ok(analyses.has('radon.halstead') && analyses.has('types') && analyses.has('secrets'));
  }
  const ids = new Set(result.diagnostics.map(d => d.id));
  assert.equal(ids.size, result.diagnostics.length, 'diagnostic IDs must be unique');
  for (const f of result.findings) for (const ref of f.diagnostics) assert.ok(ids.has(ref));
  for (const a of result.analyses) for (const ref of a.diagnostics) assert.ok(ids.has(ref));
}

function verifyReporters(options, language, source, output, env) {
  const project = path.join(output, language + '-missing-tools');
  fs.cpSync(source, project, { recursive: true });
  const emptyPath = path.join(output, language + '-empty-path');
  fs.mkdirSync(emptyPath);
  const isolated = { ...env, PATH: emptyPath, TSGUARD_NODE: path.join(emptyPath, 'missing-node') };
  const modes = {};
  for (const mode of ['json', 'agent', 'human']) {
    const args = ['diagnose', '--dirs', 'src', '--output', mode, '--allow-pipe', '--timeout', '1'];
    const response = spawnSync(options.binaries[language], args, { cwd: project, env: isolated, encoding: 'utf8', timeout: 30000, maxBuffer: 8 * 1024 * 1024 });
    modes[mode] = { status: response.status, stdout: response.stdout || '', stderr: response.stderr || '' };
    assert.ifError(response.error);
    assert.equal(response.status, 1);
    if (mode === 'json') {
      const result = JSON.parse(response.stdout);
      assert.equal(result.status, 'error');
      assert.equal(result.assessment, 'incomplete');
      assert.ok(result.findings.some(f => f.category === 'tool_missing'));
      assert.ok(result.findings.length > 10, 'missing tools must exercise bounded output');
      assert.ok(result.analyses.every(a => a.status === 'error' || (a.name === 'change' && a.status === 'not_run')));
      assert.ok(result.analyses.every(a => a.reason));
      modes.json.result = result;
    } else if (mode === 'agent') {
      assert.ok(Buffer.byteLength(response.stdout) <= 6144);
      assert.ok(response.stdout.trimEnd().split('\n').length <= 26);
      assert.match(response.stdout, /incomplete_analyses: \d+/);
      assert.ok(response.stdout.includes(`omitted: ${modes.json.result.findings.length - 10} (use --output json)`));
    } else {
      assert.match(response.stdout, /^ERROR: assessment incomplete/);
      for (const a of modes.json.result.analyses) assert.ok(response.stdout.includes(a.name + ':'));
    }
  }
  fs.writeFileSync(path.join(output, language + '-reporters.json'), JSON.stringify(modes, null, 2) + '\n');
}

function evaluate(options) {
  const output = path.resolve(options.output);
  fs.mkdirSync(output, { recursive: false });
  const env = { ...process.env, ...options.env };
  if (options.runtime) Object.assign(env, { TSGUARD_RUNTIME: options.runtime.root, TSGUARD_NODE: options.runtime.node || process.execPath });
  const cases = [];
  for (const language of ['typescript', 'python']) {
    const source = path.join(options.frozenFixtures, language, 'multiple-failures');
    const project = path.join(output, language);
    fs.cpSync(source, project, { recursive: true });
    const initial = inventory(project);
    const args = ['diagnose', '--dirs', 'src', '--output', 'json', '--timeout', String(options.timeout || 5)];
    const response = spawnSync(options.binaries[language], args, { cwd: project, env, encoding: 'utf8', timeout: 180000, maxBuffer: 32 * 1024 * 1024 });
    const entry = { language, outcome: 'fail', args, process_exit: response.status, stdout: response.stdout || '', stderr: response.stderr || '', initial, source_unchanged: JSON.stringify(initial) === JSON.stringify(inventory(project)) };
    try {
      assert.ifError(response.error);
      assert.equal(response.signal, null);
      entry.result = JSON.parse(response.stdout);
      verify(language, entry.result, response);
      assert.ok(entry.source_unchanged, 'diagnosis changed fixture inputs');
      verifyReporters(options, language, source, output, env);
      entry.outcome = 'pass';
    } catch (error) {
      entry.error = error.message;
    }
    fs.writeFileSync(path.join(output, language + '.json'), JSON.stringify(entry, null, 2) + '\n');
    cases.push(entry);
  }
  const summary = { schema_version: 1, scope: 'Native acceptance for diagnose; no new observed-agent samples', evaluator_sha256: crypto.createHash('sha256').update(fs.readFileSync(__filename)).digest('hex'), binaries: Object.fromEntries(Object.entries(options.binaries).map(([k, v]) => [k, { path: v, sha256: crypto.createHash('sha256').update(fs.readFileSync(v)).digest('hex') }])), cases: cases.map(({ language, outcome, error, source_unchanged }) => ({ language, outcome, error, source_unchanged })) };
  fs.writeFileSync(path.join(output, 'summary.json'), JSON.stringify(summary, null, 2) + '\n');
  return summary;
}

if (require.main === module) {
  const summary = evaluate(JSON.parse(fs.readFileSync(process.argv[2])));
  console.log(JSON.stringify(summary));
  process.exitCode = summary.cases.some(c => c.outcome !== 'pass') ? 1 : 0;
}

module.exports = { evaluate, verify };
