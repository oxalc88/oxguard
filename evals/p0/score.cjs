'use strict';
const fs=require('node:fs'),path=require('node:path');
const {inventory}=require('./observe.cjs');
function score({session,requiredContext=[],judgment}) {
  const config=JSON.parse(fs.readFileSync(session));
  const tracked=files=>Object.fromEntries(Object.entries(files||{}).filter(([name])=>!['.tsguard.pid','.pyguard.pid'].includes(name)));
  const events=fs.existsSync(config.events)?fs.readFileSync(config.events,'utf8').trim().split('\n').filter(Boolean).map((s,index)=>{const e=JSON.parse(s);return {...e,sequence:index+1,before:tracked(e.before),after:tracked(e.after)};}):[];
  const edits=events.filter(e=>JSON.stringify(e.before)!==JSON.stringify(e.after));
  const firstEdit=edits[0]?.sequence || Infinity;
  const reads=events.filter(e=>e.read).map(e=>({path:e.read.path,sequence:e.sequence}));
  const beforeEdit=reads.filter(e=>e.sequence<firstEdit).map(e=>e.path);
  const final=events.findLast(e=>e.operation==='finish');
  const last=events.at(-1)?.after;
  const protectedChanges=Object.entries(config.initial).filter(([name,hash])=>(config.read_only||[]).some(prefix=>name===prefix || name.startsWith(prefix+'/')) && last?.[name]!==hash).map(([name])=>name);
  const ordered=events.every((event,index)=>JSON.stringify(event.before)===JSON.stringify(index?events[index-1].after:tracked(config.initial)));
  const checks={captured:events.length>0,finished:!!final,trace_integrity:events.length>0 && ordered && JSON.stringify(last)===JSON.stringify(inventory(config.root)),protected_references_unchanged:protectedChanges.length===0};
  const current=inventory(config.root),initial=config.initial;
  const changes=[...new Set([...Object.keys(initial),...Object.keys(current)])].filter(name=>initial[name]!==current[name]).map(name=>({path:name,kind:!initial[name]?'added':!current[name]?'removed':'modified'}));
  const result={session,checks,reads_before_first_edit:reads.filter(e=>e.sequence<firstEdit),required_context_observed_before_edit:requiredContext.every(name=>beforeEdit.includes(name)),missing_context_before_edit:requiredContext.filter(name=>!beforeEdit.includes(name)),first_edit_sequence:Number.isFinite(firstEdit)?firstEdit:null,changes,native_invocations:events.filter(e=>e.operation==='exec').map(e=>({sequence:e.sequence,args:e.args,exit_code:e.exit_code})),response:final?.response,judgment:judgment || {outcome:'not_evaluated',reason:'Independent review of trace, behavior verification and diff required'}};
  result.outcome=!Object.values(checks).every(Boolean)?'invalid_capture':!result.required_context_observed_before_edit?'fail':result.judgment.outcome;
  return result;
}
if(require.main===module) {
  const config=JSON.parse(fs.readFileSync(process.argv[2]));const result=score(config);
  fs.writeFileSync(process.argv[3],JSON.stringify(result,null,2)+'\n');console.log(JSON.stringify({outcome:result.outcome,checks:result.checks,changes:result.changes}));
}
module.exports={score};
