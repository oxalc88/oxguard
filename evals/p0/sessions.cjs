'use strict';
const fs=require('node:fs'),path=require('node:path'),crypto=require('node:crypto');
const {inventory}=require('./observe.cjs');
function createSession({project,output,language,runtime,python,uv,binary,node=process.execPath,task,setupFiles={}}) {
  const root=path.join(output,'project');
  if(fs.existsSync(output)) throw Error('Refusing to replace an existing capture');
  fs.mkdirSync(output,{recursive:true});
  fs.cpSync(project,root,{recursive:true,filter: source => !['node_modules','.p0-build','.p0-evidence','__pycache__','.mypy_cache','.ruff_cache','.pytest_cache'].includes(path.basename(source))});
  for(const [name,contents] of Object.entries(setupFiles)) {
    const file=path.resolve(root,name);
    if(!file.startsWith(root+path.sep)) throw Error('Setup file escapes fixture');
    fs.mkdirSync(path.dirname(file),{recursive:true});fs.writeFileSync(file,contents);
  }
  const repository=path.resolve(__dirname,'../..');
  const skillPath=path.join('.agents','skills',language==='typescript'?'tsguard':'pyguard');
  fs.cpSync(path.join(repository,language==='typescript'?'tsguard':'pyguard','skill'),path.join(root,skillPath),{recursive:true});
  const env={TEMP:process.env.TEMP,TMP:process.env.TMP,NO_COLOR:'1',...runtime?.env};
  const tools={node:[node]};
  if(language==='typescript') {
    env.TSGUARD_RUNTIME=runtime.root;env.TSGUARD_NODE=node;
    tools.tsguard=[binary];
    tools.tsc=[node,path.join(runtime.root,'node_modules/typescript/bin/tsc')];
    tools.fta=[node,path.join(runtime.root,'bin/tool.cjs'),'fta'];
    tools.test=[node,'.p0-test.cjs'];
  } else {
    env.VIRTUAL_ENV=path.dirname(path.dirname(python));env.UV_PROJECT_ENVIRONMENT=env.VIRTUAL_ENV;env.UV_NO_SYNC='1';env.UV_OFFLINE='1';
    env.PATH=[path.dirname(python),path.dirname(uv),process.env.PATH].join(path.delimiter);
    tools.pyguard=[binary];tools.python=[python];tools.uv=[uv];tools.test=[python,'-m','unittest','discover','-s','tests'];
  }
  tools.git=['git'];
  const events=path.join(output,'events.jsonl');
  const config={schema_version:1,root,cwd:root,tools,env,events,read_only:[skillPath.replaceAll('\\','/'),'reference'],initial:inventory(root)};
  const configPath=path.join(output,'session.json');
  fs.writeFileSync(configPath,JSON.stringify(config,null,2)+'\n');
  fs.writeFileSync(path.join(output,'task.txt'),task+'\n');
  const skillFiles=Object.keys(config.initial).filter(p=>p.startsWith('.agents/skills/'));
  const metadata={language,task,root,config:configPath,events,setup_files:Object.keys(setupFiles),skill_files:skillFiles,skill_digest:crypto.createHash('sha256').update(JSON.stringify(skillFiles.map(p=>[p,config.initial[p]]))).digest('hex'),capture_started:new Date().toISOString(),model:'inherited parent model; isolated subagent with no conversation history',repetitions:1};
  fs.writeFileSync(path.join(output,'metadata.json'),JSON.stringify(metadata,null,2)+'\n');
  return metadata;
}
if(require.main===module) {
  const options=JSON.parse(fs.readFileSync(process.argv[2]));
  const result=options.map(createSession);
  fs.writeFileSync(process.argv[3],JSON.stringify(result,null,2)+'\n');
  console.log(JSON.stringify(result.map(x=>({language:x.language,root:x.root,config:x.config}))));
}
module.exports={createSession};
