'use strict';
const test = require('node:test');
const assert = require('node:assert/strict');
const { assess } = require('./maintainability-parity.cjs');
const spec = require('./maintainability-capabilities.json');
const evidence = () => ({ schema_version:'1', suite:'maintainability.installed', evaluator_sha256:'native-evidence', cases:spec.required_tsguard_cases.map(id => ({id,outcome:'pass'})) });
test('deferred Python capabilities do not block the selected Tsguard scope', () => {
  const result=assess(evidence());
  assert.equal(result.tsguard_capabilities_passed,true);
  assert.equal(result.maintainability_parity_ready,false);
  assert.equal(result.tsguard_scope_ready,true);
  assert.equal(result.pyguard_maintainability_ready,false);
  assert.equal(Object.hasOwn(result,'release_ready'),false);
  assert.equal(result.deferred_tsguard_capabilities[0].status,'not_implemented');
  assert.ok(result.capabilities.every(c=>c.pyguard==='not_evaluated'));
});
test('a failing native case prevents Tsguard readiness', () => {
  const report=evidence();report.cases.find(c=>c.id==='baseline.single_linter').outcome='fail';
  assert.equal(assess(report).tsguard_scope_ready,false);
});
test('missing, unsupported and duplicate native cases are rejected', () => {
  for (const mutate of [r=>r.cases.pop(),r=>r.cases[0].outcome='unsupported',r=>r.cases.push(r.cases[0])]) {
    const report=evidence();mutate(report);assert.throws(()=>assess(report));
  }
});
