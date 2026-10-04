'use strict';
const fs = require('node:fs');
const path = require('node:path');
const { isDeepStrictEqual } = require('node:util');
function compare(baseline, candidate) {
  for (const report of [baseline, candidate]) {
    if (report.schema_version !== '1' || !report.corpus_sha256 || !report.evaluator_sha256 || !Array.isArray(report.cases) || report.cases.length === 0) throw new Error('Incomparable report: missing or unsupported metadata');
  }
  for (const key of ['schema_version', 'corpus_sha256', 'evaluator_sha256']) if (baseline[key] !== candidate[key]) throw new Error(`Incomparable reports: ${key} differs`);
  for (const key of ['platform', 'arch', 'node']) if (baseline.environment[key] !== candidate.environment[key]) throw new Error(`Incomparable environments: ${key} differs`);
  if (!baseline.toolchain || !candidate.toolchain || !isDeepStrictEqual(baseline.toolchain, candidate.toolchain)) throw new Error('Incomparable toolchains');
  const previous = new Map(baseline.cases.map(c => [c.id, c]));
  if (previous.size !== baseline.cases.length || new Set(candidate.cases.map(c => c.id)).size !== candidate.cases.length || previous.size !== candidate.cases.length || candidate.cases.some(c => !previous.has(c.id))) throw new Error('Incomparable case sets');
  const cases = candidate.cases.map(current => {
    const old = previous.get(current.id);
    if (!['pass', 'fail', 'unsupported', 'skipped'].includes(old.outcome) || !['pass', 'fail', 'unsupported', 'skipped'].includes(current.outcome)) throw new Error('Incomparable outcome values');
    const change = (old.outcome === 'skipped') !== (current.outcome === 'skipped') ? 'applicability_changed' : old.outcome === 'pass' && current.outcome !== 'pass' ? 'regressed'
      : old.outcome !== 'pass' && current.outcome === 'pass' ? 'gained'
      : current.outcome === 'pass' ? 'retained' : 'unresolved';
    return { id: current.id, baseline: old.outcome, candidate: current.outcome, change,
      duration_delta_ms: current.duration_ms - old.duration_ms, stdout_delta_bytes: current.stdout_bytes - old.stdout_bytes };
  });
  return { schema_version: '1', corpus_sha256: candidate.corpus_sha256, baseline: baseline.revision, candidate: candidate.revision,
    totals: Object.fromEntries(['gained', 'regressed', 'retained', 'unresolved', 'applicability_changed'].map(change => [change, cases.filter(c => c.change === change).length])),
    candidate_ready: candidate.cases.every(c => c.outcome === 'pass' || c.outcome === 'skipped') && !cases.some(c => c.change === 'regressed' || (c.baseline === 'pass' && c.candidate === 'skipped')), cases };
}
function markdown(report) {
  return `# OxGuard capability comparison\n\nBaseline: ${report.baseline}\nCandidate: ${report.candidate}\n\nRequired candidate cases passed: ${report.candidate_ready}\n\n| Case | Baseline | Candidate | Change | Runtime delta (ms) | Output delta (bytes) |\n|---|---|---|---|---:|---:|\n` + report.cases.map(c => `| ${c.id} | ${c.baseline} | ${c.candidate} | ${c.change} | ${c.duration_delta_ms} | ${c.stdout_delta_bytes} |`).join('\n') + '\n';
}
if (require.main === module) {
  const [before, after, output] = process.argv.slice(2);
  if (!before || !after || !output || process.argv.length !== 5) throw new Error('Usage: node evals/compare.cjs baseline.json candidate.json comparison.json');
  const result = compare(JSON.parse(fs.readFileSync(before)), JSON.parse(fs.readFileSync(after)));
  fs.mkdirSync(path.dirname(output), { recursive: true });
  fs.writeFileSync(output, JSON.stringify(result, null, 2) + '\n');
  fs.writeFileSync(output.replace(/\.json$/, '') + '.md', markdown(result));
  console.log(JSON.stringify(result.totals));
  process.exitCode = result.candidate_ready ? 0 : 1;
}
module.exports = { compare, markdown };
