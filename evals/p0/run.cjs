'use strict';
const fs=require('node:fs'),path=require('node:path');
function run(options) {
  const reports=[];
  for(const language of options.languages || ['typescript','python']) {
    const implementation=require('./'+language+'.cjs');
    const common={root:options.fixtures,source:path.resolve(__dirname,'../..'),runtime:options.runtime,python:options.python,binary:options.binaries[language]};
    const frozen=options.existingFixtures?.[language];
    const prepared=frozen?{fixturesRoot:path.resolve(frozen)}:implementation.prepare(common);
    const report=implementation.evaluate({...common,fixturesRoot:prepared.fixturesRoot});
    fs.mkdirSync(options.reports,{recursive:true});
    fs.writeFileSync(path.join(options.reports,language+'.json'),JSON.stringify(report,null,2)+'\n');
    reports.push(report);
  }
  return reports;
}
if(require.main===module) {
  const options=JSON.parse(fs.readFileSync(process.argv[2]));
  if(options.env) Object.assign(process.env,options.env);
  const reports=run(options);const cases=reports.flatMap(r=>r.cases);
  console.log(JSON.stringify(cases.map(c=>({id:c.id,outcome:c.outcome,support_outcome:c.support_outcome}))));
  process.exitCode=cases.some(c=>c.outcome==='fail')?1:cases.some(c=>c.outcome==='not_evaluated')?2:0;
}
module.exports={run};
