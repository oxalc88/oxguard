'use strict';
const fs=require('node:fs'),path=require('node:path'),crypto=require('node:crypto');
const {score}=require('./score.cjs');
const {inventory}=require('./observe.cjs');
const hash=data=>crypto.createHash('sha256').update(data).digest('hex');
const read=file=>JSON.parse(fs.readFileSync(file));
function summarize(options) {
  const cases=[],inputs={};
  const reviews=read(options.reviews);
  inputs[options.reviews]=hash(fs.readFileSync(options.reviews));
  for(const language of options.languages) {
    const native=read(language.native),sessions=read(language.sessions),manifest=read(language.manifest);
    for(const file of [language.native,language.sessions,language.manifest]) inputs[file]=hash(fs.readFileSync(file));
    const definitions=require('./'+native.language+'.cjs').definitions;
    for(const entry of native.cases) {
      const capture=sessions.find(s=>path.basename(path.dirname(s.config))===entry.id);
      if(!capture || !reviews[entry.id]) throw Error('Missing capture or independent review: '+entry.id);
      const required=entry.id.endsWith('.context_before_edit')?definitions.find(d=>entry.id.endsWith('.'+d.id)).context:[];
      const agent=score({session:capture.config,requiredContext:required,judgment:reviews[entry.id]});
      const nativeFailed=entry.outcome==='fail' || entry.support_outcome==='fail' || entry.behavior.outcome!=='pass';
      const pending=entry.support_outcome==='not_evaluated' || (entry.id.endsWith('.multiple_failures') && !['pass','fail'].includes(entry.outcome));
      const outcome=agent.outcome==='invalid_capture'?'invalid_capture':nativeFailed || agent.outcome==='fail'?'fail':!pending && agent.outcome==='pass'?'pass':'not_evaluated';
      cases.push({id:entry.id,outcome,native_outcome:entry.outcome,support_outcome:entry.support_outcome,baseline_behavior:entry.behavior.outcome,agent});
      inputs[capture.events]=hash(fs.readFileSync(capture.events));
    }
    for(const fixture of manifest.fixtures || manifest.projects) {
      const files=inventory(path.resolve(path.dirname(language.manifest),fixture.directory));
      inputs['fixture:'+fixture.id]={tree_sha256:hash(JSON.stringify(files)),files};
    }
  }
  const evaluatorFiles=inventory(__dirname);
  const report={schema_version:1,captured_at:new Date().toISOString(),provenance:read(options.provenance),sampling:'One independent history-free agent per language/case; exploratory, not a model failure-rate estimate',counts:cases.reduce((a,c)=>(a[c.outcome]=(a[c.outcome]||0)+1,a),{}),cases,inputs,evaluator_files:evaluatorFiles,evaluator_tree_sha256:hash(JSON.stringify(evaluatorFiles))};
  fs.mkdirSync(options.output,{recursive:true});
  fs.writeFileSync(path.join(options.output,'p0-summary.json'),JSON.stringify(report,null,2)+'\n');
  for(const entry of cases) fs.writeFileSync(path.join(options.output,entry.id+'.json'),JSON.stringify(entry,null,2)+'\n');
  return report;
}
if(require.main===module) {
  const report=summarize(read(process.argv[2]));
  console.log(JSON.stringify({counts:report.counts,cases:report.cases.map(c=>({id:c.id,outcome:c.outcome,agent:c.agent.outcome,checks:c.agent.checks,context:c.agent.required_context_observed_before_edit}))}));
  process.exitCode=report.cases.some(c=>c.outcome==='invalid_capture' || c.outcome==='fail')?1:report.cases.some(c=>c.outcome!=='pass')?2:0;
}
module.exports={summarize};
