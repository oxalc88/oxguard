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
// Canned function-graph results keyed by case id for the stand-in structure
// command. Rows are [file, symbol, line, fan_in, fan_out, call_depth].
const FUNCTION_GRAPH = {
  'criticality.distinct_callers_cross_module': { functions: [['src/core.py', 'core.target', 1, 3, 0, 0], ['src/caller_one.py', 'caller_one.one', 4, 0, 1, 1], ['src/caller_two.py', 'caller_two.two', 4, 0, 1, 1], ['src/caller_three.py', 'caller_three.three', 4, 0, 1, 1]], recursive: 0, unresolved: 0 },
  'criticality.uncalled_function_zero': { functions: [['src/lonely.py', 'lonely.never_called', 1, 0, 0, 0]], recursive: 0, unresolved: 0 },
  'criticality.same_caller_not_inflated': { functions: [['src/hub.py', 'hub.hub', 1, 1, 0, 0], ['src/busy.py', 'busy.busy', 4, 0, 1, 1]], recursive: 0, unresolved: 0 },
  'criticality.self_recursion_counts_once': { functions: [['src/recursion.py', 'recursion.countdown', 1, 0, 0, 0]], recursive: 1, unresolved: 0 },
  'function_graph.tsguard_parity': { functions: [['src/source.py', 'source.target', 1, 2, 0, 0], ['src/source.py', 'source.a', 5, 0, 1, 1], ['src/source.py', 'source.b', 10, 0, 1, 1], ['src/source.py', 'source.recursive', 14, 0, 0, 0]], recursive: 1, unresolved: 0 },
  'function_graph.method_call': { functions: [['src/methods.py', 'methods.Worker.run', 2, 0, 1, 1], ['src/methods.py', 'methods.Worker.helper', 5, 1, 0, 0]], recursive: 0, unresolved: 0 },
  'function_graph.nested_function': { functions: [['src/nested.py', 'nested.outer', 1, 0, 1, 2], ['src/nested.py', 'nested.outer.inner', 2, 1, 1, 1], ['src/nested.py', 'nested.target', 6, 1, 0, 0]], recursive: 0, unresolved: 0 },
  'function_graph.test_file_exclusion': { functions: [['src/main.py', 'main.target', 1, 0, 0, 0]], recursive: 0, unresolved: 0 },
  'function_graph.mutual_recursion_depth': { functions: [['src/mutual.py', 'mutual.start', 1, 0, 1, 2], ['src/mutual.py', 'mutual.even', 4, 2, 2, 1], ['src/mutual.py', 'mutual.odd', 10, 1, 1, 1], ['src/mutual.py', 'mutual.sink', 14, 1, 0, 0]], recursive: 1, unresolved: 0 },
  'function_graph.unresolved_dynamic_call': { functions: [['src/dynamic.py', 'dynamic.dynamic', 1, 1, 0, 0], ['src/dynamic.py', 'dynamic.caller', 5, 0, 1, 1]], recursive: 0, unresolved: 1 },
  'function_graph.encoded_source_preserves_import_root': { functions: [['src/pkg/a.py', 'pkg.a.run', 4, 0, 1, 1], ['src/pkg/b.py', 'pkg.b.target', 1, 1, 0, 0]], recursive: 0, unresolved: 0 },
  'function_graph.declared_non_utf8_bytes_are_not_utf8': { functions: [['src/latin1.py', 'latin1.\u00c2a', 4, 1, 0, 0], ['src/latin1.py', 'latin1.a', 8, 1, 0, 0], ['src/latin1.py', 'latin1.caller', 12, 0, 2, 1]], recursive: 0, unresolved: 0 },
  'function_graph.unresolved_indirect_and_closure_calls': { functions: [['src/unresolved.py', 'unresolved.first', 1, 0, 0, 0], ['src/unresolved.py', 'unresolved.plain', 5, 0, 0, 0], ['src/unresolved.py', 'unresolved.outer', 9, 0, 1, 1], ['src/unresolved.py', 'unresolved.outer.inner', 10, 0, 0, 0]], recursive: 0, unresolved: 2 },
  'function_graph.builtin_import_and_resolved_calls': { functions: [['src/boundaries.py', 'boundaries.builtin_calls', 4, 0, 0, 0], ['src/boundaries.py', 'boundaries.external_call', 9, 0, 0, 0], ['src/boundaries.py', 'boundaries.resolved_call', 13, 0, 1, 1], ['src/boundaries.py', 'boundaries.target', 17, 1, 0, 0]], recursive: 0, unresolved: 0 },
  'function_graph.unresolved_alias_and_receiver_calls': { functions: [['src/aliases.py', 'aliases.invoke', 1, 0, 0, 0], ['src/aliases.py', 'aliases.indexed', 6, 0, 0, 0], ['src/aliases.py', 'aliases.receiver', 11, 0, 0, 0]], recursive: 0, unresolved: 3 },
  'function_graph.unresolved_closure_alias': { functions: [['src/nested_alias.py', 'nested_alias.outer', 1, 0, 1, 1], ['src/nested_alias.py', 'nested_alias.outer.inner', 3, 1, 0, 0]], recursive: 0, unresolved: 1 },
  'function_graph.unparsable_is_not_evaluated': { functions: [['src/good.py', 'good.target', 1, 0, 0, 0]], recursive: 0, unresolved: 0, partial: 'Unparsable source: src/broken.py', status: 'advisory', gate: 'advisory', normalization: 'partial', assessment: 'incomplete' },
};

// Canned delegation facts for the stand-in structure command. Rows are
// [file, symbol, line, delegation_depth, module_level, forward_target, mode].
const DELEGATION = {
  'delegation.resolved_chain': {
    functions: [['src/pkg/a.py', 'pkg.a.a', 4, 3, true, 'pkg.b.b', 'sync'], ['src/pkg/b.py', 'pkg.b.b', 4, 2, true, 'pkg.c.c', 'sync'], ['src/pkg/c.py', 'pkg.c.c', 4, 1, true, 'pkg.real.real', 'sync'], ['src/pkg/real.py', 'pkg.real.real', 1, 0, true, '', '']],
    findings: [{ rule: 'LONG_DELEGATION_CHAIN', file: 'src/pkg/a.py', symbol: 'pkg.a.a', related: ['src/pkg/b.py', 'src/pkg/c.py', 'src/pkg/real.py'], observed: 3, threshold: 3 }, { rule: 'FRAGMENTED_DELEGATION', file: 'src/pkg/a.py', symbol: 'pkg.a.a', related: ['src/pkg/b.py', 'src/pkg/c.py'] }],
  },
  'delegation.equivalent_simplification_control': {
    functions: [['src/pkg/direct.py', 'pkg.direct.a', 1, 0, true, '', '']], findings: [],
  },
  'delegation.legitimate_di_validation_and_boundary': {
    functions: [['src/pkg/service.py', 'pkg.service.Dependency.load', 2, 0, false, '', ''], ['src/pkg/service.py', 'pkg.service.Service.__init__', 7, 0, false, '', ''], ['src/pkg/service.py', 'pkg.service.Service.load', 10, 0, false, '', ''], ['src/pkg/service.py', 'pkg.service.domain', 14, 0, true, '', ''], ['src/pkg/service.py', 'pkg.service.boundary', 24, 1, true, 'pkg.service.domain', 'sync'], ['src/pkg/service.py', 'pkg.service.transform', 28, 0, true, '', '']],
    findings: [],
  },
  'delegation.argument_transformation_control': {
    functions: [['src/pkg/transform.py', 'pkg.transform.real', 1, 0, true, '', ''], ['src/pkg/transform.py', 'pkg.transform.reordered', 5, 0, true, '', ''], ['src/pkg/transform.py', 'pkg.transform.transformed', 9, 0, true, '', ''], ['src/pkg/transform.py', 'pkg.transform.keyword', 13, 0, true, '', '']],
    findings: [],
  },
  'delegation.return_annotation_mismatch_control': {
    functions: [['src/pkg/annotations.py', 'pkg.annotations.real', 1, 0, true, '', ''], ['src/pkg/annotations.py', 'pkg.annotations.mismatched', 5, 0, true, '', ''], ['src/pkg/annotations.py', 'pkg.annotations.missing', 9, 0, true, '', '']],
    findings: [],
  },
  'delegation.parameter_boundary': {
    functions: [['src/pkg/parameters.py', 'pkg.parameters.real', 1, 0, true, '', ''], ['src/pkg/parameters.py', 'pkg.parameters.defaults', 5, 0, true, '', ''], ['src/pkg/parameters.py', 'pkg.parameters.varargs', 9, 0, true, '', ''], ['src/pkg/parameters.py', 'pkg.parameters.kwargs', 13, 0, true, '', ''], ['src/pkg/parameters.py', 'pkg.parameters.kwonly', 17, 0, true, '', ''], ['src/pkg/parameters.py', 'pkg.parameters.posonly', 21, 0, true, '', '']],
    findings: [],
  },
  'delegation.decorated_wrapper_control': {
    functions: [['src/pkg/decorated.py', 'pkg.decorated.real', 1, 0, true, '', ''], ['src/pkg/decorated.py', 'pkg.decorated.marker', 5, 0, true, '', ''], ['src/pkg/decorated.py', 'pkg.decorated.decorated', 10, 0, true, '', '']],
    findings: [],
  },
  'delegation.threshold_boundary': {
    functions: [['src/pkg/chains.py', 'pkg.chains.real', 1, 0, true, '', ''], ['src/pkg/chains.py', 'pkg.chains.one', 5, 1, true, 'pkg.chains.real', 'sync'], ['src/pkg/chains.py', 'pkg.chains.depth_two', 9, 2, true, 'pkg.chains.one', 'sync'], ['src/pkg/chains.py', 'pkg.chains.two', 13, 2, true, 'pkg.chains.one', 'sync'], ['src/pkg/chains.py', 'pkg.chains.depth_three', 17, 3, true, 'pkg.chains.two', 'sync']],
    findings: [{ rule: 'LONG_DELEGATION_CHAIN', file: 'src/pkg/chains.py', symbol: 'pkg.chains.depth_three', related: ['src/pkg/chains.py', 'src/pkg/chains.py', 'src/pkg/chains.py'], observed: 3, threshold: 3 }],
  },
  'delegation.forwarding_cycle': {
    functions: [['src/pkg/cycle.py', 'pkg.cycle.a', 1, 0, true, 'pkg.cycle.b', 'sync'], ['src/pkg/cycle.py', 'pkg.cycle.b', 5, 0, true, 'pkg.cycle.c', 'sync'], ['src/pkg/cycle.py', 'pkg.cycle.c', 9, 0, true, 'pkg.cycle.a', 'sync']],
    findings: [],
  },
  'delegation.test_file_exclusion': {
    functions: [['src/pkg/real.py', 'pkg.real.real', 1, 0, true, '', '']], findings: [],
  },
  'delegation.module_with_real_function': {
    functions: [['src/pkg/a.py', 'pkg.a.a', 4, 3, true, 'pkg.b.b', 'sync'], ['src/pkg/b.py', 'pkg.b.b', 4, 2, true, 'pkg.c.c', 'sync'], ['src/pkg/b.py', 'pkg.b.helper', 8, 0, true, '', ''], ['src/pkg/c.py', 'pkg.c.c', 4, 1, true, 'pkg.real.real', 'sync'], ['src/pkg/real.py', 'pkg.real.real', 1, 0, true, '', '']],
    findings: [{ rule: 'LONG_DELEGATION_CHAIN', file: 'src/pkg/a.py', symbol: 'pkg.a.a', related: ['src/pkg/b.py', 'src/pkg/c.py', 'src/pkg/real.py'], observed: 3, threshold: 3 }],
  },
  'async_delegation.return_await_chain': {
    functions: [['src/pkg/async_chain.py', 'pkg.async_chain.work', 1, 0, true, '', ''], ['src/pkg/async_chain.py', 'pkg.async_chain.c', 5, 1, true, 'pkg.async_chain.work', 'async-return-await'], ['src/pkg/async_chain.py', 'pkg.async_chain.b', 9, 2, true, 'pkg.async_chain.c', 'async-return-await'], ['src/pkg/async_chain.py', 'pkg.async_chain.a', 13, 3, true, 'pkg.async_chain.b', 'async-return-await'], ['src/pkg/async_chain.py', 'pkg.async_chain.boundary', 17, 0, true, '', '']],
    findings: [{ rule: 'LONG_DELEGATION_CHAIN', file: 'src/pkg/async_chain.py', symbol: 'pkg.async_chain.a', related: ['src/pkg/async_chain.py', 'src/pkg/async_chain.py', 'src/pkg/async_chain.py'], observed: 3, threshold: 3 }],
  },
  'async_delegation.without_await_control': {
    functions: [['src/pkg/async_plain.py', 'pkg.async_plain.work', 1, 0, true, '', ''], ['src/pkg/async_plain.py', 'pkg.async_plain.c', 5, 0, true, '', ''], ['src/pkg/async_plain.py', 'pkg.async_plain.b', 9, 0, true, '', ''], ['src/pkg/async_plain.py', 'pkg.async_plain.a', 13, 0, true, '', '']],
    findings: [],
  },
  'async_delegation.non_async_target_control': {
    functions: [['src/pkg/async_sync.py', 'pkg.async_sync.sync_work', 1, 0, true, '', ''], ['src/pkg/async_sync.py', 'pkg.async_sync.awaits_sync', 5, 0, true, '', '']],
    findings: [],
  },
};

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

const FAILURE_POLICIES = {
  'failure_policy.controls': [['SILENT_EXCEPTION_FALLBACK', 'src/policy.py', 12]],
  'failure_policy.silent_loop_exits': [['SILENT_EXCEPTION_EXIT', 'src/loops.py', 5], ['SILENT_EXCEPTION_EXIT', 'src/loops.py', 15]],
  'failure_policy.caught_name_and_other_name': [['SILENT_EXCEPTION_FALLBACK', 'src/names.py', 8]],
  'failure_policy.empty_literal_boundaries': [['SILENT_EXCEPTION_FALLBACK', 'src/literals.py', 4], ['SILENT_EXCEPTION_FALLBACK', 'src/literals.py', 8]],
  'failure_policy.pass_and_typed_controls': [],
  'failure_policy.repeated_handlers': [['REPEATED_ERROR_HANDLER', 'src/repeat.py', 4, 3, 2]],
  'failure_policy.repetition_threshold_boundary': [],
  'failure_policy.literal_sensitive_fingerprint': [],
  'failure_policy.except_star_and_scope': [['REPEATED_ERROR_HANDLER', 'src/module.py', 3, 3, 2]],
  'failure_policy.test_file_exclusion': [['SILENT_EXCEPTION_FALLBACK', 'src/main.py', 2]],
  'failure_policy.unparsable_is_incomplete': [['not_evaluated', 'src/broken.py', 1]],
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
const functionGraph = ${JSON.stringify(FUNCTION_GRAPH)};
const delegation = ${JSON.stringify(DELEGATION)};
const failurePolicies = ${JSON.stringify(FAILURE_POLICIES)};
const caseId = path.basename(path.dirname(process.cwd()));
const variant = path.basename(process.cwd());
const command = process.argv[2] || 'criticality';
if (failurePolicies[caseId] && command === 'structure') {
  const partial = caseId === 'failure_policy.unparsable_is_incomplete';
  const findings = failurePolicies[caseId].map(([rule, file, line, observed, related]) => ({ level:'structure', gate:'structure', rule: rule === 'not_evaluated' ? 'pyguard.structure.not_evaluated' : 'pyguard.structure.' + rule, severity:rule === 'not_evaluated' ? 'info' : 'warning', status:'advisory', category:'quality', location:{file,line}, ...(observed ? {observed} : {}), ...(related ? {related:Array.from({length:related},()=>({file:'src/repeat.py'}))} : {}), evidence:'advisory fixture', diagnostics:[] }));
  process.stdout.write(JSON.stringify({schema_version:'1',status:findings.length?'advisory':'pass',assessment:partial?'incomplete':'complete',gates:[{name:'structure',status:findings.length?'advisory':'passed',normalization:partial?'partial':'complete'}],exit_code:0,findings,measurements:[],artifacts:[],diagnostics:[]})); process.exit(0);
}
if (ruff[caseId]) {
  const spec = ruff[caseId];
  const findings = spec.findings.map(([rule, file, line]) => ({ rule, level: 'code', gate: 'ruff', severity: 'error', status: 'blocking', category: 'quality', location: { file, line, column: 5 }, evidence: rule + ' violation', diagnostics: [] }));
  const measurements = spec.measurement ? [{ metric: 'ruff.baseline_version', level: 'code', location: { file: '.' }, value: 1, unit: 'version' }] : [];
  process.stdout.write(JSON.stringify({ schema_version: '1', status: spec.status, assessment: 'complete', gates: [{ name: 'ruff', status: spec.gate, normalization: 'complete' }], exit_code: spec.exit, findings, measurements, artifacts: [], diagnostics: [] }));
  process.exit(spec.exit);
}
if (delegation[caseId] && command === 'structure') {
  const spec = delegation[caseId];
  const measurements = [];
  for (const [file, symbol, line, depth, moduleLevel, target, mode] of spec.functions) {
    measurements.push({ metric: 'delegation.depth', level: 'structure', location: { file, line, symbol }, value: depth, unit: 'layers' });
    if (target) measurements.push({ metric: 'delegation.forwarding', level: 'structure', location: { file, line, symbol }, value: 1, unit: 'calls', forwarding_mode: mode, forward_target: target });
    measurements.push({ metric: 'function.fan_in', level: 'structure', location: { file, line, symbol }, value: 0, unit: 'callers' });
    measurements.push({ metric: 'function.fan_out', level: 'structure', location: { file, line, symbol }, value: target ? 1 : 0, unit: 'callees' });
    measurements.push({ metric: 'function.call_depth', level: 'structure', location: { file, line, symbol }, value: 0, unit: 'edges' });
  }
  const findings = spec.findings.map(finding => ({ level: 'structure', gate: 'structure', rule: 'pyguard.structure.' + finding.rule, severity: 'warning', status: 'advisory', category: 'quality', location: { file: finding.file, line: finding.line || 1, symbol: finding.symbol }, related: (finding.related || []).map(file => ({ file })), ...(finding.observed !== undefined ? { observed: finding.observed } : {}), ...(finding.threshold !== undefined ? { threshold: finding.threshold } : {}), evidence: finding.rule + ' advisory', diagnostics: [] }));
  const status = findings.length ? 'advisory' : 'pass';
  const gate = findings.length ? 'advisory' : 'passed';
  process.stdout.write(JSON.stringify({ schema_version: '1', status, assessment: 'complete', gates: [{ name: 'structure', status: gate, normalization: 'complete' }], exit_code: 0, findings, measurements, artifacts: [], diagnostics: [] }));
  process.exit(0);
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
if (functionGraph[caseId] && command === 'structure') {
  const spec = functionGraph[caseId];
  const measurements = [];
  for (const [file, symbol, line, fanIn, fanOut, depth] of spec.functions) {
    measurements.push({ metric: 'function.fan_in', level: 'structure', location: { file, line, symbol }, value: fanIn, unit: 'callers' });
    measurements.push({ metric: 'function.fan_out', level: 'structure', location: { file, line, symbol }, value: fanOut, unit: 'callees' });
    measurements.push({ metric: 'function.call_depth', level: 'structure', location: { file, line, symbol }, value: depth, unit: 'edges' });
  }
  measurements.push({ metric: 'function.recursive_components', level: 'structure', location: { file: '.' }, value: spec.recursive, unit: 'components' });
  measurements.push({ metric: 'calls.unresolved', level: 'structure', location: { file: '.' }, value: spec.unresolved, unit: 'calls' });
  const findings = spec.partial ? [{ level: 'structure', gate: 'structure', rule: 'pyguard.structure.not_evaluated', severity: 'info', status: 'advisory', category: 'quality', evidence: spec.partial, diagnostics: [] }] : [];
  const status = spec.status || (spec.partial ? 'advisory' : 'pass');
  const gate = spec.gate || (spec.partial ? 'advisory' : 'passed');
  process.stdout.write(JSON.stringify({ schema_version: '1', status, assessment: spec.assessment || 'complete', gates: [{ name: 'structure', status: gate, normalization: spec.normalization || 'complete' }], exit_code: 0, findings, measurements, artifacts: [], diagnostics: [] }));
  process.exit(0);
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
  return ({ project, args }) => {
    const response = spawnSync(process.execPath, [binary, ...(args || [])], { cwd: project, encoding: 'utf8', env: { ...process.env, ...env }, timeout: 30000 });
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
  assert.deepEqual([...new Set(definitions.map(d => d.capability))], ['quality.zero_config_exception_handling', 'maintainability.delegation', 'maintainability.failure_policies', 'structure.runtime_graph', 'structure.false_positive_controls', 'structure.function_graph', 'structure.coupling', 'maintainability.async_delegation', 'quality.cognitive_complexity']);
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
