'use strict';
// Runs existing, unchanged evaluators and captures their real outputs.
const fs=require('node:fs'),path=require('node:path'),crypto=require('node:crypto');
const {spawnSync,fork}=require('node:child_process');
const {once}=require('node:events');
const root=path.resolve(__dirname,'../..');
const hash=file=>crypto.createHash('sha256').update(fs.readFileSync(file)).digest('hex');
async function capture({runtime,evidence,node=process.execPath,git}) {
  fs.mkdirSync(evidence,{recursive:true});
  const env={...process.env,TEMP:path.join(runtime,'tmp'),TMP:path.join(runtime,'tmp'),TSGUARD_RUNTIME:path.join(runtime,'ts-runtime'),TSGUARD_NODE:node,OPENGREP_ENABLE_VERSION_CHECK:'false',GOCACHE:path.join(runtime,'go-cache'),GOPATH:path.join(runtime,'go-mod')};
  env.SEMGREP_LOG_FILE=path.join(runtime,'tmp/opengrep.log');
  env.SEMGREP_SETTINGS_FILE=path.join(runtime,'tmp/opengrep-settings.yml');
  env.GIT_CONFIG_COUNT='1';env.GIT_CONFIG_KEY_0='core.longpaths';env.GIT_CONFIG_VALUE_0='true';
  env.PATH=[path.join(runtime,'bin'),path.join(runtime,'go/bin'),path.dirname(node),path.join(runtime,'uv-package/bin'),...(git?[path.dirname(git)]:[]),process.env.PATH || process.env.Path || ''].join(path.delimiter);
  Object.assign(process.env,env);
  if(process.platform==='win32') fs.writeFileSync(path.join(runtime,'bin/npm.cmd'),`@"${node}" "${path.join(runtime,'npm/package/bin/npm-cli.js')}" %*\r\n`);
  const gitCommand=git || 'git';
  const revisionResult=spawnSync(gitCommand,['rev-parse','HEAD'],{cwd:root,encoding:'utf8'});
  if(revisionResult.status!==0) throw Error('Baseline Git preflight: '+(revisionResult.error?.message || revisionResult.stderr));
  const revision=revisionResult.stdout.trim();
  const units=spawnSync(node,['--test','evals/agent-actions.test.cjs','evals/evals.test.cjs','evals/python/evals.test.cjs','evals/record-agent-command.test.cjs','evals/maintainability-parity.test.cjs'],{cwd:root,env,encoding:'utf8',timeout:180000,maxBuffer:8*1024*1024});
  fs.writeFileSync(path.join(evidence,'existing-unit-tests.json'),JSON.stringify({argv:['node','--test','existing eval harness tests'],exit_code:units.status,stdout:units.stdout,stderr:units.stderr,error:units.error?.message},null,2));
  const replay=require('../agent-actions.cjs').evaluate(root);
  fs.writeFileSync(path.join(evidence,'recorded-agent-replay.json'),JSON.stringify(replay,null,2));
  const productDiff=spawnSync(gitCommand,['diff','HEAD','--','tsguard','pyguard','contract','npm','docs'],{cwd:root,encoding:'utf8'});
  const provenance={revision,tag:'v0.9.0',captured_at:new Date().toISOString(),platform:process.platform,architecture:process.arch,node:process.version,source_archive_sha256:hash(path.join(evidence,'source-v0.9.0.tar')),binaries:{tsguard:hash(path.join(runtime,'bin/tsguard.exe')),pyguard:hash(path.join(runtime,'bin/pyguard.exe'))},toolchain_manifest_sha256:hash(path.join(root,'npm/toolchain.json')),python_lock_sha256:hash(path.join(root,'evals/python/requirements.txt')),product_source_unchanged:productDiff.status===0 && productDiff.stdout==='',execution:'Rebuilt unchanged native binaries with workspace-owned pinned analyzers. This is not the complete release/platform matrix.'};
  fs.copyFileSync(path.join(runtime,'ts-runtime/package-lock.json'),path.join(evidence,'npm-runtime-lock.json'));
  fs.copyFileSync(path.join(root,'evals/python/requirements.txt'),path.join(evidence,'python-requirements.lock'));
  fs.writeFileSync(path.join(evidence,'provenance.json'),JSON.stringify(provenance,null,2));
  console.log('Existing harness tests and recorded replay captured. Fetching pinned SAST engine.');
  await require('../../npm/fetch-native.cjs').fetchNative(path.join(runtime,'engines'),'win32-x64');
  process.env.TSGUARD_OPENGREP=path.join(runtime,'engines/win32-x64/opengrep.exe');
  const provider=fork(path.join(root,'npm/testdata/audit-registry.cjs'),[],{stdio:['ignore','ignore','inherit','ipc'],env:process.env});
  try {
    const [message]=await once(provider,'message');
    const previous=path.join(evidence,'tsguard.json');
    if(fs.existsSync(previous) && !fs.existsSync(path.join(evidence,'tsguard-first-attempt.json'))) fs.copyFileSync(previous,path.join(evidence,'tsguard-first-attempt.json'));
    const report=require('../run.cjs').evaluate({command:[path.join(runtime,'bin/tsguard.exe')],revision,suite:'baseline',reportFile:path.join(evidence,'tsguard.json'),auditRegistry:message.registry});
    console.log(JSON.stringify({typescript:report.cases.reduce((a,c)=>(a[c.outcome]=(a[c.outcome]||0)+1,a),{})}));
  } finally {provider.kill();}
  const project=fs.mkdtempSync(path.join(evidence,'existing-maintainability-'));
  fs.writeFileSync(path.join(project,'package.json'),'{"private":true}\n');
  const execute=(args,expected=0)=>{
    const r=spawnSync(path.join(runtime,'bin/tsguard.exe'),args,{cwd:project,env:process.env,encoding:'utf8',timeout:90000,maxBuffer:16*1024*1024});
    if(r.status!==expected) throw Error(`Expected ${expected}, got ${r.status}: ${args.join(' ')}\n${r.stdout}\n${r.stderr}`);
    return r;
  };
  const structured=(args,expected=0)=>JSON.parse(execute([...args,'--output','json'],expected).stdout);
  const maintainability=require('../maintainability.cjs').evaluate({execute,structured,root:project});
  fs.writeFileSync(path.join(evidence,'maintainability.json'),JSON.stringify(maintainability,null,2));
  console.log(JSON.stringify({maintainability}));
}
if(require.main===module) capture(JSON.parse(fs.readFileSync(process.argv[2]))).catch(error=>{console.error(error.stack);process.exitCode=1;});
module.exports={capture};
