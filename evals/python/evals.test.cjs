'use strict';
const { test } = require('node:test');
const assert = require('node:assert/strict');
const { guard, checked, evaluate, relativeSource, radonValue } = require('./run.cjs');
const { parity } = require('../parity.cjs');
const mapping = require('../capabilities.json');
const python = require('./cases.json');
const ts = require('../cases.json');
const metadata = { schema_version: '1', revision: 'test', corpus_sha256: 'corpus', evaluator_sha256: 'evaluator', environment: { platform: 'linux', arch: 'x64', node: '24' }, toolchain: { python: '3.12' } };
function report(suite, cases) { return { ...metadata, suite, cases: cases.map(([id, outcome]) => ({ id, outcome, duration_ms: 1, stdout_bytes: 1 })) }; }
function result(behavior = 'pass', contract = 'unsupported') { return { behavior: report('pyguard.behavior', [['types.clean', behavior]]), parity: report('pyguard.parity', [['contract.json.clean', contract]]) }; }
test('Python behavior failures block even when baseline fails; known contract gaps stay unready', () => {
  assert.equal(guard(result(), result()).regression_guard_passed, true);
  assert.equal(guard(result(), result()).agent_ready, false);
  assert.equal(guard(result('fail'), result('fail')).regression_guard_passed, false);
});
test('Python parity gains are measurable and passing capabilities cannot be lost', () => {
  const gained = guard(result(), result('pass', 'pass'));
  assert.equal(gained.parity.totals.gained, 1); assert.equal(gained.agent_ready, true);
  for (const outcome of ['fail', 'unsupported', 'skipped']) assert.equal(guard(result('pass', 'pass'), result('pass', outcome)).regression_guard_passed, false);
});
test('new broken interfaces or concealing a known defect blocks the Python guard', () => {
  assert.equal(guard(result(), result('pass', 'fail')).regression_guard_passed, false);
  assert.equal(guard(result('pass', 'fail'), result()).regression_guard_passed, false);
  assert.equal(guard(result('pass', 'fail'), result('pass', 'fail')).agent_ready, false);
});
test('failed startup, signals and missing processes cannot be scored as capabilities', () => {
  for (const response of [{ error: Error('missing') }, { signal: 'SIGTERM', status: null }, { status: null }]) assert.throws(() => checked(response, 'case'));
  assert.throws(() => evaluate({ binary: '/does-not-exist', source: '.', python: process.execPath }), /CLI help/);
});
test('every shared capability maps to a real case; missing evaluations are explicit', () => {
  assert.equal(new Set(python.cases.map(c => c.id)).size, python.cases.length);
  assert.equal(new Set(mapping.capabilities.map(c => c.id)).size, mapping.capabilities.length);
  for (const c of mapping.capabilities) {
    if (c.tsguard !== null) assert.ok(ts.cases.some(x => x.id === c.tsguard));
    assert.ok(python.cases.some(x => x.id === c.pyguard && x.lane === c.lane));
  }
});
function reports() {
  return [report('candidate', ts.cases.map(c => [c.id, 'pass'])), report('pyguard.behavior', python.cases.filter(c => c.lane === 'behavior').map(c => [c.id, 'pass'])), report('pyguard.parity', python.cases.filter(c => c.lane === 'parity').map(c => [c.id, 'unsupported']))];
}
test('parity never equates missing Python contracts or untested TS behavior to passes', () => {
  const r = parity(...reports());
  assert.equal(r.pyguard_agent_ready, false); assert.equal(r.shared_capabilities_ready, false);
  assert.equal(r.capabilities.find(c => c.id === 'check.fail_fast').tsguard, 'pass');
  assert.equal(r.capabilities.find(c => c.id === 'contract.json.clean').pyguard, 'unsupported');
});
test('parity rejects mixed revisions, incomplete reports and forged outcomes', () => {
  for (const mutate of [r => r[1].revision = 'old', r => r[1].cases = [], r => r[1].cases[0].outcome = 'not_run', r => r[1].environment = { arch: 'arm64' }, r => r[1].cases.push(r[1].cases[0])]) {
    const r = reports(); mutate(r); assert.throws(() => parity(...r));
  }
});
test('Radon JSON paths use the same known answer on Windows and Unix', () => {
  for (const file of ['src/main.py', 'src\\main.py']) assert.equal(radonValue({ [file]: [{ complexity: 12 }] }), 12);
  assert.equal(radonValue({ 'other.py': [{ complexity: 12 }] }), undefined);
});
test('analyzer canonical paths remain project relative through a symlink', t => {
  const fs = require('node:fs'); const os = require('node:os'); const path = require('node:path');
  const root = fs.mkdtempSync(path.join(os.tmpdir(), 'pyguard-path-test-'));
  try {
    fs.mkdirSync(path.join(root, 'src')); fs.writeFileSync(path.join(root, 'src', 'main.py'), '');
    const alias = path.join(root, 'alias');
    try { fs.symlinkSync(root, alias, 'dir'); } catch (error) { t.skip(`Symlink unavailable: ${error.code}`); return; }
    assert.equal(relativeSource(alias, fs.realpathSync(path.join(root, 'src', 'main.py'))), 'src/main.py');
  } finally { fs.rmSync(root, { recursive: true, force: true }); }
});
