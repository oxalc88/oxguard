'use strict';
// Shared compiler input. Never import project application code or executable configs.
function loadProject(input) {
  const fs = require('node:fs');
  const path = require('node:path');
  const { createRequire } = require('node:module');
  const root = process.cwd();
  const project = createRequire(path.join(root, 'package.json'));
  const manifest = JSON.parse(fs.readFileSync(path.join(root, 'package.json'), 'utf8'));
  const owned = input.runtime ? createRequire(path.join(input.runtime, 'bin/tool.cjs')) : project;
  const ts = (manifest.dependencies?.typescript || manifest.devDependencies?.typescript ? project : owned)('typescript');
  const relative = file => path.relative(root, file).split(path.sep).join('/');
  const excluded = file => input.exclude.some(dir => {
    const normalized = dir.replaceAll('\\', '/').replace(/^\.\//, '').replace(/\/$/, '');
    const rel = relative(file);
    return rel === normalized || rel.startsWith(normalized + '/') || rel.split('/').includes(normalized);
  });
  const files = new Set();
  function collect(file) {
    if (excluded(file)) return;
    if (input.excludeTests && (/(?:^|[\\/])(?:__tests__|__mocks__|__fixtures__)(?:[\\/]|$)/.test(file) || /\.(?:test|spec)\.[^.]+$/.test(file))) return;
    if (/\.(?:min|bundle|generated)\.[^.]+$/.test(file)) return;
    const stat = fs.lstatSync(file);
    if (stat.isSymbolicLink()) return;
    if (stat.isDirectory()) {
      for (const name of fs.readdirSync(file).sort()) collect(path.join(file, name));
    } else if (/\.(?:ts|tsx|mts|cts|js|jsx|mjs|cjs)$/.test(file) && !/\.d\.(?:ts|mts|cts)$/.test(file)) files.add(path.resolve(file));
  }
  for (const dir of input.dirs) {
    const full = path.resolve(root, dir);
    if (!fs.existsSync(full)) throw Object.assign(new Error(`Missing source scope: ${dir}`), { category: 'invalid_configuration' });
    collect(full);
  }
  let options = { strict: true, allowJs: true, target: ts.ScriptTarget.ES2022, module: ts.ModuleKind.ESNext, moduleResolution: ts.ModuleResolutionKind.Bundler, noEmit: true, jsx: ts.JsxEmit.Preserve };
  const configFile = path.join(root, 'tsconfig.json').replaceAll('\\', '/');
  if (fs.existsSync(configFile)) {
    const config = ts.readConfigFile(configFile, ts.sys.readFile);
    if (config.error) throw Object.assign(new Error(ts.flattenDiagnosticMessageText(config.error.messageText, '\n')), { category: 'invalid_configuration' });
    const parsed = ts.parseJsonConfigFileContent(config.config, ts.sys, root.replaceAll('\\', '/'));
    const errors = parsed.errors.filter(e => e.code !== 18003);
    if (errors.length) throw Object.assign(new Error(errors.map(e => ts.flattenDiagnosticMessageText(e.messageText, '\n')).join('\n')), { category: 'invalid_configuration' });
    options = { ...parsed.options, allowJs: true, noEmit: true };
  }
  const program = ts.createProgram([...files].sort(), options);
  const sources = program.getSourceFiles().filter(s => files.has(path.resolve(s.fileName))).sort((a, b) => a.fileName < b.fileName ? -1 : a.fileName > b.fileName ? 1 : 0);
  const syntaxErrors = program.getSyntacticDiagnostics().filter(d => d.file && files.has(path.resolve(d.file.fileName)));
  if (syntaxErrors.length) throw new Error(syntaxErrors.map(d => `${relative(d.file.fileName)}: ${ts.flattenDiagnosticMessageText(d.messageText, '\n')}`).join('\n'));
  return { ts, program, sources, files, relative, owned, root, options };
}

async function emitAnalysis(analyze) {
  try { process.stdout.write(JSON.stringify(await analyze(JSON.parse(process.argv[1]))) + '\n'); }
  catch (error) {
    const category = error.category || (['MODULE_NOT_FOUND', 'ERR_MODULE_NOT_FOUND'].includes(error.code) ? 'tool_missing' : 'analyzer_failure');
    console.error(error.stack);
    process.stdout.write(JSON.stringify({ error: { category, message: error.message } }) + '\n');
  }
}
