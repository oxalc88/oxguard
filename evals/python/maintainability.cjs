'use strict';
// Seeded Python maintainability evals for issue #23. The harness builds small
// fixture projects in a temp directory and runs the built pyguard binary's
// informational `criticality` command, the `ruff` gate (including the
// zero-config S110/B012 baseline) and the `radon` gate (radon + complexipy)
// against them; no production project and no LLM is involved. Case IDs,
// capabilities and kinds mirror evals/maintainability-capabilities.json
// (enforced by this suite's tests and by evals/maintainability-parity.cjs when
// it validates this report).
const assert = require('node:assert/strict');
const fs = require('node:fs');
const os = require('node:os');
const path = require('node:path');
const crypto = require('node:crypto');
const { spawnSync } = require('node:child_process');
const { performance } = require('node:perf_hooks');

const ANALYSIS = path.join(__dirname, '..', '..', 'pyguard', 'analysis');
const EVALUATOR_SHA256 = crypto.createHash('sha256').update(fs.readFileSync(__filename)).digest('hex');
// managed=false keeps uv in the caller-provided pinned environment (same
// policy as evals/python/fixtures); a project-local .venv would hide pyan3.
const PYPROJECT = ['[project]', 'name = "oxguard-maintainability-eval"', 'version = "0.0.0"', 'requires-python = ">=3.12"', '[tool.uv]', 'managed = false', ''].join('\n');

function complete(result, gateName = 'criticality') {
  assert.equal(result.assessment, 'complete', `${gateName} assessment was ${result.assessment}`);
  const gate = result.gates.find(g => g.name === gateName);
  assert.ok(gate, `${gateName} gate missing`);
  // Advisory gate: it reports 'advisory' with ranked findings and 'passed'
  // without them; 'error'/'not_run' are analysis failures, never a pass.
  assert.ok(['passed', 'advisory'].includes(gate.status), `${gateName} gate was ${gate.status}`);
}

// The only outcome this harness treats as blocking: a failed/error run, a
// non-zero exit or a failed/error gate. Advisory status never counts.
function blockingOutcome(result) {
  return result.exit_code !== 0 || ['fail', 'error'].includes(result.status)
    || result.gates.some(g => ['failed', 'error'].includes(g.status));
}

function inDegree(result, file, symbol) {
  const found = result.measurements.find(m => m.metric === 'criticality.in_degree' && m.location.file === file && m.location.symbol === symbol);
  assert.ok(found, `missing criticality.in_degree for ${symbol} in ${file}; measurements: ${JSON.stringify(result.measurements)}`);
  return found.value;
}

function ranked(result, symbol) {
  return result.findings.find(f => f.rule === 'pyguard.criticality.ranked' && f.location.symbol === symbol);
}

// Known answers below were verified against the built binary. pyan3 derives
// module names from the files under a single requested root, so every fixture
// keeps its sources directly in src/ and symbols read `<module>.<function>`.
// cognitive fixtures below are score-verified against complexipy 8.3.0: a
// chain of d nested ifs scores d(d+1)/2 (6 → 21, 8 → 36) while radon CC stays
// at d+1 (7, 9), so the radon gate itself stays green in every fixture.
const nestChain = (depth, name) => {
  const lines = [`def ${name}(x):`, '    if x > 0:'];
  for (let level = 1; level < depth; level++) {
    const indent = '    '.repeat(level + 1);
    lines.push(`${indent}if x > ${level}:`, `${indent}    pass`);
  }
  lines.push('    return x');
  return lines.join('\n') + '\n';
};
// Two independent chains of four score (1+2+3+4) × 2 = 20 at exactly the
// threshold, with radon CC 9.
const twoChains = (name) => {
  const lines = [`def ${name}(x, y):`];
  for (const param of ['x', 'y']) {
    lines.push(`    if ${param} > 0:`);
    for (let level = 1; level <= 3; level++) {
      const indent = '    '.repeat(level + 1);
      lines.push(`${indent}if ${param} > ${level}:`, `${indent}    pass`);
    }
  }
  lines.push('    return x + y');
  return lines.join('\n') + '\n';
};
const flatFunction = 'def flat(value):\n    return value + 1\n';
// Python accepts a UTF-8 BOM and a PEP 263 non-UTF-8 source declaration.
// Write raw bytes so the fixture is identical on Windows and POSIX and is
// never silently re-encoded by the harness.
const utf8BomSource = (text) => Buffer.concat([Buffer.from([0xef, 0xbb, 0xbf]), Buffer.from(text, 'utf8')]);
const latin1Source = (text) => Buffer.from(text, 'latin1');
// complexipy folds nested functions into the enclosing function: an inner
// chain of eight scores 36 and is reported once under the outer name.
const nestedFunctionChain = (depth, outer, inner) => {
  const lines = [`def ${outer}(x):`, `    def ${inner}(y):`, '        if y > 0:'];
  for (let level = 1; level < depth; level++) {
    const indent = '    '.repeat(level + 2);
    lines.push(`${indent}if y > ${level}:`, `${indent}    pass`);
  }
  lines.push('        return y', `    return ${inner}(x)`);
  return lines.join('\n') + '\n';
};

function cognitiveMeasurement(result, symbol) {
  return result.measurements.find(m => m.metric === 'complexipy.cognitive' && m.location.symbol === symbol);
}
function cognitiveWarning(result, symbol) {
  return result.findings.find(f => f.rule === 'pyguard.complexipy.cognitive_exceeded' && f.location.symbol === symbol);
}
function structureMeasurement(result, metric) {
  const found = result.measurements.find(m => m.metric === metric && m.location.file === '.');
  assert.ok(found, `missing ${metric}; measurements: ${JSON.stringify(result.measurements)}`);
  return found.value;
}
function structureDepth(result, file) {
  const found = result.measurements.find(m => m.metric === 'module.dependency_depth' && m.location.file === file);
  assert.ok(found, `missing module.dependency_depth for ${file}; measurements: ${JSON.stringify(result.measurements)}`);
  return found.value;
}
function findModuleMeasurement(result, metric, file) {
  return result.measurements.find(m => m.metric === metric && m.location.file === file);
}
function moduleMeasurement(result, metric, file) {
  const found = findModuleMeasurement(result, metric, file);
  assert.ok(found, `missing ${metric} for ${file}; measurements: ${JSON.stringify(result.measurements)}`);
  return found;
}
function circularFindings(result) {
  return result.findings.filter(f => f.rule === 'pyguard.structure.CIRCULAR_DEPENDENCY');
}
function couplingFindings(result) {
  return result.findings.filter(f => f.rule === 'pyguard.structure.HIGH_MODULE_COUPLING');
}
// A hub plus callers and leaf dependencies. The eighth dependency can be made
// type-only, deferred or external; all three are controlled by the analyzer
// and must not contribute to fan-out. Excluded/test fixtures modify the files
// or pyproject after generation.
function couplingFixture({ importers = 3, dependencies = 8, eighth = 'runtime' } = {}) {
  const files = { 'src/pkg/__init__.py': '' };
  for (let i = 0; i < dependencies; i++) files[`src/pkg/leaf${i}.py`] = `VALUE${i} = ${i}\n`;
  const lines = [];
  const runtimeDependencies = eighth === 'type' || eighth === 'deferred' ? dependencies - 1 : dependencies;
  for (let i = 0; i < runtimeDependencies; i++) lines.push(`from . import leaf${i}`);
  if (eighth === 'type') {
    lines.push('from typing import TYPE_CHECKING', '', 'if TYPE_CHECKING:', `    from . import leaf${dependencies - 1}`);
  } else if (eighth === 'deferred') {
    lines.push('', 'def load():', `    from . import leaf${dependencies - 1}`, `    return leaf${dependencies - 1}`);
  } else if (eighth === 'external') {
    lines.push('import json');
  }
  files['src/pkg/hub.py'] = lines.join('\n') + '\n';
  for (let i = 0; i < importers; i++) files[`src/pkg/caller${i}.py`] = 'from .hub import VALUE0\n';
  return files;
}

function ruffMeasurement(result) {
  return result.measurements.find(m => m.metric === 'ruff.baseline_version');
}
function ruffFinding(result, code) {
  return result.findings.find(f => f.rule === code);
}
function assertBlockingRuff(result, code, file, line) {
  assert.equal(result.exit_code, 1);
  assert.equal(result.status, 'fail');
  const finding = ruffFinding(result, code);
  assert.ok(finding, `missing ${code}; findings: ${JSON.stringify(result.findings)}`);
  assert.equal(finding.gate, 'ruff');
  assert.equal(finding.category, 'quality');
  assert.equal(finding.status, 'blocking');
  assert.equal(finding.severity, 'error');
  assert.equal(finding.location.file, file);
  assert.equal(finding.location.line, line);
  assert.equal(result.gates.find(g => g.name === 'ruff').status, 'failed');
  const measurement = ruffMeasurement(result);
  assert.ok(measurement, 'missing ruff.baseline_version measurement');
  assert.equal(measurement.level, 'code');
  assert.equal(measurement.location.file, '.');
  assert.equal(measurement.value, 1);
  assert.equal(measurement.unit, 'version');
}

const definitions = [
  {
    id: 'ruff.baseline.empty_handler_is_blocking',
    capability: 'quality.zero_config_exception_handling',
    kind: 'positive',
    command: 'ruff',
    exitCode: 1,
    files: {
      'src/handlers.py': 'def swallow() -> None:\n    try:\n        int("x")\n    except Exception:  # noqa: BLE001\n        pass\n',
    },
    check: ({ result }) => {
      // No Ruff configuration at the project root, so pyguard adds
      // --extend-select S110,B012 and records the baseline version. S110 is a
      // blocking quality finding: exit 1 and a failed ruff gate. The fixture's
      // BLE001 suppression only hides the unrelated blind-except rule.
      assertBlockingRuff(result, 'S110', 'src/handlers.py', 4);
    },
  },
  {
    id: 'ruff.baseline.finally_jump_is_blocking',
    capability: 'quality.zero_config_exception_handling',
    kind: 'positive',
    command: 'ruff',
    exitCode: 1,
    files: {
      'src/finally.py': 'def value() -> int:\n    try:\n        return 1\n    finally:\n        return 2  # noqa: SIM107\n',
    },
    check: ({ result }) => {
      // B012 is selected by the same zero-config baseline and blocks: a return
      // inside finally discards a pending exception.
      assertBlockingRuff(result, 'B012', 'src/finally.py', 5);
    },
  },
  {
    id: 'ruff.baseline.logging_contrast_pass_is_blocking',
    capability: 'quality.zero_config_exception_handling',
    kind: 'positive',
    command: 'ruff',
    exitCode: 1,
    files: {
      'src/handlers.py': [
        'import logging',
        '',
        'logger = logging.getLogger(__name__)',
        '',
        '',
        'def logs() -> None:',
        '    try:',
        '        int("x")',
        '    except Exception:  # noqa: BLE001',
        '        pass',
        '',
      ].join('\n'),
    },
    check: ({ result }) => {
      // Contrast for the logging negative case: the same handler and exception
      // type, but with `pass` instead of logging, is exactly what S110 flags.
      // BLE001 is suppressed only because the blind `Exception` handler is
      // otherwise unrelated to proving S110.
      assertBlockingRuff(result, 'S110', 'src/handlers.py', 9);
    },
  },
  {
    id: 'ruff.baseline.logging_or_reraise_is_clean',
    capability: 'quality.zero_config_exception_handling',
    kind: 'negative',
    command: 'ruff',
    files: {
      'src/handlers.py': [
        'import logging',
        '',
        'logger = logging.getLogger(__name__)',
        '',
        '',
        'def logs() -> None:',
        '    try:',
        '        int("x")',
        '    except Exception:',
        '        logger.exception("failed")',
        '',
        '',
        'def reraises() -> None:',
        '    try:',
        '        int("x")',
        '    except Exception:  # noqa: TRY203',
        '        raise',
        '',
      ].join('\n'),
    },
    check: ({ result }) => {
      // Both handlers catch Exception, so each would trigger S110 if its body
      // were only `pass` (proved by logging_contrast_pass_is_blocking).
      // Logging and re-raising bodies are not empty handlers, so S110 stays
      // silent. TRY203 is the only unrelated finding and is suppressed; the
      // fixture is otherwise clean.
      assert.equal(result.exit_code, 0);
      assert.equal(result.status, 'pass');
      assert.equal(ruffFinding(result, 'S110'), undefined, JSON.stringify(result.findings));
      assert.ok(ruffMeasurement(result), 'zero-config run must record the baseline version');
    },
  },
  {
    id: 'ruff.baseline.project_ignore_preserved',
    capability: 'quality.zero_config_exception_handling',
    kind: 'negative',
    command: 'ruff',
    pyproject: PYPROJECT + '[tool.ruff.lint]\nselect = ["S110"]\nignore = ["S110"]\n',
    files: {
      'src/handlers.py': 'def swallow() -> None:\n    try:\n        int("x")\n    except Exception:\n        pass\n',
    },
    check: ({ result }) => {
      // A project Ruff config, even one that only ignores S110, disables the
      // baseline. No --extend-select is added (Ruff's CLI select would override
      // the ignore), and no baseline measurement is recorded.
      assert.equal(result.exit_code, 0);
      assert.equal(result.status, 'pass');
      assert.equal(ruffFinding(result, 'S110'), undefined, JSON.stringify(result.findings));
      assert.equal(ruffMeasurement(result), undefined, JSON.stringify(result.measurements));
    },
  },
  {
    id: 'ruff.baseline.noqa_suppression_respected',
    capability: 'quality.zero_config_exception_handling',
    kind: 'negative',
    command: 'ruff',
    files: {
      'src/handlers.py': 'def swallow() -> None:\n    try:\n        int("x")\n    except Exception:  # noqa: S110,BLE001\n        pass\n',
    },
    check: ({ result }) => {
      // Ruff's native inline suppression is respected; the baseline is still
      // active and recorded.
      assert.equal(result.exit_code, 0);
      assert.equal(result.status, 'pass');
      assert.equal(ruffFinding(result, 'S110'), undefined, JSON.stringify(result.findings));
      assert.ok(ruffMeasurement(result), 'zero-config run must record the baseline version');
    },
  },
  {
    id: 'ruff.baseline.ellipsis_body_not_flagged',
    capability: 'quality.zero_config_exception_handling',
    kind: 'boundary',
    command: 'ruff',
    files: {
      'src/handlers.py': 'def swallow() -> None:\n    try:\n        int("x")\n    except Exception:  # noqa: BLE001\n        ...\n',
    },
    check: ({ result }) => {
      // Documented boundary: Ruff 0.17.2 S110 flags an empty `pass` body, not an
      // ellipsis body. PyGuard keeps that analyzer semantics rather than
      // inventing a broader empty-handler rule.
      assert.equal(result.exit_code, 0);
      assert.equal(result.status, 'pass');
      assert.equal(ruffFinding(result, 'S110'), undefined, JSON.stringify(result.findings));
      assert.ok(ruffMeasurement(result), 'zero-config run must record the baseline version');
    },
  },
  {
    id: 'ruff.baseline.typed_exception_handler_not_flagged',
    capability: 'quality.zero_config_exception_handling',
    kind: 'boundary',
    command: 'ruff',
    files: {
      'src/handlers.py': 'def swallow_typed() -> None:\n    try:\n        int("x")\n    except ValueError:\n        pass\n',
    },
    check: ({ result }) => {
      // Documented limitation: Ruff's default `check-typed-exception = false`
      // means S110 only checks bare/Exception/BaseException handlers. A
      // `pass`-only typed handler such as `except ValueError: pass` is not
      // flagged, and pyguard keeps that upstream semantics rather than adding
      // S112 or enabling check-typed-exception.
      assert.equal(result.exit_code, 0);
      assert.equal(result.status, 'pass');
      assert.equal(ruffFinding(result, 'S110'), undefined, JSON.stringify(result.findings));
      assert.ok(ruffMeasurement(result), 'zero-config run must record the baseline version');
    },
  },
  {
    id: 'ruff.baseline.test_files_still_linted',
    capability: 'quality.zero_config_exception_handling',
    kind: 'boundary',
    command: 'ruff',
    exitCode: 1,
    files: {
      'src/tests/test_handlers.py': 'def test_swallow() -> None:\n    try:\n        int("x")\n    except Exception:  # noqa: BLE001\n        pass\n',
    },
    check: ({ result }) => {
      // Existing pyguard policy excludes conventional test files from the
      // complexity analyzers, not from Ruff. Ruff still gates test code, so the
      // zero-config baseline reports S110 inside tests/ and blocks.
      assertBlockingRuff(result, 'S110', 'src/tests/test_handlers.py', 4);
    },
  },
  {
    id: 'structure.runtime_cycle',
    capability: 'structure.runtime_graph',
    kind: 'positive',
    command: 'structure',
    files: {
      'src/pkg/__init__.py': '',
      'src/pkg/a.py': 'from .b import B\n\n\ndef load():\n    return B\n',
      'src/pkg/b.py': 'from .a import load\n',
    },
    check: ({ result }) => {
      // A runtime a<->b SCC is one advisory finding. pyguard stays at exit 0
      // with no blocking gate outcome, exactly like a clean structure run.
      complete(result, 'structure');
      const findings = circularFindings(result);
      assert.equal(findings.length, 1, JSON.stringify(result.findings));
      assert.equal(findings[0].severity, 'warning');
      assert.equal(findings[0].status, 'advisory');
      assert.equal(findings[0].category, 'quality');
      assert.equal(findings[0].location.file, 'src/pkg/a.py');
      assert.deepEqual(findings[0].related.map(location => location.file), ['src/pkg/b.py']);
      assert.equal(structureMeasurement(result, 'module.cycles'), 1);
      assert.equal(structureMeasurement(result, 'module.edges'), 2);
      assert.equal(structureMeasurement(result, 'module.count'), 3);
      assert.equal(structureDepth(result, 'src/pkg/a.py'), 0);
      assert.equal(structureDepth(result, 'src/pkg/b.py'), 0);
      assert.equal(result.exit_code, 0);
      assert.equal(blockingOutcome(result), false);
    },
  },
  {
    id: 'structure.self_import_cycle',
    capability: 'structure.runtime_graph',
    kind: 'positive',
    command: 'structure',
    files: {
      'src/pkg/__init__.py': '',
      'src/pkg/a.py': 'import pkg.a\n\ndef load():\n    return pkg.a\n',
    },
    check: ({ result }) => {
      // A self-loop is a strongly connected component of size one and counts
      // exactly like a multi-module cycle; it remains advisory.
      complete(result, 'structure');
      const findings = circularFindings(result);
      assert.equal(findings.length, 1, JSON.stringify(result.findings));
      assert.equal(findings[0].location.file, 'src/pkg/a.py');
      assert.deepEqual(findings[0].related || [], []);
      assert.equal(structureMeasurement(result, 'module.cycles'), 1);
      assert.equal(structureMeasurement(result, 'module.edges'), 1);
      assert.equal(structureDepth(result, 'src/pkg/a.py'), 0);
      assert.equal(result.exit_code, 0);
      assert.equal(blockingOutcome(result), false);
    },
  },
  {
    id: 'structure.nested_package_cycle',
    capability: 'structure.runtime_graph',
    kind: 'positive',
    command: 'structure',
    files: {
      'src/pkg/__init__.py': '',
      'src/pkg/sub/__init__.py': '',
      'src/pkg/sub/a.py': 'from ..b import load\n',
      'src/pkg/b.py': 'from .sub.a import load\n',
    },
    check: ({ result }) => {
      // Intermediate package directories must survive in module identities:
      // pkg/sub/a.py is pkg.sub.a, not a flattened pkg.a, and pkg.b <-> pkg.sub.a
      // is one real SCC. Flattening previously dropped modules or panicked grimp.
      complete(result, 'structure');
      const findings = circularFindings(result);
      assert.equal(findings.length, 1, JSON.stringify(result.findings));
      assert.equal(findings[0].location.file, 'src/pkg/b.py');
      assert.deepEqual(findings[0].related.map(location => location.file), ['src/pkg/sub/a.py']);
      assert.equal(structureMeasurement(result, 'module.cycles'), 1);
      assert.equal(structureMeasurement(result, 'module.edges'), 2);
      assert.equal(structureMeasurement(result, 'module.count'), 4);
      assert.equal(structureDepth(result, 'src/pkg/sub/a.py'), 0);
      assert.equal(structureDepth(result, 'src/pkg/b.py'), 0);
      assert.equal(result.exit_code, 0);
      assert.equal(blockingOutcome(result), false);
    },
  },
  {
    id: 'structure.namespace_package_cycle',
    capability: 'structure.runtime_graph',
    kind: 'positive',
    command: 'structure',
    files: {
      'src/pkg/a.py': 'import pkg.b\n',
      'src/pkg/b.py': 'import pkg.a\n',
    },
    check: ({ result }) => {
      // Without __init__.py these files are still an importable namespace
      // package. Their dotted identities participate in the same runtime cycle
      // instead of becoming unrelated bare modules named a and b.
      complete(result, 'structure');
      const findings = circularFindings(result);
      assert.equal(findings.length, 1, JSON.stringify(result.findings));
      assert.equal(findings[0].location.file, 'src/pkg/a.py');
      assert.deepEqual(findings[0].related.map(location => location.file), ['src/pkg/b.py']);
      assert.equal(structureMeasurement(result, 'module.cycles'), 1);
      assert.equal(structureMeasurement(result, 'module.edges'), 2);
      assert.equal(structureMeasurement(result, 'module.count'), 2);
      assert.equal(structureDepth(result, 'src/pkg/a.py'), 0);
      assert.equal(structureDepth(result, 'src/pkg/b.py'), 0);
      assert.equal(result.exit_code, 0);
      assert.equal(blockingOutcome(result), false);
    },
  },
  {
    id: 'structure.namespace_relative_import_cycle',
    capability: 'structure.runtime_graph',
    kind: 'positive',
    command: 'structure',
    files: {
      'src/pkg/a.py': 'from . import b\n',
      'src/pkg/b.py': 'from . import a\n',
    },
    check: ({ result }) => {
      // Namespace packages have no __init__.py, but `from . import b` still
      // validates b as pkg.b (not an attribute of a missing package module).
      complete(result, 'structure');
      const findings = circularFindings(result);
      assert.equal(findings.length, 1, JSON.stringify(result.findings));
      assert.equal(findings[0].location.file, 'src/pkg/a.py');
      assert.deepEqual(findings[0].related.map(location => location.file), ['src/pkg/b.py']);
      assert.equal(structureMeasurement(result, 'module.cycles'), 1);
      assert.equal(structureMeasurement(result, 'module.edges'), 2);
      assert.equal(structureMeasurement(result, 'module.count'), 2);
      assert.equal(result.exit_code, 0);
      assert.equal(blockingOutcome(result), false);
    },
  },
  {
    id: 'structure.default_scope_src_layout_cycle',
    capability: 'structure.runtime_graph',
    kind: 'positive',
    command: 'structure',
    dirs: '.',
    pyproject: PYPROJECT + '[tool.pyguard]\nexclude = ["tools/*"]\n',
    files: {
      'src/pkg/__init__.py': '',
      'src/pkg/a.py': 'import pkg.b\n',
      'src/pkg/b.py': 'import pkg.a\n',
    },
    check: ({ result }) => {
      // The default '.' scope must still use src as the import root for files
      // under src, so pkg.a <-> pkg.b is a real cycle instead of src.pkg.*
      // identities whose imports all look external.
      complete(result, 'structure');
      const findings = circularFindings(result);
      assert.equal(findings.length, 1, JSON.stringify(result.findings));
      assert.equal(findings[0].location.file, 'src/pkg/a.py');
      assert.deepEqual(findings[0].related.map(location => location.file), ['src/pkg/b.py']);
      assert.equal(structureMeasurement(result, 'module.cycles'), 1);
      assert.equal(structureMeasurement(result, 'module.edges'), 2);
      assert.equal(structureMeasurement(result, 'module.count'), 3);
      assert.equal(structureDepth(result, 'src/pkg/a.py'), 0);
      assert.equal(structureDepth(result, 'src/pkg/b.py'), 0);
      assert.equal(result.exit_code, 0);
      assert.equal(blockingOutcome(result), false);
    },
  },
  {
    id: 'structure.dunder_package_attribute_cycle',
    capability: 'structure.runtime_graph',
    kind: 'positive',
    command: 'structure',
    files: {
      // Python supplies __path__, __name__ and the other module dunders, so
      // `from pkg import __path__` is a real attribute import. It must close
      // the pkg <-> pkg.worker cycle instead of being unresolved.
      'src/pkg/__init__.py': 'import pkg.worker\n',
      'src/pkg/worker.py': 'from pkg import __path__, __name__, __spec__\nfrom pkg import __file__, __loader__, __package__\n',
    },
    check: ({ result }) => {
      complete(result, 'structure');
      assert.equal(result.assessment, 'complete');
      const findings = circularFindings(result);
      assert.equal(findings.length, 1, JSON.stringify(result.findings));
      assert.equal(findings[0].location.file, 'src/pkg/__init__.py');
      assert.deepEqual(findings[0].related.map(location => location.file), ['src/pkg/worker.py']);
      assert.equal(structureMeasurement(result, 'module.count'), 2);
      assert.equal(structureMeasurement(result, 'module.edges'), 2);
      assert.equal(structureMeasurement(result, 'module.cycles'), 1);
      assert.equal(structureDepth(result, 'src/pkg/__init__.py'), 0);
      assert.equal(structureDepth(result, 'src/pkg/worker.py'), 0);
      assert.equal(result.exit_code, 0);
      assert.equal(blockingOutcome(result), false);
    },
  },
  {
    id: 'structure.runtime_binding_exports_control',
    capability: 'structure.runtime_graph',
    kind: 'positive',
    command: 'structure',
    files: {
      // Imports aliases, for/with/except targets, walrus assignments, class
      // definitions and names declared global inside functions all bind
      // runtime package attributes. Each must validate, closing the cycle.
      'src/pkg/__init__.py': "import os.path as helper\nimport pkg.worker\n\n\ndef setup():\n    global plugin\n    plugin = 1\n\n\nclass Service:\n    pass\n\nfor item in ():\n    pass\n\nwith open('unused') as handle:\n    pass\n\ntry:\n    recovered = 1\nexcept Exception as error:\n    pass\n\nif (loaded := None) is None:\n    pass\n",
      'src/pkg/worker.py': 'from pkg import plugin, Service, item, handle, recovered, error, loaded, helper\n',
    },
    check: ({ result }) => {
      complete(result, 'structure');
      assert.equal(result.assessment, 'complete');
      const findings = circularFindings(result);
      assert.equal(findings.length, 1, JSON.stringify(result.findings));
      assert.equal(findings[0].location.file, 'src/pkg/__init__.py');
      assert.deepEqual(findings[0].related.map(location => location.file), ['src/pkg/worker.py']);
      assert.equal(structureMeasurement(result, 'module.count'), 2);
      assert.equal(structureMeasurement(result, 'module.edges'), 2);
      assert.equal(structureMeasurement(result, 'module.cycles'), 1);
      assert.equal(result.exit_code, 0);
      assert.equal(blockingOutcome(result), false);
    },
  },
  {
    id: 'structure.conditional_export_cycle',
    capability: 'structure.runtime_graph',
    kind: 'positive',
    command: 'structure',
    files: {
      // VALUE is bound in both runtime branches of a sys.version_info check.
      // Treating only one branch as exported previously made `from pkg import
      // VALUE` unresolved, withheld worker.py from grimp and hid the cycle.
      'src/pkg/__init__.py': "import sys\nimport pkg.worker\n\nif sys.version_info >= (3, 12):\n    VALUE = 1\nelse:\n    VALUE = 2\n",
      'src/pkg/worker.py': 'from pkg import VALUE\n',
    },
    check: ({ result }) => {
      complete(result, 'structure');
      assert.equal(result.assessment, 'complete');
      const findings = circularFindings(result);
      assert.equal(findings.length, 1, JSON.stringify(result.findings));
      assert.equal(findings[0].location.file, 'src/pkg/__init__.py');
      assert.deepEqual(findings[0].related.map(location => location.file), ['src/pkg/worker.py']);
      assert.equal(structureMeasurement(result, 'module.count'), 2);
      assert.equal(structureMeasurement(result, 'module.edges'), 2);
      assert.equal(structureMeasurement(result, 'module.cycles'), 1);
      assert.equal(structureMeasurement(result, 'module.type_imports'), 0);
      assert.equal(structureDepth(result, 'src/pkg/__init__.py'), 0);
      assert.equal(structureDepth(result, 'src/pkg/worker.py'), 0);
      assert.equal(result.exit_code, 0);
      assert.equal(blockingOutcome(result), false);
    },
  },
  {
    id: 'structure.utf8_bom_source_cycle',
    capability: 'structure.runtime_graph',
    kind: 'positive',
    command: 'structure',
    files: {
      'src/pkg/__init__.py': '',
      'src/pkg/a.py': utf8BomSource('from .b import B\n\n\ndef load():\n    return B\n'),
      'src/pkg/b.py': utf8BomSource('from .a import load\n'),
    },
    check: ({ result }) => {
      // Windows editors commonly add a UTF-8 BOM. Python accepts it, so both
      // modules must remain in the graph and their runtime SCC must survive.
      complete(result, 'structure');
      const findings = circularFindings(result);
      assert.equal(findings.length, 1, JSON.stringify(result.findings));
      assert.equal(findings[0].location.file, 'src/pkg/a.py');
      assert.deepEqual(findings[0].related.map(location => location.file), ['src/pkg/b.py']);
      assert.equal(structureMeasurement(result, 'module.count'), 3);
      assert.equal(structureMeasurement(result, 'module.edges'), 2);
      assert.equal(structureMeasurement(result, 'module.cycles'), 1);
      assert.equal(structureMeasurement(result, 'module.deferred_imports'), 0);
      assert.equal(result.exit_code, 0);
      assert.equal(blockingOutcome(result), false);
    },
  },
  {
    id: 'structure.latin1_declared_source_cycle',
    capability: 'structure.runtime_graph',
    kind: 'positive',
    command: 'structure',
    files: {
      'src/pkg/__init__.py': '',
      'src/pkg/a.py': latin1Source('# -*- coding: latin-1 -*-\n# caf\u00e9\nfrom .b import B\n\n\ndef load():\n    return B\n'),
      'src/pkg/b.py': latin1Source('# -*- coding: latin-1 -*-\n# caf\u00e9\nfrom .a import load\n'),
    },
    check: ({ result }) => {
      // A valid PEP 263 declaration makes these non-UTF-8 bytes importable by
      // Python. The AST pass and grimp must agree on the same source lines.
      complete(result, 'structure');
      const findings = circularFindings(result);
      assert.equal(findings.length, 1, JSON.stringify(result.findings));
      assert.equal(findings[0].location.file, 'src/pkg/a.py');
      assert.deepEqual(findings[0].related.map(location => location.file), ['src/pkg/b.py']);
      assert.equal(structureMeasurement(result, 'module.count'), 3);
      assert.equal(structureMeasurement(result, 'module.edges'), 2);
      assert.equal(structureMeasurement(result, 'module.cycles'), 1);
      assert.equal(structureMeasurement(result, 'module.deferred_imports'), 0);
      assert.equal(result.exit_code, 0);
      assert.equal(blockingOutcome(result), false);
    },
  },
  {
    id: 'structure.acyclic_chain',
    capability: 'structure.runtime_graph',
    kind: 'negative',
    command: 'structure',
    files: {
      'src/pkg/__init__.py': '',
      'src/pkg/a.py': 'from .b import B\n',
      'src/pkg/b.py': 'from .c import C\n',
      'src/pkg/c.py': '',
    },
    check: ({ result }) => {
      // a -> b -> c has no cycle. SCC-condensation depth is the longest path
      // to a leaf: a=2, b=1, c=0, and a package __init__ with no imports is 0.
      complete(result, 'structure');
      assert.equal(result.findings.length, 0, JSON.stringify(result.findings));
      assert.equal(structureMeasurement(result, 'module.cycles'), 0);
      assert.equal(structureMeasurement(result, 'module.edges'), 2);
      assert.equal(structureMeasurement(result, 'module.count'), 4);
      assert.equal(structureDepth(result, 'src/pkg/a.py'), 2);
      assert.equal(structureDepth(result, 'src/pkg/b.py'), 1);
      assert.equal(structureDepth(result, 'src/pkg/c.py'), 0);
      assert.equal(structureDepth(result, 'src/pkg/__init__.py'), 0);
    },
  },
  {
    id: 'structure.scope_and_test_exclusions',
    capability: 'structure.runtime_graph',
    kind: 'boundary',
    command: 'structure',
    pyproject: PYPROJECT + '[tool.pyguard]\nexclude = ["src/pkg/ignored/*"]\n',
    files: {
      'src/pkg/__init__.py': '',
      'src/pkg/main.py': 'from . import helper\n',
      'src/pkg/helper.py': '',
      'src/pkg/ignored/source.py': 'from .missing import value\n',
      'src/pkg/test_bad.py': 'from .missing import value\n',
      'src/pkg/tests/test_nested.py': 'from .missing import value\n',
    },
    check: ({ result }) => {
      // [tool.pyguard] exclude removes the ignored directory and the default
      // test exclusion removes test_*.py plus whole tests/ directories. None
      // of the excluded files' unresolved imports may make the graph partial.
      complete(result, 'structure');
      assert.equal(result.assessment, 'complete');
      assert.equal(structureMeasurement(result, 'module.count'), 3);
      assert.equal(structureMeasurement(result, 'module.edges'), 1);
      assert.equal(circularFindings(result).length, 0);
    },
  },
  {
    id: 'structure.duplicate_identities_do_not_crash',
    capability: 'structure.runtime_graph',
    kind: 'boundary',
    command: 'structure',
    files: {
      // The first two files keep distinct namespace-module identities. The
      // last two deliberately collide on pkg.a; the analyzer must keep one
      // deterministically and report incomplete evidence instead of crashing.
      'src/one/a.py': 'import a\n',
      'src/two/a.py': 'import a\n',
      'src/pkg/a.py': 'VALUE = 1\n',
      'src/pkg/a/__init__.py': 'VALUE = 2\n',
    },
    check: ({ result }) => {
      const findings = result.findings.filter(f => f.rule === 'pyguard.structure.not_evaluated');
      assert.equal(findings.length, 1, JSON.stringify(result.findings));
      assert.match(findings[0].evidence, /Duplicate module identity/);
      assert.equal(result.assessment, 'incomplete');
      assert.equal(result.gates.find(g => g.name === 'structure').normalization, 'partial');
      assert.equal(structureMeasurement(result, 'module.count'), 3);
      assert.equal(structureMeasurement(result, 'module.edges'), 0);
      assert.equal(structureMeasurement(result, 'module.cycles'), 0);
      assert.equal(result.exit_code, 0);
      assert.equal(blockingOutcome(result), false);
    },
  },
  {
    id: 'structure.out_of_scope_collision_is_ignored',
    capability: 'structure.runtime_graph',
    kind: 'boundary',
    command: 'structure',
    dirs: 'src/other.py',
    files: {
      // pkg.a collides between a module and a package, but neither identity is
      // in the requested graph and other.py imports nothing. The collision is
      // irrelevant to this scope and must not make its evidence incomplete.
      'src/other.py': '',
      'src/pkg/a.py': '',
      'src/pkg/a/__init__.py': '',
    },
    check: ({ result }) => {
      complete(result, 'structure');
      assert.equal(result.assessment, 'complete');
      assert.equal(result.findings.length, 0, JSON.stringify(result.findings));
      assert.equal(structureMeasurement(result, 'module.count'), 1);
      assert.equal(structureMeasurement(result, 'module.edges'), 0);
      assert.equal(structureMeasurement(result, 'module.cycles'), 0);
      assert.equal(structureDepth(result, 'src/other.py'), 0);
      assert.equal(result.exit_code, 0);
      assert.equal(blockingOutcome(result), false);
    },
  },
  {
    id: 'structure.runtime_cycle_with_type_only_imports',
    capability: 'structure.false_positive_controls',
    kind: 'positive',
    command: 'structure',
    files: {
      'src/pkg/__init__.py': '',
      'src/pkg/a.py': 'from typing import TYPE_CHECKING\nfrom .b import B\n\nif TYPE_CHECKING:\n    from .c import C\n',
      'src/pkg/b.py': 'from .a import load\n',
      'src/pkg/c.py': '',
    },
    check: ({ result }) => {
      // Ignoring TYPE_CHECKING imports must not hide a runtime cycle: the
      // type-only edge is counted separately while the a<->b SCC is reported.
      complete(result, 'structure');
      assert.equal(circularFindings(result).length, 1);
      assert.equal(structureMeasurement(result, 'module.cycles'), 1);
      assert.equal(structureMeasurement(result, 'module.edges'), 2);
      assert.equal(structureMeasurement(result, 'module.type_imports'), 1);
      assert.equal(result.exit_code, 0);
      assert.equal(blockingOutcome(result), false);
    },
  },
  {
    id: 'structure.type_only_cycle_control',
    capability: 'structure.false_positive_controls',
    kind: 'negative',
    command: 'structure',
    files: {
      'src/pkg/__init__.py': '',
      'src/pkg/a.py': 'from typing import TYPE_CHECKING\n\nif TYPE_CHECKING:\n    from .b import B\n',
      'src/pkg/b.py': 'from typing import TYPE_CHECKING\n\nif TYPE_CHECKING:\n    from .a import A\n',
    },
    check: ({ result }) => {
      // The same import cycle under TYPE_CHECKING is not a runtime edge:
      // zero edges, exactly two type-only references, and no finding.
      complete(result, 'structure');
      assert.equal(result.findings.length, 0, JSON.stringify(result.findings));
      assert.equal(structureMeasurement(result, 'module.edges'), 0);
      assert.equal(structureMeasurement(result, 'module.type_imports'), 2);
      assert.equal(structureMeasurement(result, 'module.cycles'), 0);
    },
  },
  {
    id: 'structure.external_imports_no_edges',
    capability: 'structure.false_positive_controls',
    kind: 'negative',
    command: 'structure',
    files: {
      'src/pkg/__init__.py': '',
      'src/pkg/a.py': 'import json\nimport os\n\ndef load():\n    return json.dumps({"path": os.sep})\n',
      'src/pkg/b.py': 'import os\n',
    },
    check: ({ result }) => {
      // External/third-party imports are boundary references, never in-scope
      // module edges, so they cannot create a cycle.
      complete(result, 'structure');
      assert.equal(result.findings.length, 0, JSON.stringify(result.findings));
      assert.equal(structureMeasurement(result, 'module.edges'), 0);
      assert.equal(structureMeasurement(result, 'module.cycles'), 0);
      assert.equal(structureMeasurement(result, 'module.count'), 3);
    },
  },
  {
    id: 'structure.external_name_collision_no_cycle',
    capability: 'structure.false_positive_controls',
    kind: 'negative',
    command: 'structure',
    files: {
      // src is the fixed import root, so `import json` is external even though
      // src/vendor/json.py exists. vendor.json stays vendor.json and cannot be
      // promoted into a false pkg.a <-> json cycle.
      'src/pkg/a.py': 'import json\n',
      'src/vendor/json.py': 'import pkg.a\n',
    },
    check: ({ result }) => {
      complete(result, 'structure');
      assert.equal(circularFindings(result).length, 0, JSON.stringify(result.findings));
      assert.equal(structureMeasurement(result, 'module.cycles'), 0);
      assert.equal(structureMeasurement(result, 'module.edges'), 1);
      assert.equal(structureMeasurement(result, 'module.count'), 2);
      assert.equal(result.exit_code, 0);
      assert.equal(blockingOutcome(result), false);
    },
  },
  {
    id: 'structure.relative_import_package_export_control',
    capability: 'structure.false_positive_controls',
    kind: 'negative',
    command: 'structure',
    files: {
      'src/pkg/__init__.py': 'VALUE = 1\n',
      'src/pkg/a.py': 'from . import VALUE\n',
    },
    check: ({ result }) => {
      // A name exported by the package __init__ is a legitimate attribute
      // import, not a missing pkg.VALUE submodule. It stays complete and keeps
      // the edge to the package.
      complete(result, 'structure');
      assert.equal(result.assessment, 'complete');
      assert.equal(result.findings.length, 0, JSON.stringify(result.findings));
      assert.equal(structureMeasurement(result, 'module.count'), 2);
      assert.equal(structureMeasurement(result, 'module.edges'), 1);
      assert.equal(structureMeasurement(result, 'module.cycles'), 0);
      assert.equal(structureDepth(result, 'src/pkg/a.py'), 1);
    },
  },
  {
    id: 'structure.module_getattr_export_control',
    capability: 'structure.false_positive_controls',
    kind: 'negative',
    command: 'structure',
    files: {
      // A module-level __getattr__ can supply any attribute, so an arbitrary
      // name imported from the package is valid rather than unresolved.
      'src/pkg/__init__.py': 'def __getattr__(name):\n    return name\n',
      'src/pkg/worker.py': 'from pkg import plugin\n',
    },
    check: ({ result }) => {
      complete(result, 'structure');
      assert.equal(result.assessment, 'complete');
      assert.equal(result.findings.length, 0, JSON.stringify(result.findings));
      assert.equal(structureMeasurement(result, 'module.count'), 2);
      assert.equal(structureMeasurement(result, 'module.edges'), 1);
      assert.equal(structureMeasurement(result, 'module.cycles'), 0);
      assert.equal(structureDepth(result, 'src/pkg/__init__.py'), 0);
      assert.equal(structureDepth(result, 'src/pkg/worker.py'), 1);
      assert.equal(result.exit_code, 0);
      assert.equal(blockingOutcome(result), false);
    },
  },
  {
    id: 'structure.star_reexport_control',
    capability: 'structure.false_positive_controls',
    kind: 'negative',
    command: 'structure',
    files: {
      // A runtime star re-export makes the package's attribute set dynamic;
      // a name imported from it is valid and the impl edge stays intact.
      'src/pkg/__init__.py': 'from .impl import *\n',
      'src/pkg/impl.py': 'VALUE = 1\n',
      'src/pkg/worker.py': 'from pkg import VALUE\n',
    },
    check: ({ result }) => {
      complete(result, 'structure');
      assert.equal(result.assessment, 'complete');
      assert.equal(result.findings.length, 0, JSON.stringify(result.findings));
      assert.equal(structureMeasurement(result, 'module.count'), 3);
      assert.equal(structureMeasurement(result, 'module.edges'), 2);
      assert.equal(structureMeasurement(result, 'module.cycles'), 0);
      assert.equal(structureDepth(result, 'src/pkg/__init__.py'), 1);
      assert.equal(structureDepth(result, 'src/pkg/impl.py'), 0);
      assert.equal(structureDepth(result, 'src/pkg/worker.py'), 2);
      assert.equal(result.exit_code, 0);
      assert.equal(blockingOutcome(result), false);
    },
  },
  {
    id: 'structure.dynamic_module_exports_control',
    capability: 'structure.false_positive_controls',
    kind: 'negative',
    command: 'structure',
    files: {
      // globals() and setattr() can create package attributes at runtime, so
      // imported names that no static statement binds remain valid.
      'src/pkg/__init__.py': "import sys\n\nglobals()['plugin'] = 1\nsetattr(sys.modules[__name__], 'extension', 2)\n",
      'src/pkg/worker.py': 'from pkg import plugin, extension, arbitrary\n',
    },
    check: ({ result }) => {
      complete(result, 'structure');
      assert.equal(result.assessment, 'complete');
      assert.equal(result.findings.length, 0, JSON.stringify(result.findings));
      assert.equal(structureMeasurement(result, 'module.count'), 2);
      assert.equal(structureMeasurement(result, 'module.edges'), 1);
      assert.equal(structureMeasurement(result, 'module.cycles'), 0);
      assert.equal(structureDepth(result, 'src/pkg/__init__.py'), 0);
      assert.equal(structureDepth(result, 'src/pkg/worker.py'), 1);
      assert.equal(result.exit_code, 0);
      assert.equal(blockingOutcome(result), false);
    },
  },
  {
    id: 'structure.deferred_import_cycle_control',
    capability: 'structure.false_positive_controls',
    kind: 'boundary',
    command: 'structure',
    files: {
      'src/pkg/__init__.py': '',
      'src/pkg/a.py': 'from .b import B\n',
      'src/pkg/b.py': 'def load():\n    from .a import load as imported\n    return imported\n',
    },
    check: ({ result }) => {
      // A function-local import closes a<->b only when it executes, not at
      // import time. It is removed from runtime edges/cycles and counted.
      complete(result, 'structure');
      assert.equal(result.findings.length, 0, JSON.stringify(result.findings));
      assert.equal(structureMeasurement(result, 'module.cycles'), 0);
      assert.equal(structureMeasurement(result, 'module.edges'), 1);
      assert.equal(structureMeasurement(result, 'module.deferred_imports'), 1);
      assert.equal(structureDepth(result, 'src/pkg/a.py'), 1);
      assert.equal(structureDepth(result, 'src/pkg/b.py'), 0);
    },
  },
  {
    id: 'structure.unresolved_local_import_is_partial',
    capability: 'structure.false_positive_controls',
    kind: 'boundary',
    command: 'structure',
    files: {
      'src/pkg/__init__.py': '',
      'src/pkg/a.py': 'from .missing import value\n',
    },
    check: ({ result }) => {
      // A missing module inside the package root is incomplete evidence, not a
      // crash or a clean pass. The not_evaluated advisory leaves exit 0.
      const findings = result.findings.filter(f => f.rule === 'pyguard.structure.not_evaluated');
      assert.equal(findings.length, 1, JSON.stringify(result.findings));
      assert.equal(findings[0].status, 'advisory');
      assert.equal(findings[0].severity, 'info');
      assert.match(findings[0].evidence, /pkg\.missing/);
      assert.equal(result.assessment, 'incomplete');
      assert.equal(result.gates.find(g => g.name === 'structure').normalization, 'partial');
      assert.equal(result.exit_code, 0);
      assert.equal(blockingOutcome(result), false);
    },
  },
  {
    id: 'structure.excluded_import_is_scope_boundary',
    capability: 'structure.false_positive_controls',
    kind: 'boundary',
    command: 'structure',
    pyproject: PYPROJECT + '[tool.pyguard]\nexclude = ["src/pkg/b.py"]\n',
    files: {
      'src/pkg/__init__.py': 'from .a import load\n',
      'src/pkg/a.py': 'import pkg.b\n',
      'src/pkg/b.py': 'VALUE = 1\n',
    },
    check: ({ result }) => {
      // b.py exists but is excluded. Grimp knows the target so it must not
      // fall back to pkg (which would invent a pkg <-> pkg.a cycle), and the
      // excluded target is a scope boundary rather than a missing module.
      complete(result, 'structure');
      assert.equal(result.assessment, 'complete');
      assert.equal(result.findings.length, 0, JSON.stringify(result.findings));
      assert.equal(structureMeasurement(result, 'module.count'), 2);
      assert.equal(structureMeasurement(result, 'module.edges'), 1);
      assert.equal(structureMeasurement(result, 'module.cycles'), 0);
      assert.equal(structureDepth(result, 'src/pkg/__init__.py'), 1);
      assert.equal(structureDepth(result, 'src/pkg/a.py'), 0);
    },
  },
  {
    id: 'structure.relative_import_missing_name_is_partial',
    capability: 'structure.false_positive_controls',
    kind: 'boundary',
    command: 'structure',
    files: {
      'src/pkg/__init__.py': '',
      'src/pkg/a.py': 'from . import missing\n',
    },
    check: ({ result }) => {
      // from . import missing refers to pkg.missing, not to an exported pkg
      // attribute. With an empty __init__ this is missing evidence, not an
      // edge to pkg with a complete assessment.
      const findings = result.findings.filter(f => f.rule === 'pyguard.structure.not_evaluated');
      assert.equal(findings.length, 1, JSON.stringify(result.findings));
      assert.equal(findings[0].status, 'advisory');
      assert.match(findings[0].evidence, /pkg\.missing/);
      assert.equal(circularFindings(result).length, 0);
      assert.equal(structureMeasurement(result, 'module.count'), 2);
      assert.equal(structureMeasurement(result, 'module.edges'), 0);
      assert.equal(structureMeasurement(result, 'module.cycles'), 0);
      assert.equal(result.assessment, 'incomplete');
      assert.equal(result.gates.find(g => g.name === 'structure').normalization, 'partial');
      assert.equal(result.exit_code, 0);
      assert.equal(blockingOutcome(result), false);
    },
  },
  {
    id: 'structure.narrow_scope_import_is_boundary',
    capability: 'structure.false_positive_controls',
    kind: 'boundary',
    command: 'structure',
    dirs: 'src/pkg/a.py',
    files: {
      'src/pkg/__init__.py': '',
      'src/pkg/a.py': 'from .b import B\n',
      'src/pkg/b.py': 'B = 1\n',
    },
    check: ({ result }) => {
      // b.py is outside the requested graph scope but exists under the fixed
      // import root. It is a boundary, not an unresolved module; no edge is
      // invented for the out-of-scope sibling.
      complete(result, 'structure');
      assert.equal(result.assessment, 'complete');
      assert.equal(result.findings.length, 0, JSON.stringify(result.findings));
      assert.equal(structureMeasurement(result, 'module.count'), 1);
      assert.equal(structureMeasurement(result, 'module.edges'), 0);
      assert.equal(structureMeasurement(result, 'module.cycles'), 0);
      assert.equal(structureDepth(result, 'src/pkg/a.py'), 0);
    },
  },
  {
    id: 'structure.narrow_scope_missing_child_is_partial',
    capability: 'structure.false_positive_controls',
    kind: 'boundary',
    command: 'structure',
    dirs: 'src/pkg/a.py',
    files: {
      'src/pkg/__init__.py': '',
      'src/pkg/a.py': 'from . import missing\n',
    },
    check: ({ result }) => {
      // An existing parent package does not prove that the requested child
      // exists. The child is validated against the full inventory before the
      // narrow graph scope makes the parent a boundary.
      const findings = result.findings.filter(f => f.rule === 'pyguard.structure.not_evaluated');
      assert.equal(findings.length, 1, JSON.stringify(result.findings));
      assert.equal(findings[0].status, 'advisory');
      assert.match(findings[0].evidence, /pkg\.missing/);
      assert.equal(circularFindings(result).length, 0);
      assert.equal(result.assessment, 'incomplete');
      assert.equal(result.gates.find(g => g.name === 'structure').normalization, 'partial');
      assert.equal(structureMeasurement(result, 'module.count'), 1);
      assert.equal(structureMeasurement(result, 'module.edges'), 0);
      assert.equal(structureMeasurement(result, 'module.cycles'), 0);
      assert.equal(structureDepth(result, 'src/pkg/a.py'), 0);
      assert.equal(result.exit_code, 0);
      assert.equal(blockingOutcome(result), false);
    },
  },
  {
    id: 'structure.invalid_relative_import_is_partial',
    capability: 'structure.false_positive_controls',
    kind: 'boundary',
    command: 'structure',
    files: {
      'src/pkg/__init__.py': '',
      'src/pkg/a.py': 'from ...missing import X\n',
    },
    check: ({ result }) => {
      // This relative level is beyond the top-level package. It must be
      // classified as unresolved before grimp sees it: passing it through
      // would let grimp 3.18 panic and turn incomplete source evidence into an
      // execution failure.
      const findings = result.findings.filter(f => f.rule === 'pyguard.structure.not_evaluated');
      assert.equal(findings.length, 1, JSON.stringify(result.findings));
      assert.equal(findings[0].status, 'advisory');
      assert.match(findings[0].evidence, /<relative>/);
      assert.equal(circularFindings(result).length, 0);
      assert.equal(result.assessment, 'incomplete');
      assert.equal(result.gates.find(g => g.name === 'structure').normalization, 'partial');
      assert.equal(structureMeasurement(result, 'module.count'), 2);
      assert.equal(structureMeasurement(result, 'module.edges'), 0);
      assert.equal(structureMeasurement(result, 'module.cycles'), 0);
      assert.equal(result.exit_code, 0);
      assert.equal(blockingOutcome(result), false);
    },
  },
  {
    id: 'structure.type_only_package_export_is_partial',
    capability: 'structure.false_positive_controls',
    kind: 'boundary',
    command: 'structure',
    files: {
      // Foo exists only under TYPE_CHECKING in the package __init__, so it is
      // not a runtime package attribute. `from pkg import Foo` in b.py is
      // therefore unresolved and the evidence is incomplete, not a cycle.
      'src/pkg/__init__.py': 'from typing import TYPE_CHECKING\n\nif TYPE_CHECKING:\n    from .a import Foo\n\nimport pkg.b\n',
      'src/pkg/a.py': 'class Foo:\n    pass\n',
      'src/pkg/b.py': 'from pkg import Foo\n',
    },
    check: ({ result }) => {
      assert.equal(result.assessment, 'incomplete');
      const findings = result.findings.filter(f => f.rule === 'pyguard.structure.not_evaluated');
      assert.equal(findings.length, 1, JSON.stringify(result.findings));
      assert.equal(findings[0].status, 'advisory');
      assert.equal(findings[0].severity, 'info');
      assert.match(findings[0].evidence, /pkg\.Foo/);
      assert.equal(circularFindings(result).length, 0);
      assert.equal(structureMeasurement(result, 'module.count'), 3);
      assert.equal(structureMeasurement(result, 'module.edges'), 1);
      assert.equal(structureMeasurement(result, 'module.cycles'), 0);
      assert.equal(structureMeasurement(result, 'module.type_imports'), 1);
      assert.equal(result.exit_code, 0);
      assert.equal(blockingOutcome(result), false);
    },
  },
  {
    id: 'structure.annotation_only_export_is_partial',
    capability: 'structure.false_positive_controls',
    kind: 'boundary',
    command: 'structure',
    files: {
      // client is annotated but never assigned in __init__, so it is not a
      // runtime package attribute. `from pkg import client` in worker.py is
      // therefore unresolved; it must not close a false pkg <-> pkg.worker
      // cycle just because the annotation mentions the name.
      'src/pkg/__init__.py': 'client: Client\n\nimport pkg.worker\n',
      'src/pkg/worker.py': 'from pkg import client\n',
    },
    check: ({ result }) => {
      assert.equal(result.assessment, 'incomplete');
      const findings = result.findings.filter(f => f.rule === 'pyguard.structure.not_evaluated');
      assert.equal(findings.length, 1, JSON.stringify(result.findings));
      assert.equal(findings[0].status, 'advisory');
      assert.equal(findings[0].severity, 'info');
      assert.match(findings[0].evidence, /pkg\.client/);
      assert.equal(circularFindings(result).length, 0);
      assert.equal(structureMeasurement(result, 'module.count'), 2);
      assert.equal(structureMeasurement(result, 'module.edges'), 1);
      assert.equal(structureMeasurement(result, 'module.cycles'), 0);
      assert.equal(structureDepth(result, 'src/pkg/__init__.py'), 1);
      assert.equal(structureDepth(result, 'src/pkg/worker.py'), 0);
      assert.equal(result.exit_code, 0);
      assert.equal(blockingOutcome(result), false);
    },
  },
  {
    id: 'criticality.distinct_callers_cross_module',
    capability: 'structure.function_graph',
    kind: 'positive',
    files: {
      'src/core.py': 'def target():\n    return 1\n',
      'src/caller_one.py': 'import core\n\n\ndef one():\n    core.target()\n    core.target()\n    return core.target()\n',
      'src/caller_two.py': 'import core\n\n\ndef two():\n    return core.target()\n',
      'src/caller_three.py': 'import core\n\n\ndef three():\n    return core.target()\n',
    },
    check: ({ result }) => {
      // Three distinct callers across three modules rank target at exactly 3;
      // one caller repeating the call three times still counts once.
      complete(result);
      assert.equal(inDegree(result, 'src/core.py', 'core.target'), 3, 'criticality.in_degree for core.target');
      assert.equal(ranked(result, 'core.target').observed, 3, 'ranked criticality finding for core.target');
      assert.equal(inDegree(result, 'src/caller_one.py', 'caller_one.one'), 0, 'criticality.in_degree for caller_one.one');
    },
  },
  {
    id: 'criticality.uncalled_function_zero',
    capability: 'structure.function_graph',
    kind: 'negative',
    files: { 'src/lonely.py': 'def never_called():\n    return 1\n' },
    check: ({ result }) => {
      // A function nobody calls stays at in_degree 0 and never enters the
      // ranked findings; silence must not be fabricated into a caller count.
      complete(result);
      assert.equal(inDegree(result, 'src/lonely.py', 'lonely.never_called'), 0, 'criticality.in_degree for lonely.never_called');
      assert.equal(ranked(result, 'lonely.never_called'), undefined, 'ranked criticality finding for lonely.never_called');
      assert.equal(result.findings.length, 0);
    },
  },
  {
    id: 'criticality.same_caller_not_inflated',
    capability: 'structure.function_graph',
    kind: 'negative',
    files: {
      'src/hub.py': 'def hub():\n    return 1\n',
      'src/busy.py': 'import hub\n\n\ndef busy():\n    hub.hub()\n    hub.hub()\n    hub.hub()\n    return hub.hub()\n',
    },
    check: ({ result }) => {
      // Four calls from one caller are one distinct caller, not four.
      complete(result);
      assert.equal(inDegree(result, 'src/hub.py', 'hub.hub'), 1, 'criticality.in_degree for hub.hub');
      assert.equal(ranked(result, 'hub.hub').observed, 1, 'ranked criticality finding for hub.hub');
      assert.equal(inDegree(result, 'src/busy.py', 'busy.busy'), 0, 'criticality.in_degree for busy.busy');
    },
  },
  {
    id: 'criticality.self_recursion_counts_once',
    capability: 'structure.function_graph',
    kind: 'boundary',
    files: { 'src/recursion.py': 'def countdown(n):\n    if n <= 0:\n        return 0\n    return countdown(n - 1)\n' },
    check: ({ result }) => {
      // Documented boundary: a self-call is one defined caller, so recursion
      // scores exactly 1 — neither 0 (the function does call itself) nor a
      // value that grows with recursion depth or repeated self-calls.
      complete(result);
      assert.equal(inDegree(result, 'src/recursion.py', 'recursion.countdown'), 1, 'criticality.in_degree for recursion.countdown');
      assert.equal(ranked(result, 'recursion.countdown').observed, 1, 'ranked criticality finding for recursion.countdown');
    },
  },
  {
    id: 'structure.coupling_hub_threshold',
    capability: 'structure.coupling',
    kind: 'positive',
    command: 'structure',
    files: couplingFixture(),
    check: ({ result }) => {
      // Exactly three in-scope importers and eight in-scope runtime
      // dependencies meet the joint review threshold. The finding is advisory
      // and never changes the exit code or blocking outcome.
      complete(result, 'structure');
      const findings = couplingFindings(result);
      assert.equal(findings.length, 1, JSON.stringify(result.findings));
      const finding = findings[0];
      assert.equal(finding.level, 'structure');
      assert.equal(finding.gate, 'structure');
      assert.equal(finding.severity, 'warning');
      assert.equal(finding.status, 'advisory');
      assert.equal(finding.category, 'quality');
      assert.equal(finding.location.file, 'src/pkg/hub.py');
      assert.equal(finding.evidence, '3 importing modules depend on this module, which imports 8 in-scope runtime modules.');
      assert.match(finding.remediation, /do not prove excessive coupling/);
      const fanIn = moduleMeasurement(result, 'module.fan_in', 'src/pkg/hub.py');
      const fanOut = moduleMeasurement(result, 'module.fan_out', 'src/pkg/hub.py');
      assert.equal(fanIn.value, 3);
      assert.equal(fanIn.unit, 'modules');
      assert.equal(fanIn.level, 'structure');
      assert.equal(fanOut.value, 8);
      assert.equal(fanOut.unit, 'modules');
      assert.equal(fanOut.level, 'structure');
      // Fan counts are per module and directional: an importer has fan-out 1
      // and no importers, while a dependency has fan-in 1 and no dependencies.
      assert.equal(moduleMeasurement(result, 'module.fan_in', 'src/pkg/caller0.py').value, 0);
      assert.equal(moduleMeasurement(result, 'module.fan_out', 'src/pkg/caller0.py').value, 1);
      assert.equal(moduleMeasurement(result, 'module.fan_in', 'src/pkg/leaf0.py').value, 1);
      assert.equal(moduleMeasurement(result, 'module.fan_out', 'src/pkg/leaf0.py').value, 0);
      assert.equal(structureMeasurement(result, 'module.count'), 13);
      assert.equal(structureMeasurement(result, 'module.edges'), 11);
      assert.equal(structureMeasurement(result, 'module.cycles'), 0);
      assert.equal(result.exit_code, 0);
      assert.equal(blockingOutcome(result), false);
    },
  },
  {
    id: 'structure.coupling_below_importer_threshold',
    capability: 'structure.coupling',
    kind: 'negative',
    command: 'structure',
    files: couplingFixture({ importers: 2 }),
    check: ({ result }) => {
      // Eight dependencies alone are not a hotspot: the joint threshold also
      // requires at least three importers.
      complete(result, 'structure');
      assert.equal(result.findings.length, 0, JSON.stringify(result.findings));
      assert.equal(moduleMeasurement(result, 'module.fan_in', 'src/pkg/hub.py').value, 2);
      assert.equal(moduleMeasurement(result, 'module.fan_out', 'src/pkg/hub.py').value, 8);
      assert.equal(structureMeasurement(result, 'module.count'), 12);
      assert.equal(structureMeasurement(result, 'module.edges'), 10);
      assert.equal(result.exit_code, 0);
      assert.equal(blockingOutcome(result), false);
    },
  },
  {
    id: 'structure.coupling_below_dependency_threshold',
    capability: 'structure.coupling',
    kind: 'negative',
    command: 'structure',
    files: couplingFixture({ dependencies: 7 }),
    check: ({ result }) => {
      // Three importers alone are not a hotspot either: seven dependencies
      // stay one below the fixed fan-out threshold.
      complete(result, 'structure');
      assert.equal(result.findings.length, 0, JSON.stringify(result.findings));
      assert.equal(moduleMeasurement(result, 'module.fan_in', 'src/pkg/hub.py').value, 3);
      assert.equal(moduleMeasurement(result, 'module.fan_out', 'src/pkg/hub.py').value, 7);
      assert.equal(structureMeasurement(result, 'module.count'), 12);
      assert.equal(structureMeasurement(result, 'module.edges'), 10);
      assert.equal(result.exit_code, 0);
      assert.equal(blockingOutcome(result), false);
    },
  },
  {
    id: 'structure.coupling_type_only_dependency_control',
    capability: 'structure.coupling',
    kind: 'boundary',
    command: 'structure',
    files: couplingFixture({ eighth: 'type' }),
    check: ({ result }) => {
      // The eighth import is under TYPE_CHECKING: it is counted as a type
      // import, creates no runtime edge and leaves fan-out at seven.
      complete(result, 'structure');
      assert.equal(result.findings.length, 0, JSON.stringify(result.findings));
      assert.equal(moduleMeasurement(result, 'module.fan_in', 'src/pkg/hub.py').value, 3);
      assert.equal(moduleMeasurement(result, 'module.fan_out', 'src/pkg/hub.py').value, 7);
      assert.equal(structureMeasurement(result, 'module.type_imports'), 1);
      assert.equal(structureMeasurement(result, 'module.edges'), 10);
      assert.equal(result.exit_code, 0);
      assert.equal(blockingOutcome(result), false);
    },
  },
  {
    id: 'structure.coupling_deferred_dependency_control',
    capability: 'structure.coupling',
    kind: 'boundary',
    command: 'structure',
    files: couplingFixture({ eighth: 'deferred' }),
    check: ({ result }) => {
      // A function-local import executes only when the function runs; it is
      // counted as deferred and leaves the runtime fan-out at seven.
      complete(result, 'structure');
      assert.equal(result.findings.length, 0, JSON.stringify(result.findings));
      assert.equal(moduleMeasurement(result, 'module.fan_in', 'src/pkg/hub.py').value, 3);
      assert.equal(moduleMeasurement(result, 'module.fan_out', 'src/pkg/hub.py').value, 7);
      assert.equal(structureMeasurement(result, 'module.deferred_imports'), 1);
      assert.equal(structureMeasurement(result, 'module.edges'), 10);
      assert.equal(result.exit_code, 0);
      assert.equal(blockingOutcome(result), false);
    },
  },
  {
    id: 'structure.coupling_external_dependency_control',
    capability: 'structure.coupling',
    kind: 'boundary',
    command: 'structure',
    files: couplingFixture({ dependencies: 7, eighth: 'external' }),
    check: ({ result }) => {
      // The eighth import is external (json). It is not an in-scope runtime
      // dependency, so the hub has seven dependencies and no finding.
      complete(result, 'structure');
      assert.equal(result.findings.length, 0, JSON.stringify(result.findings));
      assert.equal(moduleMeasurement(result, 'module.fan_in', 'src/pkg/hub.py').value, 3);
      assert.equal(moduleMeasurement(result, 'module.fan_out', 'src/pkg/hub.py').value, 7);
      assert.equal(structureMeasurement(result, 'module.type_imports'), 0);
      assert.equal(structureMeasurement(result, 'module.edges'), 10);
      assert.equal(result.exit_code, 0);
      assert.equal(blockingOutcome(result), false);
    },
  },
  {
    id: 'structure.coupling_excluded_modules_boundary',
    capability: 'structure.coupling',
    kind: 'boundary',
    command: 'structure',
    pyproject: PYPROJECT + '[tool.pyguard]\nexclude = ["src/pkg/caller2.py", "src/pkg/leaf7.py"]\n',
    files: couplingFixture(),
    check: ({ result }) => {
      // One importer and one dependency are excluded by project configuration.
      // They are scope boundaries: no edge is inferred and neither count
      // reaches the joint threshold. Their files get no module measurements.
      complete(result, 'structure');
      assert.equal(result.findings.length, 0, JSON.stringify(result.findings));
      assert.equal(moduleMeasurement(result, 'module.fan_in', 'src/pkg/hub.py').value, 2);
      assert.equal(moduleMeasurement(result, 'module.fan_out', 'src/pkg/hub.py').value, 7);
      assert.equal(findModuleMeasurement(result, 'module.fan_in', 'src/pkg/caller2.py'), undefined);
      assert.equal(findModuleMeasurement(result, 'module.fan_in', 'src/pkg/leaf7.py'), undefined);
      assert.equal(structureMeasurement(result, 'module.count'), 11);
      assert.equal(structureMeasurement(result, 'module.edges'), 9);
      assert.equal(result.exit_code, 0);
      assert.equal(blockingOutcome(result), false);
    },
  },
  {
    id: 'structure.coupling_test_file_boundary',
    capability: 'structure.coupling',
    kind: 'boundary',
    command: 'structure',
    files: (() => {
      const files = couplingFixture({ importers: 2, dependencies: 7 });
      files['src/pkg/tests/__init__.py'] = '';
      files['src/pkg/tests/test_helper.py'] = 'VALUE = 0\n';
      files['src/pkg/tests/test_consumer.py'] = 'from ..hub import VALUE0\n';
      files['src/pkg/hub.py'] += 'from .tests.test_helper import VALUE as TEST_VALUE\n';
      return files;
    })(),
    check: ({ result }) => {
      // The project nominally has three importers and eight dependencies, but
      // the third importer and eighth dependency are conventional test files.
      // Test exclusions leave the hub at 2/7 with no finding or measurements
      // for the excluded files.
      complete(result, 'structure');
      assert.equal(result.findings.length, 0, JSON.stringify(result.findings));
      assert.equal(moduleMeasurement(result, 'module.fan_in', 'src/pkg/hub.py').value, 2);
      assert.equal(moduleMeasurement(result, 'module.fan_out', 'src/pkg/hub.py').value, 7);
      assert.equal(findModuleMeasurement(result, 'module.fan_in', 'src/pkg/tests/test_consumer.py'), undefined);
      assert.equal(findModuleMeasurement(result, 'module.fan_in', 'src/pkg/tests/test_helper.py'), undefined);
      assert.equal(structureMeasurement(result, 'module.count'), 11);
      assert.equal(structureMeasurement(result, 'module.edges'), 9);
      assert.equal(result.exit_code, 0);
      assert.equal(blockingOutcome(result), false);
    },
  },
  {
    id: 'complexipy.cognitive.exceeded_is_advisory_warning',
    capability: 'quality.cognitive_complexity',
    kind: 'positive',
    command: 'radon',
    files: { 'src/complexity.py': flatFunction + '\n\n' + nestChain(6, 'above') },
    baselineFiles: { 'src/clean.py': flatFunction },
    check: ({ result, baseline }) => {
      // complexipy 8.3.0 scores the chain-of-six function exactly 21 (>20).
      // The warning must never change the exit code or the gate/overall
      // pass-fail status versus a clean project: both runs exit 0 and neither
      // the run nor the gate fails (blockingOutcome is false for both). The
      // result contract records advisory findings as status 'advisory' — the
      // only difference a warning may make, never 'fail'. pyguard has no
      // strict flag or config, so advisory behavior is unconditional.
      complete(result, 'radon');
      const warning = cognitiveWarning(result, 'above');
      assert.ok(warning, `missing cognitive warning; findings: ${JSON.stringify(result.findings)}`);
      assert.equal(warning.severity, 'warning');
      assert.equal(warning.status, 'advisory');
      assert.equal(warning.gate, 'radon');
      assert.equal(warning.category, 'quality');
      assert.equal(warning.observed, 21);
      assert.equal(warning.threshold, 20);
      assert.equal(warning.location.file, 'src/complexity.py');
      const measurement = cognitiveMeasurement(result, 'above');
      assert.ok(measurement, 'missing complexipy.cognitive measurement');
      assert.equal(measurement.value, 21);
      assert.equal(measurement.level, 'code');
      assert.equal(measurement.unit, 'score');
      assert.equal(measurement.threshold, 20);
      assert.ok(cognitiveMeasurement(result, 'flat'), 'per-function measurement for every function');
      // Clean baseline project through the same command.
      assert.equal(baseline.assessment, 'complete');
      assert.equal(baseline.exit_code, 0);
      assert.equal(baseline.status, 'pass');
      assert.equal(baseline.findings.length, 0);
      assert.equal(result.exit_code, 0);
      assert.equal(result.exit_code, baseline.exit_code, 'warning changed the exit code');
      assert.equal(blockingOutcome(result), blockingOutcome(baseline), 'warning changed the gate/overall outcome');
      assert.equal(blockingOutcome(result), false);
      assert.equal(result.status, 'advisory');
      assert.equal(result.gates.find(g => g.name === 'radon').status, 'advisory');
    },
  },
  {
    id: 'complexipy.cognitive.flat_function_no_finding',
    capability: 'quality.cognitive_complexity',
    kind: 'negative',
    command: 'radon',
    files: { 'src/flat.py': flatFunction },
    check: ({ result }) => {
      // A flat function scores 0: a measurement is emitted, no warning, and
      // the run is a clean pass exactly like a project without complexity.
      complete(result, 'radon');
      assert.equal(result.status, 'pass');
      assert.equal(result.exit_code, 0);
      assert.equal(result.findings.length, 0, JSON.stringify(result.findings));
      const measurement = cognitiveMeasurement(result, 'flat');
      assert.ok(measurement, 'missing complexipy.cognitive measurement');
      assert.equal(measurement.value, 0);
      assert.equal(measurement.threshold, 20);
    },
  },
  {
    id: 'complexipy.cognitive.nested_never_blocking',
    capability: 'quality.cognitive_complexity',
    kind: 'negative',
    command: 'radon',
    files: { 'src/nested.py': nestChain(8, 'deeply_nested') },
    check: ({ result }) => {
      // complexipy scores the deep chain 36 — far above the threshold — yet
      // the warning advisory never blocks: exit 0, no fail/error anywhere and
      // no blocking finding, no matter how extreme the score.
      complete(result, 'radon');
      const warning = cognitiveWarning(result, 'deeply_nested');
      assert.ok(warning, `missing cognitive warning; findings: ${JSON.stringify(result.findings)}`);
      assert.equal(warning.observed, 36);
      assert.equal(warning.severity, 'warning');
      assert.equal(warning.status, 'advisory');
      assert.equal(result.exit_code, 0);
      assert.equal(blockingOutcome(result), false);
      for (const f of result.findings) assert.notEqual(f.status, 'blocking');
      assert.equal(cognitiveMeasurement(result, 'deeply_nested').value, 36);
    },
  },
  {
    id: 'complexipy.cognitive.test_files_excluded',
    capability: 'quality.cognitive_complexity',
    kind: 'negative',
    command: 'radon',
    files: {
      'src/main.py': flatFunction,
      'src/test_root.py': nestChain(6, 'test_root'),
      'src/root_test.py': nestChain(6, 'root_test'),
      'src/conftest.py': nestChain(6, 'conftest_root'),
      'src/tests/test_nested.py': nestChain(6, 'test_in_tests'),
      'src/test/test_nested.py': nestChain(6, 'test_in_test'),
      'src/pkg/tests/test_nested.py': nestChain(6, 'test_in_pkg_tests'),
      'src/pkg/test/test_nested.py': nestChain(6, 'test_in_pkg_test'),
    },
    check: ({ result }) => {
      // Root-level test_*.py, *_test.py, conftest.py and whole tests/test
      // directories at any depth are outside complexipy's advisory scope. Each
      // excluded fixture scores 21, so a leak would add measurements/warnings;
      // only the clean src/main.py function may remain.
      complete(result, 'radon');
      assert.equal(result.exit_code, 0);
      assert.equal(blockingOutcome(result), false);
      assert.ok(cognitiveMeasurement(result, 'flat'), 'clean non-test function must be measured');
      assert.equal(result.measurements.filter(m => m.metric === 'complexipy.cognitive').length, 1, JSON.stringify(result.measurements));
      for (const symbol of ['test_root', 'root_test', 'conftest_root', 'test_in_tests', 'test_in_test', 'test_in_pkg_tests', 'test_in_pkg_test']) {
        assert.equal(cognitiveMeasurement(result, symbol), undefined, `${symbol} must be excluded from measurements`);
        assert.equal(cognitiveWarning(result, symbol), undefined, `${symbol} must be excluded from warnings`);
      }
      assert.equal(result.findings.filter(f => f.rule === 'pyguard.complexipy.cognitive_exceeded').length, 0);
    },
  },
  {
    id: 'complexipy.cognitive.nested_function_folds_into_outer',
    capability: 'quality.cognitive_complexity',
    kind: 'boundary',
    command: 'radon',
    files: { 'src/nested_function.py': nestedFunctionChain(8, 'outer', 'inner') },
    check: ({ result }) => {
      // complexipy reports only the enclosing function and folds the inner
      // chain (36) into it. pyguard keeps exactly that one outer measurement
      // and one advisory warning; it never invents an inner symbol or blocks.
      complete(result, 'radon');
      const measurements = result.measurements.filter(m => m.metric === 'complexipy.cognitive');
      assert.equal(measurements.length, 1, JSON.stringify(measurements));
      assert.equal(measurements[0].location.symbol, 'outer');
      assert.equal(measurements[0].value, 36);
      assert.equal(cognitiveMeasurement(result, 'inner'), undefined, 'nested function must not get a separate measurement');
      const warning = cognitiveWarning(result, 'outer');
      assert.ok(warning, `missing outer warning; findings: ${JSON.stringify(result.findings)}`);
      assert.equal(warning.observed, 36);
      assert.equal(warning.status, 'advisory');
      assert.equal(cognitiveWarning(result, 'inner'), undefined, 'nested function must not get a separate warning');
      assert.equal(result.exit_code, 0);
      assert.equal(blockingOutcome(result), false);
      for (const f of result.findings) assert.notEqual(f.status, 'blocking');
    },
  },
  {
    id: 'complexipy.cognitive.threshold_boundary',
    capability: 'quality.cognitive_complexity',
    kind: 'boundary',
    command: 'radon',
    files: { 'src/boundary.py': twoChains('boundary') + '\n\n' + nestChain(6, 'above') },
    check: ({ result }) => {
      // Exactly 20 is clean and 21 warns: both functions are measured, only
      // the one strictly above the threshold yields a finding.
      complete(result, 'radon');
      const at = cognitiveMeasurement(result, 'boundary');
      const over = cognitiveMeasurement(result, 'above');
      assert.ok(at && over, 'both functions must be measured');
      assert.equal(at.value, 20);
      assert.equal(over.value, 21);
      assert.equal(cognitiveWarning(result, 'boundary'), undefined, 'exactly 20 must not warn');
      const warning = cognitiveWarning(result, 'above');
      assert.ok(warning, `21 must warn; findings: ${JSON.stringify(result.findings)}`);
      assert.equal(result.findings.filter(f => f.rule === 'pyguard.complexipy.cognitive_exceeded').length, 1);
      assert.equal(result.exit_code, 0);
      assert.equal(blockingOutcome(result), false);
    },
  },
];

// A .cjs/.mjs/.js "binary" is a Node stand-in used by tests so CLI exit codes
// can be exercised on every platform; real pyguard binaries run directly.
function launcher(binary) {
  return /\.(?:c|m)js$/i.test(binary) ? [process.execPath, binary] : [binary];
}

function spawnInvoke({ binary, python }) {
  const prefix = launcher(binary);
  const env = {
    ...process.env,
    PATH: `${path.dirname(python)}${path.delimiter}${process.env.PATH}`,
    VIRTUAL_ENV: path.dirname(path.dirname(python)),
    UV_NO_SYNC: '1', UV_OFFLINE: '1', NO_COLOR: '1',
  };
  return ({ project, args }) => {
    const response = spawnSync(prefix[0], [...prefix.slice(1), ...args], {
      cwd: project, env, encoding: 'utf8', timeout: 60000, maxBuffer: 8 * 1024 * 1024,
    });
    return {
      status: response.status,
      stdout: response.stdout || '',
      stderr: response.stderr || '',
      error: response.error ? response.error.message : undefined,
    };
  };
}

function project(definition, base, variant = 'project') {
  const dir = path.join(base, definition.id, variant);
  fs.mkdirSync(path.join(dir, 'src'), { recursive: true });
  fs.mkdirSync(path.join(dir, 'tools'), { recursive: true });
  fs.writeFileSync(path.join(dir, 'pyproject.toml'), definition.pyproject || PYPROJECT);
  fs.cpSync(ANALYSIS, path.join(dir, 'tools', 'analysis'), { recursive: true });
  for (const [file, text] of Object.entries(definition.files)) {
    fs.mkdirSync(path.dirname(path.join(dir, file)), { recursive: true });
    fs.writeFileSync(path.join(dir, file), text);
  }
  return dir;
}

function evaluate({ binary, python, root = os.tmpdir(), invoke } = {}) {
  const execute = invoke || spawnInvoke({ binary, python });
  const base = fs.realpathSync(fs.mkdtempSync(path.join(root, 'pyguard-maintainability-')));
  const cases = [];
  try {
    for (const definition of definitions) {
      const start = performance.now();
      let outcome = 'pass';
      let error;
      const dirs = Array.isArray(definition.dirs) ? definition.dirs.join(',') : definition.dirs || 'src';
      const args = [definition.command || 'criticality', '--dirs', dirs, '--output', 'json', '--allow-pipe', '--timeout', '30'];
      try {
        const response = execute({ project: project(definition, base), args });
        assert.ok(!response.error, `pyguard could not be started: ${response.error}`);
        const expectedExit = definition.exitCode ?? 0;
        assert.equal(response.status, expectedExit, `pyguard ${args[0]} exited with ${response.status}, expected ${expectedExit}: ${String(response.stderr || '').slice(0, 1024)}`);
        const result = JSON.parse(response.stdout);
        let baseline;
        if (definition.baselineFiles) {
          const clean = execute({ project: project({ id: definition.id, files: definition.baselineFiles }, base, 'baseline'), args });
          assert.ok(!clean.error, `pyguard baseline could not be started: ${clean.error}`);
          assert.equal(clean.status, 0, `clean baseline exited with ${clean.status}: ${String(clean.stderr || '').slice(0, 1024)}`);
          baseline = JSON.parse(clean.stdout);
        }
        definition.check({ result, baseline });
      } catch (failure) {
        outcome = 'fail';
        error = String((failure && failure.message) || failure);
      }
      const entry = { id: definition.id, capability: definition.capability, kind: definition.kind, outcome, duration_ms: Math.round(performance.now() - start) };
      if (error) entry.error = error;
      cases.push(entry);
    }
    return { schema_version: '1', suite: 'maintainability.python', evaluator_sha256: EVALUATOR_SHA256, cases };
  } finally {
    fs.rmSync(base, { recursive: true, force: true });
  }
}

if (require.main === module) {
  const options = {};
  for (let i = 2; i < process.argv.length; i += 2) {
    const flag = process.argv[i];
    if (!['--binary', '--python', '--output', '--root'].includes(flag) || !process.argv[i + 1] || process.argv[i + 1].startsWith('--')) throw new Error(`Invalid argument ${flag}`);
    options[flag.slice(2)] = process.argv[i + 1];
  }
  if (!options.binary || !options.python || !options.output) throw new Error('Usage: node evals/python/maintainability.cjs --binary <pyguard> --python <python> --output <report.json> [--root <dir>]');
  // Resolve against the invocation cwd (like --python), so a relative
  // --binary survives the spawn inside the temporary fixture project.
  const report = evaluate({ binary: path.resolve(options.binary), python: path.resolve(options.python), root: options.root ? path.resolve(options.root) : os.tmpdir() });
  const output = path.resolve(options.output);
  fs.mkdirSync(path.dirname(output), { recursive: true });
  fs.writeFileSync(output, JSON.stringify(report, null, 2) + '\n');
  const failed = report.cases.filter(c => c.outcome === 'fail');
  console.log(JSON.stringify({ suite: report.suite, passed: report.cases.length - failed.length, failed: failed.length, failures: failed.map(c => ({ id: c.id, error: c.error })) }, null, 2));
  process.exitCode = failed.length ? 1 : 0;
}

module.exports = { evaluate, definitions };
