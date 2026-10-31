'use strict';
const fs = require('node:fs');
const spec = require('./maintainability-capabilities.json');

// Every case id declared by the pyguard slot of the capability spec, with the
// capability and kind that declared it.
function declaredPyguardCases() {
  const declared = new Map();
  for (const capability of spec.capabilities) {
    for (const [kind, ids] of Object.entries(capability.pyguard.cases || {})) {
      for (const id of ids) {
        if (declared.has(id)) throw new Error(`Duplicate pyguard case ${id} in capability spec`);
        declared.set(id, { capability: capability.id, kind });
      }
    }
  }
  return declared;
}

function validatePyguard(report, declared) {
  if (report.schema_version !== '1' || report.suite !== 'maintainability.python' || !report.evaluator_sha256 || !Array.isArray(report.cases)) throw new Error('Invalid Python maintainability evidence');
  if (new Set(report.cases.map(c => c.id)).size !== report.cases.length) throw new Error('Duplicate Python maintainability case id');
  if (report.cases.some(c => !['pass', 'fail'].includes(c.outcome))) throw new Error('Unsupported Python case cannot establish readiness');
  for (const c of report.cases) {
    const expected = declared.get(c.id);
    if (!expected) throw new Error(`Unexpected Python maintainability case ${c.id}`);
    if (c.capability !== expected.capability || c.kind !== expected.kind) throw new Error(`Misdeclared Python maintainability case ${c.id}`);
  }
  const expected = [...declared.keys()].sort();
  if (JSON.stringify(report.cases.map(c => c.id).sort()) !== JSON.stringify(expected)) throw new Error('Incomplete or unexpected Python maintainability case set');
}

function capabilityPyguard(slot, capability, outcomes) {
  if (slot.status === 'deferred') return 'deferred';
  if (slot.status === 'planned') return 'not_implemented';
  if (slot.status === 'implemented') {
    // All three case kinds must be declared as non-empty arrays; a missing
    // property or a missing/empty cases object is a failure, never a throw.
    const kinds = ['positive', 'negative', 'boundary'];
    const cases = slot.cases && typeof slot.cases === 'object' ? slot.cases : undefined;
    if (!kinds.every(kind => Array.isArray(cases?.[kind]) && cases[kind].length > 0)) return 'fail';
    return kinds.flatMap(kind => cases[kind]).every(id => outcomes.get(id) === 'pass') ? 'pass' : 'fail';
  }
  throw new Error(`Unknown pyguard status ${slot.status} for ${capability}`);
}

// The optional second argument keeps single-report callers working: without a
// Python report nothing is evaluated, and pyguard values come from the spec
// slot ('deferred' only; everything else stays 'not_evaluated').
function assess(tsguardReport, pyguardReport, { strict = false } = {}) {
  const report = tsguardReport;
  if (report.schema_version !== '1' || report.suite !== 'maintainability.installed' || !report.evaluator_sha256 || !Array.isArray(report.cases) || new Set(report.cases.map(c => c.id)).size !== report.cases.length) throw new Error('Invalid installed maintainability evidence');
  const expected = [...spec.required_tsguard_cases].sort();
  if (JSON.stringify(report.cases.map(c => c.id).sort()) !== JSON.stringify(expected)) throw new Error('Incomplete or unexpected Tsguard maintainability case set');
  if (report.cases.some(c => !['pass', 'fail'].includes(c.outcome))) throw new Error('Unsupported native case cannot establish Tsguard readiness');
  const declared = declaredPyguardCases();
  if (pyguardReport) validatePyguard(pyguardReport, declared);
  const outcomes = new Map(pyguardReport ? pyguardReport.cases.map(c => [c.id, c.outcome]) : []);
  const rows = spec.capabilities.map(capability => {
    const native = report.cases.find(c => c.id === capability.tsguard);
    if (!native || !['pass', 'fail'].includes(native.outcome)) throw new Error(`Missing native capability ${capability.tsguard}`);
    const pyguardCases = {};
    if (pyguardReport) for (const ids of Object.values(capability.pyguard.cases || {})) for (const id of ids) pyguardCases[id] = outcomes.get(id);
    const pyguard = pyguardReport ? capabilityPyguard(capability.pyguard, capability.id, outcomes)
      : (capability.pyguard.status === 'deferred' ? 'deferred' : 'not_evaluated');
    return { id: capability.id, tsguard: native.outcome, pyguard, pyguard_cases: pyguardCases, python_candidate: capability.python_candidate };
  });
  const tsguardPassed = rows.every(r => r.tsguard === 'pass') && report.cases.every(c => c.outcome === 'pass');
  const pyguardReady = rows.every(r => r.pyguard === 'deferred' || r.pyguard === 'pass');
  const pyguardCasesPassed = Boolean(pyguardReport) && pyguardReport.cases.every(c => c.outcome === 'pass');
  const levels = Object.fromEntries(Object.entries(spec.levels).map(([level, ids])=>[level,{
    implemented:true, evaluated_cases:ids.length, native_evals_passed:ids.every(id=>report.cases.find(c=>c.id===id)?.outcome==='pass'),
  }]));
  const deferredPyguardCapabilities = rows.filter(r => r.pyguard === 'deferred').map(r => r.id);
  const result = { schema_version: '1', scope: spec.scope,
    readiness_scope: 'Ready means every supported Python capability is implemented and all declared evidence passes; deferred capabilities are excluded. This is not full Tsguard parity.',
    tsguard_capabilities_passed: tsguardPassed,
    tsguard_scope_ready: tsguardPassed, pyguard_maintainability_ready: pyguardReady, pyguard_cases_passed: pyguardCasesPassed,
    maintainability_parity_ready: tsguardPassed && pyguardReady,
    levels, deferred_tsguard_capabilities: spec.deferred_tsguard_capabilities,
    deferred_pyguard_capabilities: deferredPyguardCapabilities, capabilities: rows,
    limitation: 'The planned Tsguard code, structure and change capabilities have native evaluations. Python readiness covers the complete supported scope with deferred capabilities excluded; it is not full Tsguard parity. Provider limits and static-analysis boundaries remain explicit. This report does not replace retained Level 1 checks, platform verification or release authorization.' };
  if (strict && pyguardReport && !result.pyguard_maintainability_ready) throw new Error('Strict Python maintainability readiness failed: supported capabilities must be implemented and all declared cases must pass');
  return result;
}
if (require.main === module) {
  const strict = process.argv.includes('--strict');
  const files = process.argv.slice(2).filter(arg => arg !== '--strict');
  const [tsguardFile, pyguardFile] = files;
  if (!tsguardFile) throw new Error('Usage: node evals/maintainability-parity.cjs <tsguard.json> [<pyguard.json>]');
  const tsguard = JSON.parse(fs.readFileSync(tsguardFile, 'utf8'));
  const pyguard = pyguardFile ? JSON.parse(fs.readFileSync(pyguardFile, 'utf8')) : undefined;
  const result = assess(tsguard, pyguard, { strict });
  console.log(JSON.stringify(result, null, 2));
  // Default mode remains report-only for planned Python capabilities. Strict
  // mode is used by CI when a Python report is supplied.
  process.exitCode = result.tsguard_capabilities_passed && (!pyguard || result.pyguard_cases_passed) ? 0 : 1;
}
module.exports = { assess };
