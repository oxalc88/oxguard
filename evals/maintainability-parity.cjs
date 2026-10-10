'use strict';
const fs = require('node:fs');
const spec = require('./maintainability-capabilities.json');
function assess(report) {
  if (report.schema_version !== '1' || report.suite !== 'maintainability.installed' || !report.evaluator_sha256 || !Array.isArray(report.cases) || new Set(report.cases.map(c => c.id)).size !== report.cases.length) throw new Error('Invalid installed maintainability evidence');
  const expected = [...spec.required_tsguard_cases].sort();
  if (JSON.stringify(report.cases.map(c => c.id).sort()) !== JSON.stringify(expected)) throw new Error('Incomplete or unexpected Tsguard maintainability case set');
  if (report.cases.some(c => !['pass','fail'].includes(c.outcome))) throw new Error('Unsupported native case cannot establish Tsguard readiness');
  const rows = spec.capabilities.map(capability => {
    const native = report.cases.find(c => c.id === capability.tsguard);
    if (!native || !['pass','fail'].includes(native.outcome)) throw new Error(`Missing native capability ${capability.tsguard}`);
    return { id: capability.id, tsguard: native.outcome, pyguard: 'not_evaluated', python_candidate: capability.python_candidate };
  });
  const tsguardPassed = rows.every(r => r.tsguard === 'pass') && report.cases.every(c => c.outcome === 'pass');
  const levels = Object.fromEntries(Object.entries(spec.levels).map(([level, ids])=>[level,{
    implemented:true, evaluated_cases:ids.length, native_evals_passed:ids.every(id=>report.cases.find(c=>c.id===id)?.outcome==='pass'),
  }]));
  return { schema_version: '1', scope: spec.scope, tsguard_capabilities_passed: tsguardPassed,
    tsguard_scope_ready: tsguardPassed, pyguard_maintainability_ready: false, maintainability_parity_ready: false,
    levels, deferred_tsguard_capabilities: spec.deferred_tsguard_capabilities, capabilities: rows,
    limitation: 'The planned Tsguard code, structure and change capabilities have native evaluations. New Python maintainability capabilities are deferred and do not block that scope. Provider limits and static-analysis boundaries remain explicit. This report does not replace retained Level 1 checks, platform verification or release authorization.' };
}
if (require.main === module) {
  const report = assess(JSON.parse(fs.readFileSync(process.argv[2])));
  console.log(JSON.stringify(report, null, 2));
  process.exitCode = report.tsguard_capabilities_passed ? 0 : 1;
}
module.exports = { assess };
