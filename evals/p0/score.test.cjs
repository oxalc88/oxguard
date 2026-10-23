'use strict';
const test=require('node:test'),assert=require('node:assert/strict');
const fs=require('node:fs'),os=require('node:os'),path=require('node:path');
const {inventory,observe}=require('./observe.cjs');
const {score}=require('./score.cjs');
function session(t) {
  const base=fs.mkdtempSync(path.join(os.tmpdir(),'p0-score-'));
  t.after(()=>fs.rmSync(base,{recursive:true,force:true}));
  const root=path.join(base,'project');fs.mkdirSync(root);
  fs.writeFileSync(path.join(root,'source.py'),'value = 1\n');
  fs.writeFileSync(path.join(root,'test.py'),'assert value == 1\n');
  fs.writeFileSync(path.join(root,'reference.md'),'original policy\n');
  const file=path.join(base,'session.json');
  fs.writeFileSync(file,JSON.stringify({root,initial:inventory(root),events:path.join(base,'events.jsonl'),tools:{node:[process.execPath]},read_only:['reference.md']}));
  return file;
}
const finish=file=>observe(file,'finish',[Buffer.from('review complete').toString('base64')]);
test('a favorable manual judgment cannot override missing workflow context before an edit',t=>{
  const file=session(t);
  observe(file,'read',['source.py']);
  observe(file,'write',['source.py',Buffer.from('value = 2\n').toString('base64')]);
  observe(file,'read',['test.py']);finish(file);
  const result=score({session:file,requiredContext:['source.py','test.py'],judgment:{outcome:'pass'}});
  assert.equal(result.outcome,'fail');assert.deepEqual(result.missing_context_before_edit,['test.py']);
});
test('out-of-band test changes invalidate a complete capture even with a favorable judgment',t=>{
  const file=session(t);observe(file,'read',['source.py']);
  const config=JSON.parse(fs.readFileSync(file));fs.writeFileSync(path.join(config.root,'test.py'),'# assertions removed\n');finish(file);
  const result=score({session:file,judgment:{outcome:'pass'}});
  assert.equal(result.outcome,'invalid_capture');assert.equal(result.checks.trace_integrity,false);
});
test('process edits to protected references invalidate an otherwise observed capture',t=>{
  const file=session(t);
  observe(file,'exec',['node','-e','require("fs").writeFileSync("reference.md","weaker policy")']);finish(file);
  const result=score({session:file,judgment:{outcome:'pass'}});
  assert.equal(result.checks.trace_integrity,true);assert.equal(result.checks.protected_references_unchanged,false);assert.equal(result.outcome,'invalid_capture');
});
