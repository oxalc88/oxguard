'use strict';

// Reuse existing known answers for platform smoke checks. These reports are
// intentionally not full capability or parity evidence.
const tsguard = Object.freeze([
  'execution.lock', 'paths.explicit_root_log', 'paths.symlink_root_log',
  'criticality.known_callers', 'coverage.functions.100', 'security.dependency_failure_still_sast',
]);
const python = Object.freeze([
  'execution.lock', 'execution.pipe_refusal', 'execution.missing_tool',
  'diagnostics.log', 'input.explicit_root', 'contract.json.multiple',
  'contract.json.lint', 'contract.json.complexity', 'contract.json.annotations',
  'criticality.cross_module_callers', 'security.bandit.blocking',
  'coverage.pytest9_config', 'coverage.test_failure',
  'dead_code.advisory', 'dependencies.advisory',
]);
const pythonMaintainability = Object.freeze([
  'ruff.baseline.empty_handler_is_blocking',
  'structure.namespace_relative_import_cycle',
  'structure.utf8_bom_source_cycle', 'structure.latin1_declared_source_cycle',
  'structure.scope_and_test_exclusions',
  'function_graph.encoded_source_preserves_import_root',
  'function_graph.declared_non_utf8_bytes_are_not_utf8',
  'change.nested_project_prefix', 'change.cognitive_suppression_is_partial',
]);

function selectCases(cases, ids) {
  if (ids === undefined) return cases;
  if (!Array.isArray(ids) || !ids.length || new Set(ids).size !== ids.length) {
    throw new Error('Platform smoke selection must contain unique case IDs');
  }
  const selected = cases.filter(c => ids.includes(c.id));
  if (selected.length !== ids.length) throw new Error('Unknown platform smoke case ID');
  return selected;
}

module.exports = { tsguard, python, pythonMaintainability, selectCases };
