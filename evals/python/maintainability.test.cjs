'use strict';
const { test } = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const os = require('node:os');
const path = require('node:path');
const crypto = require('node:crypto');
const { spawnSync } = require('node:child_process');
const { evaluate, definitions } = require('./maintainability.cjs');
const spec = require('../maintainability-capabilities.json');

const harness = require.resolve('./maintainability.cjs');
const declared = () => spec.capabilities.flatMap(capability => Object.entries(capability.pyguard.cases)
  .flatMap(([kind, ids]) => ids.map(id => ({ id, capability: capability.id, kind }))));

function temporary(t) {
  const dir = fs.mkdtempSync(path.join(os.tmpdir(), 'pyguard-maint-test-'));
  t.after(() => fs.rmSync(dir, { recursive: true, force: true }));
  return dir;
}

// Canned normalized results keyed by the fixture project the stand-in runs in
// (the project directory is named after the case id). Answers mirror the
// known answers pinned in the harness; the real-binary test verifies them
// independently when PYGUARD_EVAL_BINARY is provided.
const ANSWERS = {
  'criticality.distinct_callers_cross_module': [['src/core.py', 'core.target', 3, 1], ['src/caller_one.py', 'caller_one.one', 0, 4]],
  'criticality.uncalled_function_zero': [['src/lonely.py', 'lonely.never_called', 0, 1]],
  'criticality.same_caller_not_inflated': [['src/hub.py', 'hub.hub', 1, 1], ['src/busy.py', 'busy.busy', 0, 4]],
  'criticality.self_recursion_counts_once': [['src/recursion.py', 'recursion.countdown', 1, 1]],
};

// Score-verified against complexipy 8.3.0 by the real-binary test: 0 flat,
// 21 chain-of-six, 36 chain-of-eight, 20 two-chains-of-four, 21 in boundary.
const COMPLEXIPI = {
  'complexipy.cognitive.exceeded_is_advisory_warning': [['src/complexity.py', 'flat', 0], ['src/complexity.py', 'above', 21]],
  'complexipy.cognitive.flat_function_no_finding': [['src/flat.py', 'flat', 0]],
  'complexipy.cognitive.nested_never_blocking': [['src/nested.py', 'deeply_nested', 36]],
  'complexipy.cognitive.test_files_excluded': [['src/main.py', 'flat', 0]],
  'complexipy.cognitive.nested_function_folds_into_outer': [['src/nested_function.py', 'outer', 36]],
  'complexipy.cognitive.threshold_boundary': [['src/boundary.py', 'boundary', 20], ['src/boundary.py', 'above', 21]],
};

function standInBinary(t) {
  const file = path.join(temporary(t), 'pyguard-stand-in.cjs');
  fs.writeFileSync(file, `'use strict';
const path = require('node:path');
const answers = ${JSON.stringify(ANSWERS)};
const cognitive = ${JSON.stringify(COMPLEXIPI)};
const caseId = path.basename(path.dirname(process.cwd()));
const variant = path.basename(process.cwd());
if (variant === 'baseline' || cognitive[caseId]) {
  const rows = variant === 'baseline' ? [] : cognitive[caseId];
  const measurements = rows.map(([file, symbol, value]) => ({ metric: 'complexipy.cognitive', level: 'code', location: { file, symbol }, value, unit: 'score', threshold: 20 }));
  const findings = rows.filter(([, , value]) => value > 20).map(([file, symbol, value]) => ({ level: 'code', gate: 'radon', rule: 'pyguard.complexipy.cognitive_exceeded', severity: 'warning', status: 'advisory', category: 'quality', location: { file, symbol }, observed: value, threshold: 20, evidence: 'Cognitive complexity exceeds 20.', diagnostics: [] }));
  const status = findings.length ? 'advisory' : 'pass';
  const gateStatus = findings.length ? 'advisory' : 'passed';
  process.stdout.write(JSON.stringify({ schema_version: '1', status, assessment: 'complete', gates: [{ name: 'radon', status: gateStatus, normalization: 'complete' }], exit_code: 0, findings, measurements, artifacts: [], diagnostics: [] }));
  process.exit(0);
}
const rows = answers[caseId];
if (!rows) { process.stdout.write('unexpected project ' + caseId); process.exit(0); }
const wrong = process.env.PYGUARD_EVAL_MOCK_WRONG_CASE;
const measurements = rows.map(([file, symbol, value, line], index) => ({ metric: 'criticality.in_degree', level: 'structure', location: { file, line, symbol }, value: wrong === caseId && index === 0 ? value + 1 : value, unit: 'callers' }));
const findings = measurements.filter(m => m.value > 0).map(m => ({ rule: 'pyguard.criticality.ranked', level: 'structure', severity: 'info', location: m.location, observed: m.value, evidence: m.location.symbol }));
const status = findings.length ? 'advisory' : 'passed';
process.stdout.write(JSON.stringify({ schema_version: '1', status, assessment: 'complete', gates: [{ name: 'criticality', status }], exit_code: 0, findings, measurements, artifacts: [], diagnostics: [] }));
`);
  return file;
}

function standInInvoke(binary, env = {}) {
  return ({ project }) => {
    const response = spawnSync(process.execPath, [binary], { cwd: project, encoding: 'utf8', env: { ...process.env, ...env }, timeout: 30000 });
    return { status: response.status, stdout: response.stdout || '', stderr: response.stderr || '', error: response.error && response.error.message };
  };
}

function cli(args, { env = {}, cwd } = {}) {
  return spawnSync(process.execPath, [harness, ...args], { encoding: 'utf8', env: { ...process.env, ...env }, cwd, timeout: 120000 });
}

test('harness cases match the capability spec exactly', () => {
  const expected = declared();
  assert.equal(new Set(expected.map(d => d.id)).size, expected.length);
  assert.deepEqual(definitions.map(d => d.id), expected.map(d => d.id));
  definitions.forEach((definition, i) => {
    assert.equal(definition.capability, expected[i].capability);
    assert.equal(definition.kind, expected[i].kind);
    assert.ok(spec.capabilities.some(capability => capability.id === definition.capability));
  });
  assert.deepEqual([...new Set(definitions.map(d => d.capability))], ['structure.function_graph', 'quality.cognitive_complexity']);
  for (const kind of ['positive', 'negative', 'boundary']) assert.ok(expected.some(d => d.kind === kind));
});

test('the report runs every declared case exactly once and passes with a faithful stand-in', t => {
  const binary = standInBinary(t);
  const report = evaluate({ binary, python: process.execPath, root: temporary(t), invoke: standInInvoke(binary) });
  assert.equal(report.schema_version, '1');
  assert.equal(report.suite, 'maintainability.python');
  assert.equal(report.evaluator_sha256, crypto.createHash('sha256').update(fs.readFileSync(harness)).digest('hex'));
  assert.deepEqual(report.cases.map(c => c.id).toSorted(), declared().map(d => d.id).toSorted());
  assert.equal(new Set(report.cases.map(c => c.id)).size, report.cases.length);
  assert.equal(report.cases.length, definitions.length);
  assert.ok(report.cases.every(c => c.outcome === 'pass'), JSON.stringify(report.cases, null, 2));
  for (const entry of report.cases) {
    const expected = declared().find(d => d.id === entry.id);
    assert.equal(entry.capability, expected.capability);
    assert.equal(entry.kind, expected.kind);
    assert.ok(Number.isFinite(entry.duration_ms) && entry.duration_ms >= 0);
  }
});

test('a failing assertion is recorded as a failing case and the run continues', t => {
  const binary = standInBinary(t);
  const report = evaluate({ binary, python: process.execPath, root: temporary(t), invoke: standInInvoke(binary, { PYGUARD_EVAL_MOCK_WRONG_CASE: 'criticality.uncalled_function_zero' }) });
  assert.equal(report.cases.length, definitions.length);
  assert.deepEqual(report.cases.filter(c => c.outcome === 'fail').map(c => c.id), ['criticality.uncalled_function_zero']);
  const failure = report.cases.find(c => c.outcome === 'fail');
  assert.match(failure.error, /criticality\.in_degree/);
  assert.ok(report.cases.filter(c => c.id !== failure.id).every(c => c.outcome === 'pass'));
});

test('a binary that cannot start fails every case instead of aborting the run', t => {
  const root = temporary(t);
  const report = evaluate({ binary: path.join(root, 'missing-pyguard'), python: process.execPath, root });
  assert.equal(report.cases.length, definitions.length);
  assert.ok(report.cases.every(c => c.outcome === 'fail'));
  assert.ok(report.cases.every(c => /could not be started/.test(c.error)));
});

test('the CLI writes the report and exits 0 on passes, 1 on a failing case', t => {
  const binary = standInBinary(t);
  const root = temporary(t);
  const passing = path.join(root, 'passing.json');
  const response = cli(['--binary', binary, '--python', process.execPath, '--output', passing]);
  assert.equal(response.status, 0, response.stderr || response.stdout);
  const report = JSON.parse(fs.readFileSync(passing, 'utf8'));
  assert.equal(report.suite, 'maintainability.python');
  assert.ok(report.cases.every(c => c.outcome === 'pass'));
  const failing = path.join(root, 'failing.json');
  const failed = cli(['--binary', binary, '--python', process.execPath, '--output', failing], { env: { PYGUARD_EVAL_MOCK_WRONG_CASE: 'criticality.self_recursion_counts_once' } });
  assert.equal(failed.status, 1, failed.stderr || failed.stdout);
  const failedReport = JSON.parse(fs.readFileSync(failing, 'utf8'));
  assert.deepEqual(failedReport.cases.filter(c => c.outcome === 'fail').map(c => c.id), ['criticality.self_recursion_counts_once']);
});

test('the CLI resolves a relative --binary path against the invocation cwd', t => {
  const binary = standInBinary(t);
  const root = temporary(t);
  const passing = path.join(root, 'relative.json');
  // The temp dir may live on a different drive than the checkout (e.g. C: vs
  // D: on Windows), so run the CLI from the stand-in's directory and pass a
  // sibling-relative path that is guaranteed to resolve against the invocation cwd.
  const relative = path.join('.', path.basename(binary));
  const response = cli(['--binary', relative, '--python', process.execPath, '--output', passing], { cwd: path.dirname(binary) });
  assert.equal(response.status, 0, response.stderr || response.stdout);
  const report = JSON.parse(fs.readFileSync(passing, 'utf8'));
  assert.ok(report.cases.every(c => c.outcome === 'pass'));
});

test('a built pyguard binary reproduces the known answers', { skip: !(process.env.PYGUARD_EVAL_BINARY && (process.env.PYGUARD_EVAL_PYTHON || process.env.EVAL_PY)) }, t => {
  const report = evaluate({ binary: process.env.PYGUARD_EVAL_BINARY, python: process.env.PYGUARD_EVAL_PYTHON || process.env.EVAL_PY, root: temporary(t) });
  assert.deepEqual(report.cases.filter(c => c.outcome !== 'pass'), [], JSON.stringify(report.cases, null, 2));
});
