'use strict';
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');

function evaluate({ source, run, structured, root, args }) {
  const rules = result => result.findings.map(f => f.rule);
  const check = (id, code, command, verify, exit = 0, extra = []) => run(id, () => {
    const dir = source(id.replaceAll('.', '-'), { 'source.ts': code });
    verify(structured(args(command, dir, extra), exit));
  });
  const graph = result => JSON.parse(fs.readFileSync(path.join(root, result.artifacts.find(a => a.kind === 'maintainability_graph').path)));
  check('typed.unsafe_assertion_condition_and_promises', 'declare const value: unknown; export const cast = value as {name: string};\nexport function condition(value: string) { if (value !== undefined) return value; return ""; }\nPromise.resolve(1);\nexport function misuse(values: number[]) { values.forEach(async value => { await Promise.resolve(value); }); }\n', 'typed-lint', result => {
    for (const rule of ['no-unsafe-type-assertion','no-unnecessary-condition','no-floating-promises','no-misused-promises']) assert.ok(rules(result).includes(`typescript/${rule}`));
    assert.equal(result.assessment, 'complete');
    assert.equal(result.findings.find(f => f.rule.endsWith('no-unnecessary-condition')).status, 'advisory');
  }, 1);
  check('typed.valid_guards_and_handled_promises', 'export async function valid(value: unknown) { if(typeof value === "string") return value.toUpperCase(); await Promise.resolve(1); return "fallback"; }\nexport const handled = Promise.resolve(1).catch(error => { console.error(error); return 0; });\n', 'typed-lint', result => assert.equal(result.findings.length, 0));
  check('typed.unresolved_types_are_partial', 'import { value } from "./missing"; export const assigned: string = value;\n', 'typed-lint', result => {
    assert.equal(result.assessment, 'incomplete');
    assert.ok(rules(result).includes('tsguard.typed-lint.not_evaluated'));
    assert.ok(!rules(result).includes('typescript/no-unsafe-assignment'));
  });
  run('typed.unsupported_config_and_stale_report', () => {
    const dir = source('typed-config', { 'source.ts': 'export const value = 1;\n' });
    assert.ok(structured(args('typed-lint', dir)).artifacts.some(a => a.kind === 'typed_lint_native'));
    fs.writeFileSync(path.join(root, 'tsconfig.json'), JSON.stringify({ compilerOptions: { strict:true,target:'ES2022',module:'ESNext',moduleResolution:'Bundler',baseUrl:'.' } }));
    try {
      const result = structured(args('typed-lint', dir));
      assert.equal(result.assessment, 'incomplete');
      const native = JSON.parse(fs.readFileSync(path.join(root, result.artifacts.find(a => a.kind === 'typed_lint_native').path)));
      assert.ok(native.native.diagnostics.some(d => d.code === 'typescript(tsconfig-error)'));
      fs.writeFileSync(path.join(root, 'tsconfig.json'), JSON.stringify({compilerOptions:{strict:false}}));
      const nonStrict = structured(args('typed-lint', dir));
      assert.equal(nonStrict.assessment, 'incomplete');
      assert.ok(!nonStrict.artifacts.some(a => a.kind === 'typed_lint_native'));
    } finally { fs.rmSync(path.join(root, 'tsconfig.json')); }
  });
  check('smells.silent_exits_and_promise_recovery_controls', 'export function loop(values: string[]) { for(const value of values) { try { JSON.parse(value); } catch { continue; } } }\nexport function recorded(values: string[]) { for(const value of values) { try { JSON.parse(value); } catch(error) { console.error(error); continue; } } }\nexport const silent = Promise.reject("bad").catch(() => []);\nexport const recordedPromise = Promise.reject("bad").catch(error => { console.error(error); return []; });\nexport function breakLoop(values: string[]) { for(const value of values) { try { JSON.parse(value); } catch { break; } } }\ndeclare const custom: PromiseLike<number> & {catch(fn: () => number): number}; export const customRecovery = custom.catch(() => 0);\n', 'smells', result => {
    assert.equal(result.findings.filter(f => f.rule.endsWith('SILENT_EXCEPTION_EXIT')).length, 2);
    assert.equal(result.findings.filter(f => f.rule.endsWith('SILENT_PROMISE_REJECTION')).length, 1);
    assert.ok(result.findings.every(f => f.status === 'advisory'));
  });
  check('smells.discarded_settled_rejection', 'export async function publish(values: number[]) { const results = await Promise.allSettled(values.map(v => Promise.resolve(v))); const alias=results; return alias.flatMap((r,i) => { if(r.status === "fulfilled") return []; if(!values[i]) return []; return [r.reason]; }); }\n', 'smells', result => assert.equal(result.findings.filter(f => f.rule.endsWith('DISCARDED_SETTLED_REJECTION')).length, 1));
  check('smells.settled_recorded_and_propagated_controls', 'export async function recorded() { const results=await Promise.allSettled([Promise.resolve(1)]); return results.flatMap(r => { if(r.status === "fulfilled") return []; console.error(r.reason); return []; }); }\nexport async function propagated() { const results=await Promise.allSettled([Promise.resolve(1)]); return results.flatMap(r => { if(r.status === "fulfilled") return []; throw r.reason; }); }\nexport function unrelated(results: {status: string}[]) { return results.flatMap(r => { if(r.status === "fulfilled") return []; return []; }); }\n', 'smells', result => assert.equal(result.findings.length, 0));
  check('smells.branch_specific_failure_visibility', 'export async function branches(flag: boolean) { const results=await Promise.allSettled([Promise.resolve(1)]); return results.flatMap(r => { if(r.status === "fulfilled") return []; if(flag) { console.error(r.reason); return []; } return []; }); }\n', 'smells', result => assert.equal(result.findings.filter(f => f.rule.endsWith('DISCARDED_SETTLED_REJECTION')).length, 1));
  check('smells.async_forwarding_and_boundary_controls', 'export async function work(x: number): Promise<number> { return x * 2; }\nexport async function c(x: number): Promise<number> { return await work(x); }\nexport async function b(x: number): Promise<number> { return c(x); }\nexport async function a(x: number): Promise<number> { return b(x); }\nexport async function boundary(x: number): Promise<number> { try { return await work(x); } catch(error) { throw new Error("boundary",{cause:error}); } }\n', 'maintainability', result => {
    assert.equal(result.findings.filter(f => f.rule.endsWith('LONG_DELEGATION_CHAIN')).length, 1);
    assert.equal(graph(result).functions.find(f => f.location.symbol === 'c').forwarding_mode, 'async-return-await');
    assert.equal(graph(result).functions.find(f => f.location.symbol === 'boundary').forward_target, '');
  });
  check('complexity.function_nesting_and_callback_boundaries', 'export function domain(values: number[]) { for(const value of values) { if(value > 0) { if(value > 1) return values.map(v => { if(v > 2) return v; return 0; }); } } return []; }\n', 'maintainability', result => {
    assert.equal(result.measurements.find(m => m.metric === 'function.max_branch_nesting' && m.location.symbol === 'domain').value, 3);
    assert.equal(graph(result).functions.find(f => f.location.symbol === 'domain').branch_locations.length, 3);
    assert.equal(graph(result).functions.find(f => f.location.symbol === 'domain.<anonymous>').max_branch_nesting, 1);
  });
  check('complexity.fta_failure_has_context', 'export function domain(x: number) { if(x>0) { if(x>1) { if(x>2) { if(x>3) return 4; return 3; } return 2; } return 1; } return 0; }\n', 'fta', result => {
    assert.ok(rules(result).includes('tsguard.fta.score_exceeded'));
    assert.ok(result.measurements.some(m => m.metric === 'function.max_branch_nesting' && m.value === 4));
    assert.ok(result.artifacts.some(a => a.kind === 'maintainability_graph'));
    assert.ok(!result.measurements.some(m => m.metric === 'function.fta.score'));
  }, 1, ['--max-fta-score','1']);
}
module.exports = { evaluate };
