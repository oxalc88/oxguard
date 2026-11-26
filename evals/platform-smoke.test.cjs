'use strict';

const { test } = require('node:test');
const assert = require('node:assert/strict');
const { tsguard, python, pythonMaintainability, selectCases } = require('./platform-smoke.cjs');

test('platform smoke selections reference real known answers and never replace full evals', () => {
  for (const [cases, ids] of [
    [require('./cases.json').cases, tsguard],
    [require('./python/cases.json').cases, python],
    [require('./python/maintainability.cjs').definitions, pythonMaintainability],
  ]) {
    assert.equal(selectCases(cases), cases);
    assert.ok(ids.length < cases.length);
    assert.deepEqual(selectCases(cases, ids).map(c => c.id).sort(), [...ids].sort());
    for (const invalid of [[], [ids[0], ids[0]], ['unknown-case']]) {
      assert.throws(() => selectCases(cases, invalid), /Platform smoke selection|Unknown platform smoke/);
    }
  }
});

test('partial capability reports cannot be written as full candidate evidence', () => {
  assert.throws(() => require('./run.cjs').evaluate({ caseIds: tsguard }), /cannot write full eval evidence/);
  assert.throws(() => require('./run.cjs').evaluate({ caseIds: tsguard, suite: 'platform-smoke', reportFile: 'candidate.json' }), /cannot write full eval evidence/);
  assert.throws(() => require('./python/run.cjs').evaluate({ caseIds: python, reportDirectory: 'candidate' }), /cannot write full eval evidence/);
});
