'use strict';
const { test } = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const os = require('node:os');
const path = require('node:path');
const crypto = require('node:crypto');
const { spawnSync } = require('node:child_process');
const { evaluate, definitions } = require('./maintainability.cjs');
const spec = require('../maintainability-capabilities.json');

const harness = require.resolve('./maintainability.cjs');
const declared = () => spec.capabilities.flatMap(capability => Object.entries(capability.pyguard.cases)
  .flatMap(([kind, ids]) => ids.map(id => ({ id, capability: capability.id, kind }))));

function temporary(t) {
  const dir = fs.mkdtempSync(path.join(os.tmpdir(), 'pyguard-maint-test-'));
  t.after(() => fs.rmSync(dir, { recursive: true, force: true }));
  return dir;
}

// Canned normalized results keyed by the fixture project the stand-in runs in
// (the project directory is named after the case id). Answers mirror the
// known answers pinned in the harness; the real-binary test verifies them
// independently when PYGUARD_EVAL_BINARY is provided.
const ANSWERS = {
  'criticality.distinct_callers_cross_module': [['src/core.py', 'core.target', 3, 1], ['src/caller_one.py', 'caller_one.one', 0, 4]],
  'criticality.uncalled_function_zero': [['src/lonely.py', 'lonely.never_called', 0, 1]],
  'criticality.same_caller_not_inflated': [['src/hub.py', 'hub.hub', 1, 1], ['src/busy.py', 'busy.busy', 0, 4]],
  'criticality.self_recursion_counts_once': [['src/recursion.py', 'recursion.countdown', 1, 1]],
};

// Score-verified against complexipy 8.3.0 by the real-binary test: 0 flat,
// 21 chain-of-six, 36 chain-of-eight, 20 two-chains-of-four, 21 in boundary.
const COMPLEXIPI = {
  'complexipy.cognitive.exceeded_is_advisory_warning': [['src/complexity.py', 'flat', 0], ['src/complexity.py', 'above', 21]],
  'complexipy.cognitive.flat_function_no_finding': [['src/flat.py', 'flat', 0]],
  'complexipy.cognitive.nested_never_blocking': [['src/nested.py', 'deeply_nested', 36]],
  'complexipy.cognitive.test_files_excluded': [['src/main.py', 'flat', 0]],
  'complexipy.cognitive.nested_function_folds_into_outer': [['src/nested_function.py', 'outer', 36]],
  'complexipy.cognitive.threshold_boundary': [['src/boundary.py', 'boundary', 20], ['src/boundary.py', 'above', 21]],
};

// Canned zero-config Ruff baseline results. Real-binary cases verify these
// independently when PYGUARD_EVAL_BINARY is set.
const RUFF = {
  'ruff.baseline.empty_handler_is_blocking': { exit: 1, status: 'fail', gate: 'failed', measurement: true, findings: [['S110', 'src/handlers.py', 4]] },
  'ruff.baseline.finally_jump_is_blocking': { exit: 1, status: 'fail', gate: 'failed', measurement: true, findings: [['B012', 'src/finally.py', 5]] },
  'ruff.baseline.logging_contrast_pass_is_blocking': { exit: 1, status: 'fail', gate: 'failed', measurement: true, findings: [['S110', 'src/handlers.py', 9]] },
  'ruff.baseline.logging_or_reraise_is_clean': { exit: 0, status: 'pass', gate: 'passed', measurement: true, findings: [] },
  'ruff.baseline.project_ignore_preserved': { exit: 0, status: 'pass', gate: 'passed', measurement: false, findings: [] },
  'ruff.baseline.noqa_suppression_respected': { exit: 0, status: 'pass', gate: 'passed', measurement: true, findings: [] },
  'ruff.baseline.ellipsis_body_not_flagged': { exit: 0, status: 'pass', gate: 'passed', measurement: true, findings: [] },
  'ruff.baseline.typed_exception_handler_not_flagged': { exit: 0, status: 'pass', gate: 'passed', measurement: true, findings: [] },
  'ruff.baseline.test_files_still_linted': { exit: 1, status: 'fail', gate: 'failed', measurement: true, findings: [['S110', 'src/tests/test_handlers.py', 4]] },
};

// Canned structure reports for the Python module-graph controls. The real-binary
// test independently verifies the same measurements and findings with grimp.
const STRUCTURE = {
  'structure.runtime_cycle': { status: 'advisory', assessment: 'complete', gate: 'advisory', normalization: 'complete', exit: 0, counts: { modules: 3, edges: 2, cycles: 1, type_imports: 0, deferred_imports: 0 }, depths: { 'src/pkg/__init__.py': 0, 'src/pkg/a.py': 0, 'src/pkg/b.py': 0 }, cycles: [['src/pkg/a.py', 'src/pkg/b.py']] },
  'structure.utf8_bom_source_cycle': { status: 'advisory', assessment: 'complete', gate: 'advisory', normalization: 'complete', exit: 0, counts: { modules: 3, edges: 2, cycles: 1, type_imports: 0, deferred_imports: 0 }, depths: { 'src/pkg/__init__.py': 0, 'src/pkg/a.py': 0, 'src/pkg/b.py': 0 }, cycles: [['src/pkg/a.py', 'src/pkg/b.py']] },
  'structure.latin1_declared_source_cycle': { status: 'advisory', assessment: 'complete', gate: 'advisory', normalization: 'complete', exit: 0, counts: { modules: 3, edges: 2, cycles: 1, type_imports: 0, deferred_imports: 0 }, depths: { 'src/pkg/__init__.py': 0, 'src/pkg/a.py': 0, 'src/pkg/b.py': 0 }, cycles: [['src/pkg/a.py', 'src/pkg/b.py']] },
  'structure.self_import_cycle': { status: 'advisory', assessment: 'complete', gate: 'advisory', normalization: 'complete', exit: 0, counts: { modules: 2, edges: 1, cycles: 1, type_imports: 0, deferred_imports: 0 }, depths: { 'src/pkg/__init__.py': 0, 'src/pkg/a.py': 0 }, cycles: [['src/pkg/a.py']] },
  'structure.nested_package_cycle': { status: 'advisory', assessment: 'complete', gate: 'advisory', normalization: 'complete', exit: 0, counts: { modules: 4, edges: 2, cycles: 1, type_imports: 0, deferred_imports: 0 }, depths: { 'src/pkg/__init__.py': 0, 'src/pkg/sub/__init__.py': 0, 'src/pkg/sub/a.py': 0, 'src/pkg/b.py': 0 }, cycles: [['src/pkg/b.py', 'src/pkg/sub/a.py']] },
  'structure.namespace_package_cycle': { status: 'advisory', assessment: 'complete', gate: 'advisory', normalization: 'complete', exit: 0, counts: { modules: 2, edges: 2, cycles: 1, type_imports: 0, deferred_imports: 0 }, depths: { 'src/pkg/a.py': 0, 'src/pkg/b.py': 0 }, cycles: [['src/pkg/a.py', 'src/pkg/b.py']] },
  'structure.namespace_relative_import_cycle': { status: 'advisory', assessment: 'complete', gate: 'advisory', normalization: 'complete', exit: 0, counts: { modules: 2, edges: 2, cycles: 1, type_imports: 0, deferred_imports: 0 }, depths: { 'src/pkg/a.py': 0, 'src/pkg/b.py': 0 }, cycles: [['src/pkg/a.py', 'src/pkg/b.py']] },
  'structure.default_scope_src_layout_cycle': { status: 'advisory', assessment: 'complete', gate: 'advisory', normalization: 'complete', exit: 0, counts: { modules: 3, edges: 2, cycles: 1, type_imports: 0, deferred_imports: 0 }, depths: { 'src/pkg/__init__.py': 0, 'src/pkg/a.py': 0, 'src/pkg/b.py': 0 }, cycles: [['src/pkg/a.py', 'src/pkg/b.py']] },
  'structure.dunder_package_attribute_cycle': { status: 'advisory', assessment: 'complete', gate: 'advisory', normalization: 'complete', exit: 0, counts: { modules: 2, edges: 2, cycles: 1, type_imports: 0, deferred_imports: 0 }, depths: { 'src/pkg/__init__.py': 0, 'src/pkg/worker.py': 0 }, cycles: [['src/pkg/__init__.py', 'src/pkg/worker.py']] },
  'structure.runtime_binding_exports_control': { status: 'advisory', assessment: 'complete', gate: 'advisory', normalization: 'complete', exit: 0, counts: { modules: 2, edges: 2, cycles: 1, type_imports: 0, deferred_imports: 0 }, depths: { 'src/pkg/__init__.py': 0, 'src/pkg/worker.py': 0 }, cycles: [['src/pkg/__init__.py', 'src/pkg/worker.py']] },
  'structure.conditional_export_cycle': { status: 'advisory', assessment: 'complete', gate: 'advisory', normalization: 'complete', exit: 0, counts: { modules: 2, edges: 2, cycles: 1, type_imports: 0, deferred_imports: 0 }, depths: { 'src/pkg/__init__.py': 0, 'src/pkg/worker.py': 0 }, cycles: [['src/pkg/__init__.py', 'src/pkg/worker.py']] },
  'structure.acyclic_chain': { status: 'pass', assessment: 'complete', gate: 'passed', normalization: 'complete', exit: 0, counts: { modules: 4, edges: 2, cycles: 0, type_imports: 0, deferred_imports: 0 }, depths: { 'src/pkg/__init__.py': 0, 'src/pkg/a.py': 2, 'src/pkg/b.py': 1, 'src/pkg/c.py': 0 } },
  'structure.scope_and_test_exclusions': { status: 'pass', assessment: 'complete', gate: 'passed', normalization: 'complete', exit: 0, counts: { modules: 3, edges: 1, cycles: 0, type_imports: 0, deferred_imports: 0 }, depths: { 'src/pkg/__init__.py': 0, 'src/pkg/main.py': 1, 'src/pkg/helper.py': 0 } },
  'structure.duplicate_identities_do_not_crash': { status: 'advisory', assessment: 'incomplete', gate: 'advisory', normalization: 'partial', exit: 0, counts: { modules: 3, edges: 0, cycles: 0, type_imports: 0, deferred_imports: 0 }, depths: { 'src/one/a.py': 0, 'src/two/a.py': 0, 'src/pkg/a.py': 0 }, partial: 'Duplicate module identity: pkg.a (src/pkg/a/__init__.py, src/pkg/a.py)' },
  'structure.out_of_scope_collision_is_ignored': { status: 'pass', assessment: 'complete', gate: 'passed', normalization: 'complete', exit: 0, counts: { modules: 1, edges: 0, cycles: 0, type_imports: 0, deferred_imports: 0 }, depths: { 'src/other.py': 0 } },
  'structure.runtime_cycle_with_type_only_imports': { status: 'advisory', assessment: 'complete', gate: 'advisory', normalization: 'complete', exit: 0, counts: { modules: 4, edges: 2, cycles: 1, type_imports: 1, deferred_imports: 0 }, depths: { 'src/pkg/__init__.py': 0, 'src/pkg/a.py': 0, 'src/pkg/b.py': 0, 'src/pkg/c.py': 0 }, cycles: [['src/pkg/a.py', 'src/pkg/b.py']] },
  'structure.type_only_cycle_control': { status: 'pass', assessment: 'complete', gate: 'passed', normalization: 'complete', exit: 0, counts: { modules: 3, edges: 0, cycles: 0, type_imports: 2, deferred_imports: 0 }, depths: { 'src/pkg/__init__.py': 0, 'src/pkg/a.py': 0, 'src/pkg/b.py': 0 } },
  'structure.external_imports_no_edges': { status: 'pass', assessment: 'complete', gate: 'passed', normalization: 'complete', exit: 0, counts: { modules: 3, edges: 0, cycles: 0, type_imports: 0, deferred_imports: 0 }, depths: { 'src/pkg/__init__.py': 0, 'src/pkg/a.py': 0, 'src/pkg/b.py': 0 } },
  'structure.external_name_collision_no_cycle': { status: 'pass', assessment: 'complete', gate: 'passed', normalization: 'complete', exit: 0, counts: { modules: 2, edges: 1, cycles: 0, type_imports: 0, deferred_imports: 0 }, depths: { 'src/pkg/a.py': 0, 'src/vendor/json.py': 0 } },
  'structure.relative_import_package_export_control': { status: 'pass', assessment: 'complete', gate: 'passed', normalization: 'complete', exit: 0, counts: { modules: 2, edges: 1, cycles: 0, type_imports: 0, deferred_imports: 0 }, depths: { 'src/pkg/__init__.py': 0, 'src/pkg/a.py': 1 } },
  'structure.module_getattr_export_control': { status: 'pass', assessment: 'complete', gate: 'passed', normalization: 'complete', exit: 0, counts: { modules: 2, edges: 1, cycles: 0, type_imports: 0, deferred_imports: 0 }, depths: { 'src/pkg/__init__.py': 0, 'src/pkg/worker.py': 1 } },
  'structure.star_reexport_control': { status: 'pass', assessment: 'complete', gate: 'passed', normalization: 'complete', exit: 0, counts: { modules: 3, edges: 2, cycles: 0, type_imports: 0, deferred_imports: 0 }, depths: { 'src/pkg/__init__.py': 1, 'src/pkg/impl.py': 0, 'src/pkg/worker.py': 2 } },
  'structure.dynamic_module_exports_control': { status: 'pass', assessment: 'complete', gate: 'passed', normalization: 'complete', exit: 0, counts: { modules: 2, edges: 1, cycles: 0, type_imports: 0, deferred_imports: 0 }, depths: { 'src/pkg/__init__.py': 0, 'src/pkg/worker.py': 1 } },
  'structure.deferred_import_cycle_control': { status: 'pass', assessment: 'complete', gate: 'passed', normalization: 'complete', exit: 0, counts: { modules: 3, edges: 1, cycles: 0, type_imports: 0, deferred_imports: 1 }, depths: { 'src/pkg/__init__.py': 0, 'src/pkg/a.py': 1, 'src/pkg/b.py': 0 } },
  'structure.unresolved_local_import_is_partial': { status: 'advisory', assessment: 'incomplete', gate: 'advisory', normalization: 'partial', exit: 0, counts: { modules: 2, edges: 0, cycles: 0, type_imports: 0, deferred_imports: 0 }, depths: { 'src/pkg/__init__.py': 0, 'src/pkg/a.py': 0 }, partial: 'Unresolved local import: pkg.missing (src/pkg/a.py:1)' },
  'structure.excluded_import_is_scope_boundary': { status: 'pass', assessment: 'complete', gate: 'passed', normalization: 'complete', exit: 0, counts: { modules: 2, edges: 1, cycles: 0, type_imports: 0, deferred_imports: 0 }, depths: { 'src/pkg/__init__.py': 1, 'src/pkg/a.py': 0 } },
  'structure.relative_import_missing_name_is_partial': { status: 'advisory', assessment: 'incomplete', gate: 'advisory', normalization: 'partial', exit: 0, counts: { modules: 2, edges: 0, cycles: 0, type_imports: 0, deferred_imports: 0 }, depths: { 'src/pkg/__init__.py': 0, 'src/pkg/a.py': 0 }, partial: 'Unresolved local import: pkg.missing (src/pkg/a.py:1)' },
  'structure.narrow_scope_import_is_boundary': { status: 'pass', assessment: 'complete', gate: 'passed', normalization: 'complete', exit: 0, counts: { modules: 1, edges: 0, cycles: 0, type_imports: 0, deferred_imports: 0 }, depths: { 'src/pkg/a.py': 0 } },
  'structure.narrow_scope_missing_child_is_partial': { status: 'advisory', assessment: 'incomplete', gate: 'advisory', normalization: 'partial', exit: 0, counts: { modules: 1, edges: 0, cycles: 0, type_imports: 0, deferred_imports: 0 }, depths: { 'src/pkg/a.py': 0 }, partial: 'Unresolved local import: pkg.missing (src/pkg/a.py:1)' },
  'structure.invalid_relative_import_is_partial': { status: 'advisory', assessment: 'incomplete', gate: 'advisory', normalization: 'partial', exit: 0, counts: { modules: 2, edges: 0, cycles: 0, type_imports: 0, deferred_imports: 0 }, depths: { 'src/pkg/__init__.py': 0, 'src/pkg/a.py': 0 }, partial: 'Unresolved local import: <relative> (src/pkg/a.py:1)' },
  'structure.type_only_package_export_is_partial': { status: 'advisory', assessment: 'incomplete', gate: 'advisory', normalization: 'partial', exit: 0, counts: { modules: 3, edges: 1, cycles: 0, type_imports: 1, deferred_imports: 0 }, depths: { 'src/pkg/__init__.py': 1, 'src/pkg/a.py': 0, 'src/pkg/b.py': 0 }, partial: 'Unresolved local import: pkg.Foo (src/pkg/b.py:1)' },
  'structure.annotation_only_export_is_partial': { status: 'advisory', assessment: 'incomplete', gate: 'advisory', normalization: 'partial', exit: 0, counts: { modules: 2, edges: 1, cycles: 0, type_imports: 0, deferred_imports: 0 }, depths: { 'src/pkg/__init__.py': 1, 'src/pkg/worker.py': 0 }, partial: 'Unresolved local import: pkg.client (src/pkg/worker.py:1)' },
  'structure.coupling_hub_threshold': { status: 'advisory', assessment: 'complete', gate: 'advisory', normalization: 'complete', exit: 0, counts: { modules: 13, edges: 11, cycles: 0, type_imports: 0, deferred_imports: 0 }, fan_in: { 'src/pkg/hub.py': 3, 'src/pkg/caller0.py': 0, 'src/pkg/leaf0.py': 1 }, fan_out: { 'src/pkg/hub.py': 8, 'src/pkg/caller0.py': 1, 'src/pkg/leaf0.py': 0 }, coupling: ['src/pkg/hub.py'] },
  'structure.coupling_human_warning': { status: 'advisory', assessment: 'complete', gate: 'advisory', normalization: 'complete', exit: 0, human: true, counts: { modules: 15, edges: 13, cycles: 1, type_imports: 0, deferred_imports: 0 }, fan_in: { 'src/pkg/hub.py': 3 }, fan_out: { 'src/pkg/hub.py': 8 }, coupling: ['src/pkg/hub.py'], cycles: [['src/pkg/a.py', 'src/pkg/b.py']] },
  'structure.coupling_below_importer_threshold': { status: 'pass', assessment: 'complete', gate: 'passed', normalization: 'complete', exit: 0, counts: { modules: 12, edges: 10, cycles: 0, type_imports: 0, deferred_imports: 0 }, fan_in: { 'src/pkg/hub.py': 2 }, fan_out: { 'src/pkg/hub.py': 8 } },
  'structure.coupling_below_dependency_threshold': { status: 'pass', assessment: 'complete', gate: 'passed', normalization: 'complete', exit: 0, counts: { modules: 12, edges: 10, cycles: 0, type_imports: 0, deferred_imports: 0 }, fan_in: { 'src/pkg/hub.py': 3 }, fan_out: { 'src/pkg/hub.py': 7 } },
  'structure.coupling_type_only_dependency_control': { status: 'pass', assessment: 'complete', gate: 'passed', normalization: 'complete', exit: 0, counts: { modules: 13, edges: 10, cycles: 0, type_imports: 1, deferred_imports: 0 }, fan_in: { 'src/pkg/hub.py': 3 }, fan_out: { 'src/pkg/hub.py': 7 } },
  'structure.coupling_deferred_dependency_control': { status: 'pass', assessment: 'complete', gate: 'passed', normalization: 'complete', exit: 0, counts: { modules: 13, edges: 10, cycles: 0, type_imports: 0, deferred_imports: 1 }, fan_in: { 'src/pkg/hub.py': 3 }, fan_out: { 'src/pkg/hub.py': 7 } },
  'structure.coupling_external_dependency_control': { status: 'pass', assessment: 'complete', gate: 'passed', normalization: 'complete', exit: 0, counts: { modules: 12, edges: 10, cycles: 0, type_imports: 0, deferred_imports: 0 }, fan_in: { 'src/pkg/hub.py': 3 }, fan_out: { 'src/pkg/hub.py': 7 } },
  'structure.coupling_excluded_modules_boundary': { status: 'pass', assessment: 'complete', gate: 'passed', normalization: 'complete', exit: 0, counts: { modules: 11, edges: 9, cycles: 0, type_imports: 0, deferred_imports: 0 }, fan_in: { 'src/pkg/hub.py': 2 }, fan_out: { 'src/pkg/hub.py': 7 } },
  'structure.coupling_test_file_boundary': { status: 'pass', assessment: 'complete', gate: 'passed', normalization: 'complete', exit: 0, counts: { modules: 11, edges: 9, cycles: 0, type_imports: 0, deferred_imports: 0 }, fan_in: { 'src/pkg/hub.py': 2 }, fan_out: { 'src/pkg/hub.py': 7 } },
};

function standInBinary(t) {
  const file = path.join(temporary(t), 'pyguard-stand-in.cjs');
  fs.writeFileSync(file, `'use strict';
const path = require('node:path');
const answers = ${JSON.stringify(ANSWERS)};
const cognitive = ${JSON.stringify(COMPLEXIPI)};
const ruff = ${JSON.stringify(RUFF)};
const structure = ${JSON.stringify(STRUCTURE)};
const caseId = path.basename(path.dirname(process.cwd()));
const variant = path.basename(process.cwd());
if (ruff[caseId]) {
  const spec = ruff[caseId];
  const findings = spec.findings.map(([rule, file, line]) => ({ rule, level: 'code', gate: 'ruff', severity: 'error', status: 'blocking', category: 'quality', location: { file, line, column: 5 }, evidence: rule + ' violation', diagnostics: [] }));
  const measurements = spec.measurement ? [{ metric: 'ruff.baseline_version', level: 'code', location: { file: '.' }, value: 1, unit: 'version' }] : [];
  process.stdout.write(JSON.stringify({ schema_version: '1', status: spec.status, assessment: 'complete', gates: [{ name: 'ruff', status: spec.gate, normalization: 'complete' }], exit_code: spec.exit, findings, measurements, artifacts: [], diagnostics: [] }));
  process.exit(spec.exit);
}
if (structure[caseId]) {
  const spec = structure[caseId];
  if (spec.human) {
    const lines = ['  [OK]   structure'];
    for (const file of spec.coupling || []) lines.push('  [ADVISORY] pyguard.structure.HIGH_MODULE_COUPLING ' + file + ' — ' + (spec.fan_in?.[file] ?? 0) + ' importing modules depend on this module, which imports ' + (spec.fan_out?.[file] ?? 0) + ' in-scope runtime modules.');
    for (const files of spec.cycles || []) lines.push('  [ADVISORY] pyguard.structure.CIRCULAR_DEPENDENCY ' + files[0] + ' — Runtime imports form a strongly connected component: ' + files.join(', '));
    process.stdout.write(lines.join('\\n') + '\\n');
    process.exit(spec.exit);
  }
  const measurements = Object.entries(spec.depths || {}).map(([file, value]) => ({ metric: 'module.dependency_depth', level: 'structure', location: { file }, value, unit: 'edges' }));
  for (const [metric, values] of [['module.fan_in', spec.fan_in], ['module.fan_out', spec.fan_out]]) for (const [file, value] of Object.entries(values || {})) measurements.push({ metric, level: 'structure', location: { file }, value, unit: 'modules' });
  const units = { modules: 'modules', edges: 'edges', cycles: 'components', type_imports: 'references', deferred_imports: 'references' };
  for (const [name, value] of Object.entries(spec.counts)) measurements.push({ metric: name === 'modules' ? 'module.count' : 'module.' + name, level: 'structure', location: { file: '.' }, value, unit: units[name] });
  const findings = (spec.cycles || []).map(files => ({ level: 'structure', gate: 'structure', rule: 'pyguard.structure.CIRCULAR_DEPENDENCY', severity: 'warning', status: 'advisory', category: 'quality', location: { file: files[0] }, related: files.slice(1).map(file => ({ file })), evidence: 'Runtime imports form a strongly connected component: ' + files.join(', '), diagnostics: [] }));
  for (const file of spec.coupling || []) findings.push({ level: 'structure', gate: 'structure', rule: 'pyguard.structure.HIGH_MODULE_COUPLING', severity: 'warning', status: 'advisory', category: 'quality', location: { file }, evidence: (spec.fan_in?.[file] ?? 0) + ' importing modules depend on this module, which imports ' + (spec.fan_out?.[file] ?? 0) + ' in-scope runtime modules.', remediation: 'Review the change surface and cohesion; these counts alone do not prove excessive coupling.', diagnostics: [] });
  if (spec.partial) findings.push({ level: 'structure', gate: 'structure', rule: 'pyguard.structure.not_evaluated', severity: 'info', status: 'advisory', category: 'quality', evidence: spec.partial, diagnostics: [] });
  process.stdout.write(JSON.stringify({ schema_version: '1', status: spec.status, assessment: spec.assessment, gates: [{ name: 'structure', status: spec.gate, normalization: spec.normalization }], exit_code: spec.exit, findings, measurements, artifacts: [], diagnostics: [] }));
  process.exit(spec.exit);
}
if (variant === 'baseline' || cognitive[caseId]) {
  const rows = variant === 'baseline' ? [] : cognitive[caseId];
  const measurements = rows.map(([file, symbol, value]) => ({ metric: 'complexipy.cognitive', level: 'code', location: { file, symbol }, value, unit: 'score', threshold: 20 }));
  const findings = rows.filter(([, , value]) => value > 20).map(([file, symbol, value]) => ({ level: 'code', gate: 'radon', rule: 'pyguard.complexipy.cognitive_exceeded', severity: 'warning', status: 'advisory', category: 'quality', location: { file, symbol }, observed: value, threshold: 20, evidence: 'Cognitive complexity exceeds 20.', diagnostics: [] }));
  const status = findings.length ? 'advisory' : 'pass';
  const gateStatus = findings.length ? 'advisory' : 'passed';
  process.stdout.write(JSON.stringify({ schema_version: '1', status, assessment: 'complete', gates: [{ name: 'radon', status: gateStatus, normalization: 'complete' }], exit_code: 0, findings, measurements, artifacts: [], diagnostics: [] }));
  process.exit(0);
}
const rows = answers[caseId];
if (!rows) { process.stdout.write('unexpected project ' + caseId); process.exit(0); }
const wrong = process.env.PYGUARD_EVAL_MOCK_WRONG_CASE;
const measurements = rows.map(([file, symbol, value, line], index) => ({ metric: 'criticality.in_degree', level: 'structure', location: { file, line, symbol }, value: wrong === caseId && index === 0 ? value + 1 : value, unit: 'callers' }));
const findings = measurements.filter(m => m.value > 0).map(m => ({ rule: 'pyguard.criticality.ranked', level: 'structure', severity: 'info', location: m.location, observed: m.value, evidence: m.location.symbol }));
const status = findings.length ? 'advisory' : 'passed';
process.stdout.write(JSON.stringify({ schema_version: '1', status, assessment: 'complete', gates: [{ name: 'criticality', status }], exit_code: 0, findings, measurements, artifacts: [], diagnostics: [] }));
`);
  return file;
}

function standInInvoke(binary, env = {}) {
  return ({ project }) => {
    const response = spawnSync(process.execPath, [binary], { cwd: project, encoding: 'utf8', env: { ...process.env, ...env }, timeout: 30000 });
    return { status: response.status, stdout: response.stdout || '', stderr: response.stderr || '', error: response.error && response.error.message };
  };
}

function cli(args, { env = {}, cwd } = {}) {
  return spawnSync(process.execPath, [harness, ...args], { encoding: 'utf8', env: { ...process.env, ...env }, cwd, timeout: 120000 });
}

test('harness cases match the capability spec exactly', () => {
  const expected = declared();
  assert.equal(new Set(expected.map(d => d.id)).size, expected.length);
  assert.deepEqual(definitions.map(d => d.id), expected.map(d => d.id));
  definitions.forEach((definition, i) => {
    assert.equal(definition.capability, expected[i].capability);
    assert.equal(definition.kind, expected[i].kind);
    assert.ok(spec.capabilities.some(capability => capability.id === definition.capability));
  });
  assert.deepEqual([...new Set(definitions.map(d => d.capability))], ['quality.zero_config_exception_handling', 'structure.runtime_graph', 'structure.false_positive_controls', 'structure.function_graph', 'structure.coupling', 'quality.cognitive_complexity']);
  for (const kind of ['positive', 'negative', 'boundary']) assert.ok(expected.some(d => d.kind === kind));
});

test('the report runs every declared case exactly once and passes with a faithful stand-in', t => {
  const binary = standInBinary(t);
  const report = evaluate({ binary, python: process.execPath, root: temporary(t), invoke: standInInvoke(binary) });
  assert.equal(report.schema_version, '1');
  assert.equal(report.suite, 'maintainability.python');
  assert.equal(report.evaluator_sha256, crypto.createHash('sha256').update(fs.readFileSync(harness)).digest('hex'));
  assert.deepEqual(report.cases.map(c => c.id).toSorted(), declared().map(d => d.id).toSorted());
  assert.equal(new Set(report.cases.map(c => c.id)).size, report.cases.length);
  assert.equal(report.cases.length, definitions.length);
  assert.ok(report.cases.every(c => c.outcome === 'pass'), JSON.stringify(report.cases, null, 2));
  for (const entry of report.cases) {
    const expected = declared().find(d => d.id === entry.id);
    assert.equal(entry.capability, expected.capability);
    assert.equal(entry.kind, expected.kind);
    assert.ok(Number.isFinite(entry.duration_ms) && entry.duration_ms >= 0);
  }
});

test('a failing assertion is recorded as a failing case and the run continues', t => {
  const binary = standInBinary(t);
  const report = evaluate({ binary, python: process.execPath, root: temporary(t), invoke: standInInvoke(binary, { PYGUARD_EVAL_MOCK_WRONG_CASE: 'criticality.uncalled_function_zero' }) });
  assert.equal(report.cases.length, definitions.length);
  assert.deepEqual(report.cases.filter(c => c.outcome === 'fail').map(c => c.id), ['criticality.uncalled_function_zero']);
  const failure = report.cases.find(c => c.outcome === 'fail');
  assert.match(failure.error, /criticality\.in_degree/);
  assert.ok(report.cases.filter(c => c.id !== failure.id).every(c => c.outcome === 'pass'));
});

test('a binary that cannot start fails every case instead of aborting the run', t => {
  const root = temporary(t);
  const report = evaluate({ binary: path.join(root, 'missing-pyguard'), python: process.execPath, root });
  assert.equal(report.cases.length, definitions.length);
  assert.ok(report.cases.every(c => c.outcome === 'fail'));
  assert.ok(report.cases.every(c => /could not be started/.test(c.error)));
});

test('the CLI writes the report and exits 0 on passes, 1 on a failing case', t => {
  const binary = standInBinary(t);
  const root = temporary(t);
  const passing = path.join(root, 'passing.json');
  const response = cli(['--binary', binary, '--python', process.execPath, '--output', passing]);
  assert.equal(response.status, 0, response.stderr || response.stdout);
  const report = JSON.parse(fs.readFileSync(passing, 'utf8'));
  assert.equal(report.suite, 'maintainability.python');
  assert.ok(report.cases.every(c => c.outcome === 'pass'));
  const failing = path.join(root, 'failing.json');
  const failed = cli(['--binary', binary, '--python', process.execPath, '--output', failing], { env: { PYGUARD_EVAL_MOCK_WRONG_CASE: 'criticality.self_recursion_counts_once' } });
  assert.equal(failed.status, 1, failed.stderr || failed.stdout);
  const failedReport = JSON.parse(fs.readFileSync(failing, 'utf8'));
  assert.deepEqual(failedReport.cases.filter(c => c.outcome === 'fail').map(c => c.id), ['criticality.self_recursion_counts_once']);
});

test('the CLI resolves a relative --binary path against the invocation cwd', t => {
  const binary = standInBinary(t);
  const root = temporary(t);
  const passing = path.join(root, 'relative.json');
  // The temp dir may live on a different drive than the checkout (e.g. C: vs
  // D: on Windows), so run the CLI from the stand-in's directory and pass a
  // sibling-relative path that is guaranteed to resolve against the invocation cwd.
  const relative = path.join('.', path.basename(binary));
  const response = cli(['--binary', relative, '--python', process.execPath, '--output', passing], { cwd: path.dirname(binary) });
  assert.equal(response.status, 0, response.stderr || response.stdout);
  const report = JSON.parse(fs.readFileSync(passing, 'utf8'));
  assert.ok(report.cases.every(c => c.outcome === 'pass'));
});

test('a built pyguard binary reproduces the known answers', { skip: !(process.env.PYGUARD_EVAL_BINARY && (process.env.PYGUARD_EVAL_PYTHON || process.env.EVAL_PY)) }, t => {
  const report = evaluate({ binary: process.env.PYGUARD_EVAL_BINARY, python: process.env.PYGUARD_EVAL_PYTHON || process.env.EVAL_PY, root: temporary(t) });
  assert.deepEqual(report.cases.filter(c => c.outcome !== 'pass'), [], JSON.stringify(report.cases, null, 2));
});
