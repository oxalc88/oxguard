'use strict';
const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const os = require('node:os');
const path = require('node:path');
const { spawnSync } = require('node:child_process');
const { assess } = require('./maintainability-parity.cjs');
const spec = require('./maintainability-capabilities.json');
const pristine = JSON.parse(JSON.stringify(spec.capabilities));
const restoreSpec = () => spec.capabilities.forEach((capability, i) => { capability.pyguard = JSON.parse(JSON.stringify(pristine[i].pyguard)); });
const evidence = () => ({ schema_version:'1', suite:'maintainability.installed', evaluator_sha256:'native-evidence', cases:spec.required_tsguard_cases.map(id => ({id,outcome:'pass'})) });
const declared = () => spec.capabilities.flatMap(capability => Object.entries(capability.pyguard.cases || {})
  .flatMap(([kind, ids]) => ids.map(id => ({ id, capability: capability.id, kind }))));
const pythonEvidence = mutate => {
  const report = { schema_version:'1', suite:'maintainability.python', evaluator_sha256:'python-evidence',
    cases: declared().map(d => ({ id:d.id, capability:d.capability, kind:d.kind, outcome:'pass', duration_ms:1 })) };
  if (mutate) mutate(report);
  return report;
};
function cli(reports) {
  const dir = fs.mkdtempSync(path.join(os.tmpdir(), 'pyguard-mparity-test-'));
  try {
    const args = reports.map((report, i) => {
      const file = path.join(dir, `${i}.json`);
      fs.writeFileSync(file, JSON.stringify(report));
      return file;
    });
    return spawnSync(process.execPath, [require.resolve('./maintainability-parity.cjs'), ...args], { encoding:'utf8', timeout:60000 });
  } finally { fs.rmSync(dir, { recursive:true, force:true }); }
}
test('deferred Python capabilities do not block the selected Tsguard scope', () => {
  const result=assess(evidence());
  assert.equal(result.tsguard_capabilities_passed,true);
  assert.equal(result.maintainability_parity_ready,false);
  assert.equal(result.tsguard_scope_ready,true);
  assert.equal(result.pyguard_maintainability_ready,false);
  assert.deepEqual(Object.keys(result.levels),['code','structure','change']);
  assert.ok(Object.values(result.levels).every(level=>level.implemented&&level.native_evals_passed));
  assert.equal(Object.hasOwn(result,'release_ready'),false);
  assert.deepEqual(result.deferred_tsguard_capabilities,[]);
  // Back-compat without a Python report: only the spec slot is visible.
  assert.ok(result.capabilities.every(c=>c.pyguard===(c.id==='change.native_duplication'?'deferred':'not_evaluated')));
  assert.ok(result.capabilities.every(c=>Object.keys(c.pyguard_cases).length===0));
  assert.equal(result.pyguard_cases_passed,false);
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
test('the CLI keeps its single-report contract and exits on native failures', () => {
  const passing=cli([evidence()]);
  assert.equal(passing.status,0,passing.stderr||passing.stdout);
  assert.equal(JSON.parse(passing.stdout).pyguard_maintainability_ready,false);
  const failing=evidence();failing.cases.find(c=>c.id==='baseline.single_linter').outcome='fail';
  assert.equal(cli([failing]).status,1);
});
test('a passing Python report is accepted as not_implemented without claiming readiness', () => {
  const result=assess(evidence(),pythonEvidence());
  const seeded=result.capabilities.find(c=>c.id==='structure.function_graph');
  assert.equal(seeded.pyguard,'not_implemented');
  assert.deepEqual(seeded.pyguard_cases,Object.fromEntries(declared().filter(d=>d.capability==='structure.function_graph').map(d=>[d.id,'pass'])));
  assert.equal(result.capabilities.find(c=>c.id==='change.native_duplication').pyguard,'deferred');
  for (const row of result.capabilities) assert.equal(Object.keys(row.pyguard_cases).length,declared().filter(d=>d.capability===row.id).length);
  assert.equal(result.pyguard_cases_passed,true);
  assert.equal(result.pyguard_maintainability_ready,false);
  assert.equal(result.maintainability_parity_ready,false);
  assert.equal(cli([evidence(),pythonEvidence()]).status,0);
});
test('a failing Python seed case is not ready and fails the process', () => {
  const pyguard=pythonEvidence(r=>{ r.cases.find(c=>c.kind==='boundary').outcome='fail'; });
  const result=assess(evidence(),pyguard);
  assert.equal(result.pyguard_cases_passed,false);
  assert.equal(result.pyguard_maintainability_ready,false);
  assert.equal(result.maintainability_parity_ready,false);
  const seeded=result.capabilities.find(c=>c.id==='structure.function_graph');
  assert.equal(seeded.pyguard,'not_implemented');
  assert.deepEqual(Object.values(seeded.pyguard_cases).filter(o=>o==='fail'),['fail']);
  assert.equal(cli([evidence(),pyguard]).status,1);
});
test('implemented capabilities are ready only when positive, negative and boundary cases all pass', () => {
  try {
    for (const capability of spec.capabilities) {
      if (capability.pyguard.status==='deferred') continue;
      capability.pyguard={ status:'implemented', cases:{ positive:[`${capability.id}.p`], negative:[`${capability.id}.n`], boundary:[`${capability.id}.b`] } };
    }
    const ready=assess(evidence(),pythonEvidence());
    assert.ok(ready.capabilities.every(c=>c.pyguard==='pass'||c.pyguard==='deferred'));
    assert.equal(ready.pyguard_maintainability_ready,true);
    assert.equal(ready.maintainability_parity_ready,true);

    const failing=assess(evidence(),pythonEvidence(r=>{ r.cases.find(c=>c.id==='structure.coupling.p').outcome='fail'; }));
    assert.equal(failing.capabilities.find(c=>c.id==='structure.coupling').pyguard,'fail');
    assert.equal(failing.pyguard_maintainability_ready,false);
    assert.equal(failing.maintainability_parity_ready,false);
    assert.equal(failing.pyguard_cases_passed,false);

    // An implemented capability missing one of the three case kinds cannot pass.
    const incomplete=spec.capabilities.find(c=>c.id==='structure.coupling');
    incomplete.pyguard.cases.boundary=[];
    try {
      const missing=assess(evidence(),pythonEvidence());
      assert.equal(missing.capabilities.find(c=>c.id==='structure.coupling').pyguard,'fail');
      assert.equal(missing.maintainability_parity_ready,false);
    } finally { restoreSpec(); }
  } finally { restoreSpec(); }
});
test('an implemented capability with a missing boundary property or an empty cases object fails without throwing', () => {
  try {
    for (const capability of spec.capabilities) {
      if (capability.pyguard.status==='deferred') continue;
      capability.pyguard={ status:'implemented', cases:{ positive:[`${capability.id}.p`], negative:[`${capability.id}.n`] } };
    }
    const missing=assess(evidence(),pythonEvidence());
    assert.ok(missing.capabilities.filter(c=>c.pyguard!=='deferred').every(c=>c.pyguard==='fail'));
    assert.equal(missing.pyguard_maintainability_ready,false);
    assert.equal(missing.maintainability_parity_ready,false);

    for (const capability of spec.capabilities) {
      if (capability.pyguard.status==='deferred') continue;
      capability.pyguard={ status:'implemented', cases:{} };
    }
    const empty=assess(evidence(),pythonEvidence());
    assert.ok(empty.capabilities.filter(c=>c.pyguard!=='deferred').every(c=>c.pyguard==='fail'));
    assert.equal(empty.pyguard_maintainability_ready,false);
    assert.equal(empty.maintainability_parity_ready,false);

    // Even a cases-less slot must not throw.
    for (const capability of spec.capabilities) {
      if (capability.pyguard.status==='deferred') continue;
      capability.pyguard={ status:'implemented' };
    }
    const absent=assess(evidence(),pythonEvidence());
    assert.ok(absent.capabilities.filter(c=>c.pyguard!=='deferred').every(c=>c.pyguard==='fail'));
  } finally { restoreSpec(); }
});
test('invalid, mismatched or incomplete Python reports are rejected', () => {
  const mutations = [
    r=>{ r.suite='pyguard.parity'; },
    r=>{ r.schema_version='2'; },
    r=>{ delete r.evaluator_sha256; },
    r=>{ r.cases.push({ ...r.cases[0] }); },
    r=>{ r.cases[0].outcome='unsupported'; },
    r=>{ r.cases.pop(); },
    r=>{ r.cases.push({ id:'criticality.unplanned', capability:'structure.function_graph', kind:'negative', outcome:'pass' }); },
    r=>{ r.cases[0].kind='boundary'; },
    r=>{ r.cases[0].capability='structure.coupling'; },
    r=>{ delete r.cases; },
  ];
  for (const mutate of mutations) {
    assert.throws(()=>assess(evidence(),pythonEvidence(mutate)),/Unsupported|Duplicate|Unexpected|Misdeclared|Incomplete|Invalid/);
  }
  assert.doesNotThrow(()=>assess(evidence(),pythonEvidence()));
});
