'use strict';
const test=require('node:test'),assert=require('node:assert/strict');
const fs=require('node:fs'),os=require('node:os'),path=require('node:path');
const {inventory,observe}=require('./observe.cjs');
function fixture(t) {
  const base=fs.mkdtempSync(path.join(os.tmpdir(),'p0-observer-'));
  t.after(()=>fs.rmSync(base,{recursive:true,force:true}));
  const root=path.join(base,'project');fs.mkdirSync(path.join(root,'src'),{recursive:true});
  fs.mkdirSync(path.join(root,'tests'));fs.writeFileSync(path.join(root,'src/main.py'),'value = 1\n');
  fs.writeFileSync(path.join(root,'tests/test_main.py'),'assert True\n');
  fs.writeFileSync(path.join(root,'policy.toml'),'cap = 60\n');
  const config=path.join(base,'session.json');
  fs.writeFileSync(config,JSON.stringify({root,initial:inventory(root),events:path.join(base,'events.jsonl'),tools:{node:[process.execPath]},read_only:['policy.toml']}));
  return {base,root,config};
}
test('observer preserves read-before-edit chronology and covers tests/configuration',t=>{
  const f=fixture(t);
  assert.equal(observe(f.config,'read',['src/main.py']).event.sequence,1);
  const edit=observe(f.config,'write',['src/main.py',Buffer.from('value = 2\n').toString('base64')]);
  assert.equal(edit.event.sequence,2);assert.equal(edit.event.observed_drift,false);
  assert.notEqual(edit.event.before['src/main.py'],edit.event.after['src/main.py']);
  assert.ok(edit.event.after['tests/test_main.py']);assert.ok(edit.event.after['policy.toml']);
  fs.writeFileSync(path.join(f.root,'.pyguard.pid'),'123');
  const finish=observe(f.config,'finish',[Buffer.from('done').toString('base64')]);
  assert.equal(finish.event.response,'done');assert.equal(finish.event.observed_drift,false);
});
test('unobserved test edits and protected configuration writes cannot become valid evidence',t=>{
  const f=fixture(t);observe(f.config,'list',[]);
  fs.writeFileSync(path.join(f.root,'tests/test_main.py'),'# removed assertion\n');
  assert.equal(observe(f.config,'read',['src/main.py']).event.observed_drift,true);
  assert.equal(observe(f.config,'write',['policy.toml',Buffer.from('cap = 1000').toString('base64')]).status,1);
  assert.equal(observe(f.config,'read',['../outside']).status,1);
});
test('real process mutations and failures are recorded without grading agent behavior',t=>{
  const f=fixture(t);
  const result=observe(f.config,'exec',['node','-e','require("fs").writeFileSync("src/main.py","value = 3\\n");process.exit(7)']);
  assert.equal(result.status,7);assert.equal(result.event.exit_code,7);
  assert.notEqual(result.event.before['src/main.py'],result.event.after['src/main.py']);
  assert.equal(result.event.outcome,undefined);
});
