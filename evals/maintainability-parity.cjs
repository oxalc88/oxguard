'use strict';
const fs = require('node:fs');
const spec = require('./maintainability-capabilities.json');
function assess(report) {
  if (report.schema_version !== '1' || report.suite !== 'maintainability.installed' || !report.evaluator_sha256 || !Array.isArray(report.cases) || new Set(report.cases.map(c => c.id)).size !== report.cases.length) throw new Error('Invalid installed maintainability evidence');
  const rows = spec.capabilities.map(capability => {
    const native = report.cases.find(c => c.id === capability.tsguard);
    if (!native || !['pass','fail'].includes(native.outcome)) throw new Error(`Missing native capability ${capability.tsguard}`);
    return { id: capability.id, tsguard: native.outcome, pyguard: 'not_evaluated', python_candidate: capability.python_candidate };
  });
  return { schema_version: '1', scope: spec.scope, tsguard_capabilities_passed: rows.every(r => r.tsguard === 'pass'), maintainability_parity_ready: false, release_ready: false, capabilities: rows,
    limitation: 'New Python maintainability capabilities are not implemented or evaluated. This report does not replace the retained Level 1 parity checks or platform release checks.' };
}
if (require.main === module) {
  const report = assess(JSON.parse(fs.readFileSync(process.argv[2])));
  console.log(JSON.stringify(report, null, 2));
  process.exitCode = report.tsguard_capabilities_passed ? 0 : 1;
}
module.exports = { assess };
