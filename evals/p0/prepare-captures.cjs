'use strict';
const fs=require('node:fs'),path=require('node:path');
const {createSession}=require('./sessions.cjs');
function prepareCaptures({manifest,output,runtime,python,uv,binary,node=process.execPath,select,setupFiles}) {
  const input=JSON.parse(fs.readFileSync(manifest));
  const language=input.language;
  const rows=input.fixtures || input.projects;
  if(!Array.isArray(rows)) throw Error('Fixture manifest must supply fixtures');
  const selected=rows.filter(row=>!select || select.includes(row.id.split('.').at(-1)));
  return selected.map(row=>createSession({project:path.resolve(path.dirname(manifest),row.directory),output:path.join(output,row.id),language,runtime,python,uv,binary,node,task:row.task,setupFiles}));
}
if(require.main===module) {
  const options=JSON.parse(fs.readFileSync(process.argv[2]));
  const results=prepareCaptures(options);
  fs.writeFileSync(process.argv[3],JSON.stringify(results,null,2)+'\n');
  console.log(JSON.stringify(results.map(x=>({language:x.language,root:x.root,config:x.config,task:x.task}))));
}
module.exports={prepareCaptures};
