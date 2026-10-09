'use strict';
const test = require('node:test');
const assert = require('node:assert/strict');
const { assess } = require('./maintainability-parity.cjs');
const spec = require('./maintainability-capabilities.json');
const evidence = () => ({ schema_version:'1', suite:'maintainability.installed', evaluator_sha256:'native-evidence', cases:spec.capabilities.map(c => ({id:c.tsguard,outcome:'pass'})) });
test('new Python gaps cannot become parity or release readiness', () => {
  const result=assess(evidence());
  assert.equal(result.tsguard_capabilities_passed,true);
  assert.equal(result.maintainability_parity_ready,false);
  assert.equal(result.release_ready,false);
  assert.ok(result.capabilities.every(c=>c.pyguard==='not_evaluated'));
});
test('missing, unsupported and duplicate native cases are rejected', () => {
  for (const mutate of [r=>r.cases.pop(),r=>r.cases[0].outcome='unsupported',r=>r.cases.push(r.cases[0])]) {
    const report=evidence();mutate(report);assert.throws(()=>assess(report));
  }
});
