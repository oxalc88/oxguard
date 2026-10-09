'use strict';
// Adversarial cases run through the packed, installed native CLI. No LLM and no
// production fixture execution are involved in ordinary guard invocations.
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const { spawnSync } = require('node:child_process');
const { performance } = require('node:perf_hooks');
const crypto = require('node:crypto');

function evaluate({ execute, structured, root }) {
  const directory = path.join(root, 'maintainability-evals');
  fs.mkdirSync(directory);
  const cases = [], timings = [];
  const source = (name, files) => {
    const dir = path.join(directory, name);
    fs.mkdirSync(dir);
    for (const [file, text] of Object.entries(files)) {
      fs.mkdirSync(path.dirname(path.join(dir, file)), { recursive: true });
      fs.writeFileSync(path.join(dir, file), text);
    }
    return path.relative(root, dir).replaceAll('\\', '/');
  };
  const run = (id, check) => {
    const start = performance.now();
    check();
    const duration_ms = Math.round(performance.now() - start);
    cases.push({ id, outcome: 'pass', duration_ms });
    timings.push(duration_ms);
  };
  const rules = result => result.findings.map(f => f.rule);
  const args = (command, dir, extra = []) => [command, '--dirs', dir, ...extra];
  try {
    const unsafe = source('typed', { 'source.ts': 'export const value: number = JSON.parse("1");\n' });
    run('baseline.no_config', () => {
      execute(args('fix', unsafe));
      const result = structured(args('lint', unsafe));
      assert.ok(result.measurements.some(m => m.metric === 'lint.baseline_version' && m.value === 1));
      assert.equal(fs.existsSync(path.join(root, 'biome.json')), false);
      assert.equal(fs.existsSync(path.join(root, 'tsconfig.json')), false);
    });
    run('baseline.single_linter', () => {
      const removed = structured(args('typed-lint', unsafe), 3);
      assert.equal(removed.status, 'error');
      assert.equal(removed.findings[0].category, 'invalid_configuration');
      // Compilation of JSON.parse into number is not proof of safe assignment.
      const result = structured(args('types', unsafe));
      assert.equal(result.status, 'pass');
    });
    const emptyCatch = source('empty-catch', { 'source.ts': 'export function swallow() { try { JSON.parse("bad"); } catch {} }\n' });
    run('baseline.empty_catch', () => {
      execute(args('fix', emptyCatch), 1);
      assert.ok(rules(structured(args('lint', emptyCatch), 1)).includes('lint/suspicious/noEmptyBlockStatements'));
    });
    run('baseline.project_overrides_preserved', () => {
      const custom = '{"formatter":{"enabled":false},"assist":{"enabled":false},"linter":{"rules":{"recommended":false,"suspicious":{"noExplicitAny":"off"}}}}';
      fs.writeFileSync(path.join(root, 'biome.json'), custom);
      const explicitAny = source('explicit-any', { 'source.ts': 'export const value: any = 1;\n' });
      try {
        const result = structured(args('lint', explicitAny));
        assert.ok(!result.measurements.some(m => m.metric === 'lint.baseline_version'));
        assert.equal(fs.readFileSync(path.join(root, 'biome.json'), 'utf8'), custom);
      } finally { fs.rmSync(path.join(root, 'biome.json')); }
    });
    run('baseline.external_policy_preserved', () => {
      const custom = 'export default [{ files: ["**/*.js"], rules: { "no-debugger": "error" } }];\n';
      fs.writeFileSync(path.join(root,'eslint.config.mjs'),custom);
      const dir=source('eslint-policy',{'source.js':'debugger;\n'});
      try {
        const result=structured(args('lint',dir),1);
        assert.ok(!result.measurements.some(m=>m.metric==='lint.baseline_version'));
        assert.equal(result.assessment,'incomplete'); // upstream ESLint report is not normalized
        assert.equal(fs.readFileSync(path.join(root,'eslint.config.mjs'),'utf8'),custom);
      } finally {fs.rmSync(path.join(root,'eslint.config.mjs'));}
    });
    const chain = source('chain', {
      'real.ts': 'export function real(x: number): number { return x * 2; }\n',
      'c.ts': 'import { real } from "./real"; export function c(x: number): number { return real(x); }\n',
      'b.ts': 'import { c } from "./c"; export function b(x: number): number { return c(x); }\n',
      'a.ts': 'import { b } from "./b"; export function a(x: number): number { return b(x); }\n',
    });
    let chainResult;
    run('smells.resolved_chain', () => {
      chainResult = structured(args('maintainability', chain));
      assert.ok(rules(chainResult).includes('tsguard.maintainability.LONG_DELEGATION_CHAIN'));
      assert.ok(rules(chainResult).includes('tsguard.structure.FRAGMENTED_DELEGATION'));
      assert.ok(chainResult.findings.every(f => f.status === 'advisory'));
      assert.equal(chainResult.measurements.find(m => m.metric === 'module.dependency_depth' && m.location.file.endsWith('/a.ts')).value, 3);
      const graph = JSON.parse(fs.readFileSync(path.join(root, chainResult.artifacts.find(a => a.kind === 'maintainability_graph').path)));
      assert.equal(graph.modules.length, 4);
      assert.equal(graph.imports.length, 3);
      assert.ok(graph.imports.every(e => e.id === JSON.stringify([e.caller, e.callee])));
    });
    run('contract.deterministic_and_bounded', () => {
      assert.deepEqual(structured(args('maintainability', chain)).findings.map(f => f.id), chainResult.findings.map(f => f.id));
      const result = execute([...args('maintainability', chain), '--output', 'agent']);
      assert.ok(Buffer.byteLength(result.stdout) <= 6144 && result.stdout.split('\n').length <= 27);
    });
    run('smells.equivalent_simplification_control', () => {
      // The unchanged forwarding layers pass existing gates. The direct
      // implementation returns the same x * 2 without those extra layers.
      execute(args('fix', chain));
      structured(args('lint', chain));
      structured(args('fta', chain));
      structured(args('types', chain));
      const direct = source('direct', { 'source.ts': 'export function a(x: number): number { return x * 2; }\n' });
      assert.equal(structured(args('maintainability', direct)).findings.length, 0);
    });
    const legitimate = source('legitimate', {
      'source.ts': 'interface Dependency { load(x: number): number; }\nexport class Service { constructor(private dependency: Dependency) {} load(x: number) { return this.dependency.load(x); } }\nexport function domain(x: number) { if (x < 0) return -1; if (x === 0) return 0; if (x > 100) return 100; return x; }\nexport function boundary(x: number) { return domain(x); }\nexport function transform(x: number) { return boundary(x) + 1; }\n',
    });
    run('smells.legitimate_DI_validation_and_boundary', () => {
      assert.equal(structured(args('maintainability', legitimate)).findings.length, 0);
    });
    const policies = source('policies', { 'source.ts': 'function work() { return 1; }\nexport function propagate() { try { return work(); } catch (error) { throw error; } }\nexport function record() { try { return work(); } catch (error) { console.error(error); return null; } }\nexport function silent() { try { return work(); } catch { return null; } }\n' });
    run('smells.failure_policy_controls', () => {
      const findings = structured(args('smells', policies)).findings;
      assert.equal(findings.length, 1);
      assert.equal(findings[0].rule, 'tsguard.maintainability.SILENT_EXCEPTION_FALLBACK');
      assert.equal(findings[0].location.line, 4);
      assert.equal(findings[0].status, 'advisory');
    });
    const handlers = source('repeated-handlers', { 'source.ts': ['a', 'b', 'c'].map(n => `export function ${n}() { try { return JSON.parse("0"); } catch (error) { console.error("failed", error); throw error; } }`).join('\n') });
    run('smells.repeated_handlers', () => {
      const finding = structured(args('smells', handlers)).findings.find(f => f.rule.endsWith('REPEATED_ERROR_HANDLER'));
      assert.equal(finding.observed, 3);
      assert.equal(finding.related.length, 2);
    });
    run('duplicates.repeated_validation', () => {
      const validation='export function validate(value: number) {\n  if (!Number.isFinite(value)) throw new Error("finite");\n  if (value < 0) throw new Error("negative");\n  if (value > 100) throw new Error("large");\n  if (value % 1 !== 0) throw new Error("integer");\n  return value;\n}\n';
      const dir=source('validation-duplicates',{'a.ts':validation,'b.ts':validation});
      const result=structured(args('duplicates',dir));
      assert.ok(rules(result).includes('jscpd.duplicate'));
      assert.ok(result.findings.every(f=>f.status==='advisory'));
    });
    const cycle = source('cycle', { 'a.ts': 'export { b } from "./b"; export const a = 1;\n', 'b.ts': 'export { a } from "./a"; export const b = 2;\n' });
    run('structure.runtime_cycle', () => {
      const result = structured(args('structure', cycle));
      assert.ok(rules(result).includes('tsguard.structure.CIRCULAR_DEPENDENCY'));
      assert.equal(result.measurements.find(m => m.metric === 'module.cycles').value, 1);
      assert.ok(result.measurements.filter(m => m.metric === 'module.dependency_depth').every(m => m.value === 0));
    });
    const typeCycle = source('type-cycle', { 'a.ts': 'import type { B } from "./b"; export interface A { b?: B }\n', 'b.ts': 'import type { A } from "./a"; export interface B { a?: A }\n' });
    run('structure.type_only_cycle_control', () => {
      const result = structured(args('structure', typeCycle));
      assert.equal(result.findings.length, 0);
      assert.equal(result.measurements.find(m => m.metric === 'module.edges').value, 0);
      assert.equal(result.measurements.find(m => m.metric === 'module.type_imports').value, 2);
    });
    const unresolved = source('unresolved', { 'source.ts': 'import { missing } from "./missing"; export const value = missing;\n' });
    run('structure.unresolved_is_partial', () => {
      const result = structured(args('structure', unresolved));
      assert.equal(result.assessment, 'incomplete');
      assert.ok(result.findings.some(f => f.rule.endsWith('not_evaluated')));
    });
    const excluded = source('excluded', { 'source.ts': 'export const value = 1;\n', 'ignored/source.ts': 'import { nope } from "./nope";\n', 'source.test.ts': 'import { nope } from "./nope";\n', 'source.generated.ts': 'import { nope } from "./nope";\n' });
    run('structure.scope_and_generated_exclusions', () => {
      const result = structured(args('structure', excluded, ['--exclude', `${excluded}/ignored`]));
      assert.equal(result.assessment, 'complete');
      assert.equal(result.measurements.find(m => m.metric === 'module.count').value, 1);
    });
    run('structure.function_graph', () => {
      const dir = source('function-graph', { 'source.ts': 'function target() { return 1; }\nexport function a() { target(); return target(); }\nexport function b() { return target(); }\nexport function recursive() { return recursive(); }\n' });
      const result = structured(args('structure', dir));
      assert.equal(result.measurements.find(m => m.metric === 'function.fan_in' && m.location.symbol === 'target').value, 2);
      assert.equal(result.measurements.find(m => m.metric === 'function.fan_out' && m.location.symbol === 'a').value, 1);
      assert.equal(result.measurements.find(m => m.metric === 'function.call_depth' && m.location.symbol === 'a').value, 1);
      assert.equal(result.measurements.find(m => m.metric === 'function.recursive_components').value, 1);
      assert.equal(result.findings.length, 0);
    });
    run('structure.coupling', () => {
      const files = Object.fromEntries(Array.from({length:8},(_,i) => [`leaf${i}.ts`, `export const value${i} = ${i};\n`]));
      files['hub.ts'] = Array.from({length:8},(_,i) => `export { value${i} } from "./leaf${i}";`).join('\n');
      for (let i=0;i<3;i++) files[`caller${i}.ts`] = 'export { value0 } from "./hub";\n';
      const dir = source('coupling',files);
      const result = structured(args('structure',dir));
      assert.equal(result.measurements.find(m=>m.metric==='module.fan_in' && m.location.file.endsWith('/hub.ts')).value,3);
      assert.equal(result.measurements.find(m=>m.metric==='module.fan_out' && m.location.file.endsWith('/hub.ts')).value,8);
      assert.ok(rules(result).includes('tsguard.structure.HIGH_MODULE_COUPLING'));
    });
    run('change.absent_baseline', () => {
      const result = structured(args('change', legitimate));
      assert.equal(result.assessment, 'incomplete');
      assert.ok(result.gates.some(g => g.name === 'change' && g.status === 'not_run'));
      assert.ok(rules(result).includes('tsguard.change.BASELINE_UNAVAILABLE'));
    });
    const comparison = path.join(directory, 'comparison');
    fs.mkdirSync(comparison);
    const git = argv => {
      const result = spawnSync('git', argv, { cwd: comparison, encoding: 'utf8', env: { ...process.env, GIT_CONFIG_NOSYSTEM: '1' } });
      assert.equal(result.status, 0, result.stderr);
      return result.stdout.trim();
    };
    fs.writeFileSync(path.join(comparison, 'package.json'), JSON.stringify({ private: true, scripts: { prepare: 'node dangerous.cjs' } }));
    const marker = path.join(directory, 'baseline-executed');
    fs.writeFileSync(path.join(comparison, 'dangerous.cjs'), `require('node:fs').writeFileSync(${JSON.stringify(marker)}, 'executed');`);
    fs.writeFileSync(path.join(comparison, '.gitattributes'), '*.ts export-ignore\n');
    fs.mkdirSync(path.join(comparison,'nested'));
    fs.writeFileSync(path.join(comparison,'nested/package.json'),'{}');
    fs.writeFileSync(path.join(comparison,'nested/source.ts'),'export function identity(x: number) { return x; }\n');
    fs.writeFileSync(path.join(comparison, 'original.ts'), 'export function original(x: number) { let out = 0; if (x > 0) out += 1; if (x > 1) out += 2; if (x > 2) out += 3; if (x > 3) out += 4; return out; }\n');
    git(['init', '-q']); git(['add', '.']); git(['-c', 'user.name=Eval', '-c', 'user.email=eval@example.invalid', 'commit', '-qm', 'inert baseline']);
    const sha = git(['rev-parse', 'HEAD']);
    fs.rmSync(path.join(comparison, 'original.ts'));
    fs.writeFileSync(path.join(comparison, 'a.ts'), 'export function a(x: number) { let out = 0; if (x > 0) out += 1; if (x > 1) out += 2; return out; }\n');
    fs.writeFileSync(path.join(comparison, 'b.ts'), 'export function b(x: number) { let out = 0; if (x > 2) out += 3; if (x > 3) out += 4; return out; }\n');
    fs.writeFileSync(path.join(comparison, 'entry.ts'), 'import { a } from "./a"; import { b } from "./b"; export function original(x: number) { return a(x) + b(x); }\n');
    const comparisonArgs = ['change', '--root', comparison, '--dirs', '.', '--baseline', sha];
    run('change.artificial_split_and_inert_baseline', () => {
      const result = structured(comparisonArgs);
      assert.ok(rules(result).includes('tsguard.change.POSSIBLE_COMPLEXITY_DISPLACEMENT'));
      assert.equal(result.measurements.find(m => m.metric === 'change.baseline.total_branches').value, 4);
      assert.equal(result.measurements.find(m => m.metric === 'change.candidate.total_branches').value, 4);
      assert.equal(fs.existsSync(marker), false);
      assert.ok(result.measurements.some(m=>m.metric==='change.baseline.fta.score'));
      assert.ok(result.measurements.some(m=>m.metric==='change.candidate.function.cognitive_min'));
      assert.ok(result.measurements.some(m=>m.metric==='change.baseline.duplicated_tokens'));
      const artifact = JSON.parse(fs.readFileSync(path.join(comparison,result.artifacts.find(a=>a.kind==='maintainability_change').path),'utf8'));
      assert.equal(artifact.baseline_sha,sha);
      assert.ok(artifact.native_metrics.baseline.files.some(f=>f.file==='original.ts'));
      assert.ok(artifact.native_metrics.candidate.files.some(f=>f.file==='a.ts'));
    });
    run('change.real_simplification_control', () => {
      fs.writeFileSync(path.join(comparison, 'a.ts'), 'export function a(x: number) { return Number(x > 0) + 2 * Number(x > 1); }\n');
      fs.writeFileSync(path.join(comparison, 'b.ts'), 'export function b(x: number) { return 3 * Number(x > 2) + 4 * Number(x > 3); }\n');
      assert.ok(!rules(structured(comparisonArgs)).includes('tsguard.change.POSSIBLE_COMPLEXITY_DISPLACEMENT'));
    });
    run('change.unavailable_ref', () => {
      const result = structured(['change', '--root', comparison, '--dirs', '.', '--baseline', 'missing-reference']);
      assert.equal(result.assessment, 'incomplete');
      assert.ok(result.gates.some(g => g.name === 'change' && g.status === 'not_run'));
    });
    run('change.nested_project_prefix', () => {
      const result=structured(['change','--root',path.join(comparison,'nested'),'--dirs','.','--baseline',sha]);
      assert.equal(result.assessment,'complete');
      assert.equal(result.measurements.find(m=>m.metric==='change.baseline.module_count').value,1);
      assert.equal(result.measurements.find(m=>m.metric==='change.candidate.module_count').value,1);
    });
    const revision = (name, files) => {
      const rel = source(name, {'package.json':'{}',...files});
      const project = path.join(root,rel);
      const git = argv => {
        const result = spawnSync('git',argv,{cwd:project,encoding:'utf8',env:{...process.env,GIT_CONFIG_NOSYSTEM:'1'}});
        assert.equal(result.status,0,result.stderr); return result.stdout.trim();
      };
      git(['init','-q']);git(['add','.']);git(['-c','user.name=Eval','-c','user.email=eval@example.invalid','commit','-qm','source baseline']);
      const sha=git(['rev-parse','HEAD']);
      return {project,git,sha,compare:baseline=>structured(['change','--root',project,'--dirs','.','--baseline',baseline||sha])};
    };
    run('change.native_duplication_and_move_control', () => {
      const validation='export function validate(value: number) {\n  if (!Number.isFinite(value)) throw new Error("finite");\n  if (value < 0) throw new Error("negative");\n  if (value > 100) throw new Error("large");\n  if (value % 1 !== 0) throw new Error("integer");\n  return value;\n}\n';
      const fixture=revision('clone-change',{'a.ts':validation});
      fs.writeFileSync(path.join(fixture.project,'b.ts'),validation);
      // Historical and current analyzer configs cannot hide selected source.
      fs.writeFileSync(path.join(fixture.project,'.jscpd.json'),'{"ignore":["**/*"]}');
      fs.writeFileSync(path.join(fixture.project,'fta.json'),'{"exclude_filenames":["*"]}');
      let result=fixture.compare();
      assert.equal(result.assessment,'complete');
      assert.ok(rules(result).includes('tsguard.change.NEW_DUPLICATION'));
      assert.equal(result.measurements.find(m=>m.metric==='change.baseline.duplicate_clones').value,0);
      assert.ok(result.measurements.find(m=>m.metric==='change.candidate.duplicated_tokens').value>=50);
      fixture.git(['add','.']);fixture.git(['-c','user.name=Eval','-c','user.email=eval@example.invalid','commit','-qm','existing clones']);
      const duplicatedSHA=fixture.git(['rev-parse','HEAD']);
      fs.renameSync(path.join(fixture.project,'a.ts'),path.join(fixture.project,'renamed.ts'));
      result=fixture.compare(duplicatedSHA);
      assert.equal(result.assessment,'complete');
      assert.ok(!rules(result).includes('tsguard.change.NEW_DUPLICATION'));
      const artifact=JSON.parse(fs.readFileSync(path.join(fixture.project,result.artifacts.find(a=>a.kind==='maintainability_change').path),'utf8'));
      assert.ok(artifact.file_matches.some(m=>m.baseline==='a.ts'&&m.candidate==='renamed.ts'&&m.method==='exact_fingerprint'));
    });
    run('change.native_function_complexity_and_move_control', () => {
      const fixture=revision('function-change',{'a.ts':'export function domain(x: number) { if (x > 0) return 1; return 0; }\n'});
      fs.writeFileSync(path.join(fixture.project,'a.ts'),'export function domain(x: number) { if (x > 0) { if (x > 1) { if (x > 2) return 3; return 2; } return 1; } return 0; }\n');
      let result=fixture.compare();
      assert.equal(result.assessment,'complete');
      assert.ok(rules(result).includes('tsguard.change.FUNCTION_COMPLEXITY_INCREASE'));
      fixture.git(['add','a.ts']);fixture.git(['-c','user.name=Eval','-c','user.email=eval@example.invalid','commit','-qm','necessary domain behavior']);
      const complexSHA=fixture.git(['rev-parse','HEAD']);
      fs.renameSync(path.join(fixture.project,'a.ts'),path.join(fixture.project,'moved.ts'));
      result=fixture.compare(complexSHA);
      assert.equal(result.assessment,'complete');
      assert.equal(result.findings.length,0);
      assert.equal(result.measurements.find(m=>m.metric==='change.matched_functions').value,1);
    });
    run('change.cognitive_suppression_is_partial', () => {
      const code='// biome-ignore lint/complexity/noExcessiveCognitiveComplexity: intentional domain work\nexport function domain(x: number) { if (x > 0) { if (x > 1) return 2; return 1; } return 0; }\n';
      const fixture=revision('cognitive-suppressed',{'source.ts':code});
      const result=fixture.compare();
      assert.equal(result.assessment,'incomplete');
      assert.equal(result.gates.find(g=>g.name==='change').normalization,'partial');
      assert.ok(result.findings.some(f=>f.rule==='tsguard.change.not_evaluated'));
      assert.ok(!result.measurements.some(m=>m.metric==='change.candidate.total_cognitive_max'));
      assert.ok(result.measurements.some(m=>m.metric==='change.candidate.fta.score'));
    });
    run('change.large_graph_input_and_generated_scope', () => {
      const files=Object.fromEntries(Array.from({length:160},(_,i)=>[`source${i}.ts`,`export function function${i}(value: number) { return value + ${i}; }\n`]));
      files['ignored.generated.ts']='throw new Error("must not run");\n';
      const fixture=revision('large-change',files);
      const result=fixture.compare();
      assert.equal(result.assessment,'complete');
      assert.equal(result.measurements.find(m=>m.metric==='change.matched_functions').value,160);
      assert.equal(result.measurements.filter(m=>m.metric==='change.candidate.fta.score').length,160);
      assert.equal(result.findings.length,0);
    });
    run('change.project_compiler_consistency', () => {
      const fixture=revision('selected-compiler',{'package.json':JSON.stringify({devDependencies:{typescript:'5.9.3'}}),'source.ts':'export function identity(value: number) { return value; }\n',
        'node_modules/typescript/package.json':'{"name":"typescript","main":"index.cjs"}', 'node_modules/typescript/index.cjs':'throw new Error("historical compiler must not be loaded");\n'});
      const runtime=process.env.TSGUARD_RUNTIME || fs.realpathSync(path.join(root,'node_modules/@oxguard/tsguard'));
      const owned=require('node:module').createRequire(path.join(runtime,'package.json'));
      fs.writeFileSync(path.join(fixture.project,'node_modules/typescript/index.cjs'),`module.exports = { ...require(${JSON.stringify(owned.resolve('typescript'))}), version: "5.9.3-eval-current" };\n`);
      const result=fixture.compare();
      assert.equal(result.assessment,'complete');
      const artifact=JSON.parse(fs.readFileSync(path.join(fixture.project,result.artifacts.find(a=>a.kind==='maintainability_change').path),'utf8'));
      assert.equal(artifact.baseline_structure.compiler_version,'5.9.3-eval-current');
      assert.equal(artifact.candidate_structure.compiler_version,'5.9.3-eval-current');
    });
    return { schema_version: '1', suite: 'maintainability.installed', evaluator_sha256: crypto.createHash('sha256').update(fs.readFileSync(__filename)).digest('hex'), environment: {platform:process.platform,arch:process.arch,node:process.version}, cases, passed: cases.length,
      duration_ms: timings.reduce((a,b) => a+b, 0), python_parity: 'not implemented for new Level 2/3 capabilities; existing Level 1 is unchanged' };
  } finally { fs.rmSync(directory, { recursive: true, force: true }); }
}
module.exports = { evaluate };
