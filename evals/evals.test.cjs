'use strict';
const { test } = require('node:test');
const assert = require('node:assert/strict');
const { compare } = require('./compare.cjs');
const { detection, corpusDigest, evaluate } = require('./run.cjs');
const corpus = require('./cases.json');
function report(outcomes) {
  return { schema_version: '1', corpus_sha256: corpusDigest(), revision: 'test', evaluator_sha256: 'oracle', environment: { platform: 'test', arch: 'test', node: 'test' }, toolchain: { typescript: '5.9.3' }, totals: { fail: outcomes.filter(x => x === 'fail').length, unsupported: outcomes.filter(x => x === 'unsupported').length }, cases: outcomes.map((outcome, i) => ({ id: String(i), outcome, duration_ms: 1, stdout_bytes: 2 })) };
}
test('comparison distinguishes gains, losses, retained capabilities and unresolved cases', () => {
  const value = compare(report(['unsupported', 'pass', 'pass', 'fail']), report(['pass', 'fail', 'pass', 'fail']));
  assert.deepEqual(value.totals, { gained: 1, regressed: 1, retained: 1, unresolved: 1, applicability_changed: 0 });
  assert.equal(value.candidate_ready, false);
  assert.equal(compare(report(['fail']), report(['pass'])).candidate_ready, true);
});
test('candidate failure blocks even when baseline already failed', () => {
  assert.equal(compare(report(['fail']), report(['fail'])).candidate_ready, false);
  assert.equal(compare(report(['unsupported']), report(['unsupported'])).candidate_ready, false);
});
test('different datasets, environments, toolchains and case sets are rejected', () => {
  for (const mutate of [r => r.corpus_sha256 = 'different', r => r.evaluator_sha256 = 'different', r => r.environment.arch = 'different', r => r.toolchain.typescript = 'different', r => r.cases[0].id = 'different']) {
    const a = report(['pass']); const b = report(['pass']); mutate(b);
    assert.throws(() => compare(a, b), /Incomparable/);
  }
});
test('detection scores misses, duplicate false positives and clean controls', () => {
  const expected = [{ rule: 'TS2322', category: 'quality', file: 'src/index.ts', line: 1 }];
  const actual = [{ rule: 'TS2322', category: 'quality', location: { file: 'src/index.ts', line: 1 } }];
  assert.deepEqual(detection(expected, actual), { true_positive: 1, false_positive: 0, false_negative: 0 });
  assert.deepEqual(detection(expected, []), { true_positive: 0, false_positive: 0, false_negative: 1 });
  assert.deepEqual(detection(expected, [...actual, ...actual]), { true_positive: 1, false_positive: 1, false_negative: 0 });
  assert.deepEqual(detection([], actual), { true_positive: 0, false_positive: 1, false_negative: 0 });
});
test('a missing or malformed CLI never becomes unsupported or passing', () => {
  const failed = evaluate({ command: [process.execPath, '-e', 'process.exit(1)', '--'] });
  assert.equal(failed.totals.fail, corpus.cases.length);
  assert.equal(failed.totals.unsupported, 0);
  const malformed = evaluate({ command: [process.execPath, '-e', 'console.log("--output json\\ntsguard criticality ")', '--'] });
  assert.ok(malformed.totals.fail > 0);
  assert.equal(malformed.totals.pass, 0);
});
test('a working historical CLI without JSON is unsupported, not a pass', () => {
  const old = evaluate({ command: [process.execPath, '-e', 'console.log("legacy help")', '--'] });
  assert.equal(old.totals.unsupported, corpus.cases.length);
  assert.equal(old.totals.pass, 0);
  assert.equal(old.detection.recall, null);
});
test('case IDs and historical origins exist and expected answers are independent files', () => {
  assert.equal(new Set(corpus.cases.map(c => c.id)).size, corpus.cases.length);
  assert.ok(corpus.cases.every(c => c.origin.startsWith('https://github.com/oxalc88/oxguard/') && c.expected && c.fixture));
  assert.match(corpusDigest(), /^[a-f0-9]{64}$/);
});

test('skips are applicability changes and must not masquerade as gains', () => {
  const value = compare(report(['skipped']), report(['pass']));
  assert.equal(value.totals.gained, 0);
  assert.equal(value.totals.applicability_changed, 1);
  assert.equal(compare(report(['pass']), report(['skipped'])).candidate_ready, false);
});
test('duplicate cases and forged passing totals cannot conceal a regression', () => {
  const before = report(['pass', 'pass']); const after = report(['pass', 'pass']); after.cases[1].id = '0';
  assert.throws(() => compare(before, after), /Incomparable/);
  const failed = report(['fail']); failed.totals = { fail: 0, unsupported: 0 };
  assert.equal(compare(report(['pass']), failed).candidate_ready, false);
});
test('empty or unidentified reports cannot produce a green comparison', () => {
  for (const mutate of [r => r.schema_version = '2', r => delete r.corpus_sha256, r => delete r.evaluator_sha256, r => r.cases = []]) {
    const a = report(['pass']); const b = report(['pass']); mutate(a); mutate(b);
    assert.throws(() => compare(a, b), /Incomparable/);
  }
});
