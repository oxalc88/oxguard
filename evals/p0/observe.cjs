'use strict';
// Evaluation-only observer. It records actions; it does not choose repairs or grade them.
const fs = require('node:fs');
const path = require('node:path');
const crypto = require('node:crypto');
const { spawnSync } = require('node:child_process');
const ignored = new Set(['node_modules', '.git', '__pycache__', '.mypy_cache', '.ruff_cache', '.pytest_cache', '.pyguard-cache', 'coverage', '.cache', '.p0-build', '.p0-evidence']);
const sha = data => crypto.createHash('sha256').update(data).digest('hex');
function inventory(root) {
  const files = {};
  function walk(dir) {
    for (const entry of fs.readdirSync(dir, { withFileTypes: true }).sort((a,b) => a.name.localeCompare(b.name))) {
      if (ignored.has(entry.name) || ['.coverage','.tsguard.pid','.pyguard.pid'].includes(entry.name) || entry.name.endsWith('.pyc')) continue;
      const file = path.join(dir, entry.name);
      if (entry.isSymbolicLink()) continue;
      if (entry.isDirectory()) walk(file);
      else if (entry.isFile()) files[path.relative(root, file).replaceAll('\\','/')] = sha(fs.readFileSync(file));
    }
  }
  walk(root);
  return files;
}
function target(root, relative) {
  const file = path.resolve(root, relative || '.');
  if (file !== root && !file.startsWith(root + path.sep)) throw Error('Path outside fixture');
  let cursor = file;
  while (!fs.existsSync(cursor)) cursor = path.dirname(cursor);
  const physical = fs.realpathSync(cursor);
  if (physical !== root && !physical.startsWith(root + path.sep)) throw Error('Linked path outside fixture');
  return file;
}
function observe(configPath, operation, args) {
  const config = JSON.parse(fs.readFileSync(configPath));
  const root = fs.realpathSync(config.root);
  const before = inventory(root);
  const events = fs.existsSync(config.events) ? fs.readFileSync(config.events,'utf8').trim().split('\n').filter(Boolean).map(s=>JSON.parse(s)) : [];
  const prior = events.at(-1)?.after || config.initial;
  const event = { sequence: events.length + 1, operation, args, before, observed_drift: prior && JSON.stringify(prior) !== JSON.stringify(before), timestamp: new Date().toISOString() };
  let output = '', status = 0;
  try {
    if (operation === 'read') {
      const file = target(root,args[0]);
      output = fs.readFileSync(file,'utf8');
      event.read = { path:path.relative(root,file).replaceAll('\\','/'), sha256:sha(Buffer.from(output)) };
    } else if (operation === 'list') {
      output = Object.keys(before).filter(name => !args[0] || name.startsWith(args[0].replaceAll('\\','/'))).join('\n');
    } else if (operation === 'search') {
      const expression = new RegExp(args[0], 'i');
      const matches = [];
      for (const name of Object.keys(before)) {
        if (!/\.(?:ts|tsx|js|cjs|mjs|py|toml|json|md)$/.test(name)) continue;
        fs.readFileSync(target(root,name),'utf8').split('\n').forEach((line,index) => {
          if (expression.test(line)) matches.push({file:name,line:index+1,text:line});
        });
      }
      event.search_matches = matches.map(({file,line})=>({file,line}));
      output = matches.map(m=>`${m.file}:${m.line}: ${m.text}`).join('\n');
    } else if (operation === 'write') {
      const file = target(root,args[0]);
      const name = path.relative(root,file).replaceAll('\\','/');
      if ((config.read_only || []).some(prefix => name === prefix || name.startsWith(prefix + '/'))) throw Error('Evaluation reference is read-only');
      if (args.length !== 2) throw Error('write expects relative path and base64 UTF-8 contents');
      const previous = fs.existsSync(file) ? fs.readFileSync(file,'utf8') : null;
      const contents = Buffer.from(args[1],'base64');
      fs.mkdirSync(path.dirname(file),{recursive:true}); fs.writeFileSync(file,contents);
      event.edit = {path:name,before:previous,after:contents.toString('utf8')};
      output = `Wrote ${name}`;
    } else if (operation === 'exec') {
      const tool = config.tools[args[0]];
      if (!tool) throw Error('Unknown tool; available: ' + Object.keys(config.tools).join(', '));
      const command = typeof tool === 'string' ? [tool] : tool;
      const result = spawnSync(command[0], [...command.slice(1),...args.slice(1)], {
        cwd:config.cwd || root,env:{...process.env,...config.env},encoding:'utf8',timeout:config.timeout_ms || 90000,maxBuffer:16*1024*1024,
      });
      event.exit_code = result.status;
      event.stdout = result.stdout || ''; event.stderr = result.stderr || '';
      if(result.error || result.signal) event.execution_error = result.error?.message || result.signal;
      output = JSON.stringify({exit_code:result.status,stdout:event.stdout,stderr:event.stderr,error:event.execution_error},null,2);
      status = result.status === null ? 1 : result.status;
    } else if (operation === 'finish') {
      event.response = args[0] ? Buffer.from(args[0],'base64').toString('utf8') : '';
      output = 'Recorded final response.';
    } else throw Error('Use list [prefix], read <path>, search <regex>, write <path> <base64>, exec <tool> [args], finish <base64 response>');
  } catch(error) {event.observer_error=error.message;output=error.message;status=1;}
  event.after=inventory(root);
  fs.mkdirSync(path.dirname(config.events),{recursive:true});
  // Reads may be launched independently; number events at append time rather
  // than at operation start. Consumers also retain physical JSONL record order.
  const recorded=fs.existsSync(config.events)?fs.readFileSync(config.events,'utf8').trim().split('\n').filter(Boolean):[];
  event.sequence=recorded.length+1;
  fs.appendFileSync(config.events,JSON.stringify(event)+'\n');
  return {output,status,event};
}
if(require.main===module) {
  const [config,operation,...args]=process.argv.slice(2);
  if(!config || !operation) throw Error('Usage: node observe.cjs session.json <operation> [args]');
  const result=observe(config,operation,args); process.stdout.write(result.output+'\n');process.exitCode=result.status;
}
module.exports={inventory,observe};
