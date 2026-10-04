'use strict';
const fs = require('node:fs');
const path = require('node:path');
const crypto = require('node:crypto');
const spec = require('./capabilities.json');
function parity(tsguard, behavior, contract) {
  for (const report of [tsguard, behavior, contract]) {
    if (report.schema_version !== '1' || !report.corpus_sha256 || !report.evaluator_sha256 || !report.toolchain || !report.cases?.length || new Set(report.cases.map(c => c.id)).size !== report.cases.length) throw new Error('Invalid capability report');
    if (report.revision !== tsguard.revision) throw new Error('Parity requires reports from the same candidate revision');
    for (const key of ['platform', 'arch', 'node']) if (report.environment[key] !== tsguard.environment[key]) throw new Error('Parity requires the same environment');
    if (report.cases.some(c => !['pass', 'fail', 'unsupported', 'skipped'].includes(c.outcome))) throw new Error('Invalid capability outcome');
  }
  if (behavior.suite !== 'pyguard.behavior' || contract.suite !== 'pyguard.parity' || behavior.corpus_sha256 !== contract.corpus_sha256 || behavior.evaluator_sha256 !== contract.evaluator_sha256) throw new Error('Mismatched Python lanes');
  const find = (report, id) => {
    if (id === null) return 'not_evaluated';
    const c = report.cases.find(c => c.id === id);
    if (!c) throw new Error(`Missing capability case ${id}`);
    return c.outcome;
  };
  const rows = spec.capabilities.map(c => ({ id: c.id, tsguard: find(tsguard, c.tsguard), pyguard: find(c.lane === 'behavior' ? behavior : contract, c.pyguard), cases: { tsguard: c.tsguard, pyguard: c.pyguard }, equivalent: 'same quality intent; language-specific rules/metrics remain different' }));
  return { schema_version: '1', capability_map_sha256: crypto.createHash('sha256').update(fs.readFileSync(path.join(__dirname, 'capabilities.json'))).digest('hex'), revision: tsguard.revision, evidence: { tsguard: { corpus: tsguard.corpus_sha256, evaluator: tsguard.evaluator_sha256, toolchain: tsguard.toolchain }, pyguard: { corpus: behavior.corpus_sha256, evaluator: behavior.evaluator_sha256, toolchain: behavior.toolchain } }, shared_capabilities_ready: rows.every(c => c.tsguard === 'pass' && c.pyguard === 'pass'), pyguard_agent_ready: contract.cases.filter(c => c.id.startsWith('contract.') || c.id.startsWith('input.')).every(c => c.outcome === 'pass'), totals: { both_pass: rows.filter(c => c.tsguard === 'pass' && c.pyguard === 'pass').length, gaps: rows.filter(c => c.tsguard !== 'pass' || c.pyguard !== 'pass').length }, capabilities: rows };
}
function markdown(report) {
  return `# OxGuard capability parity\n\nRevision: ${report.revision}\nShared capabilities ready: ${report.shared_capabilities_ready}\nPyGuard agent ready: ${report.pyguard_agent_ready}\n\nThese are capability-level equivalents, not equal analyzer scores. Unsupported, failed, skipped and untested cases are not passes.\n\n| Capability | Tsguard | PyGuard |\n|---|---|---|\n` + report.capabilities.map(c => `| ${c.id} | ${c.tsguard} | ${c.pyguard} |`).join('\n') + '\n';
}
if (require.main === module) {
  const [ts, behavior, contract, output] = process.argv.slice(2);
  if (!output || process.argv.length !== 6) throw new Error('Usage: node evals/parity.cjs tsguard.json python-behavior.json python-parity.json output.json');
  const report = parity(...[ts, behavior, contract].map(file => JSON.parse(fs.readFileSync(file))));
  fs.mkdirSync(path.dirname(output), { recursive: true }); fs.writeFileSync(output, JSON.stringify(report, null, 2) + '\n'); fs.writeFileSync(output.replace(/\.json$/, '') + '.md', markdown(report));
  console.log(JSON.stringify({ shared_capabilities_ready: report.shared_capabilities_ready, pyguard_agent_ready: report.pyguard_agent_ready, totals: report.totals }));
  // A successful report is not a claim that missing capabilities exist.
}
module.exports = { parity, markdown };
