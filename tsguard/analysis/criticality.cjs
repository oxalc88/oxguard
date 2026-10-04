'use strict';
// Compiler-backed analyzer only: emits nodes/edges. Go owns ranking, policy,
// reporting and artifacts. This is embedded in the native binary, not a launcher.
const fs = require('node:fs');
const path = require('node:path');
const { createRequire } = require('node:module');
function analyze() {
const input = JSON.parse(process.argv[1]);
const root = process.cwd();
const project = createRequire(path.join(root, 'package.json'));
const manifest = JSON.parse(fs.readFileSync(path.join(root, 'package.json'), 'utf8'));
const resolver = manifest.dependencies?.typescript || manifest.devDependencies?.typescript || !input.runtime
  ? project : createRequire(path.join(input.runtime, 'bin/tool.cjs'));
const ts = resolver('typescript');
const relative = file => path.relative(root, file).split(path.sep).join('/');
const excluded = file => input.exclude.some(dir => {
  const normalized = dir.replaceAll('\\', '/').replace(/^\.\//, '').replace(/\/$/, '');
  const rel = relative(file);
  return rel === normalized || rel.startsWith(normalized + '/') || rel.split('/').includes(normalized);
});
const files = new Set();
function collect(file) {
  if (excluded(file)) return;
  const stat = fs.lstatSync(file);
  if (stat.isSymbolicLink()) return;
  if (stat.isDirectory()) {
    for (const name of fs.readdirSync(file).sort()) collect(path.join(file, name));
  } else if (/\.(?:ts|tsx|mts|cts)$/.test(file) && !/\.d\.(?:ts|mts|cts)$/.test(file)) files.add(path.resolve(file));
}
for (const dir of input.dirs) {
  const full = path.resolve(root, dir);
  if (fs.existsSync(full)) collect(full);
}
let options = { target: ts.ScriptTarget.Latest, module: ts.ModuleKind.NodeNext, moduleResolution: ts.ModuleResolutionKind.NodeNext, noEmit: true, jsx: ts.JsxEmit.Preserve };
const configFile = path.join(root, 'tsconfig.json');
if (fs.existsSync(configFile)) {
  // Compiler config diagnostics compare normalized filenames on Windows.
  const config = ts.readConfigFile(configFile.replaceAll('\\', '/'), ts.sys.readFile);
  if (config.error) throw Object.assign(new Error(ts.flattenDiagnosticMessageText(config.error.messageText, '\n')), { category: 'invalid_configuration' });
  const parsed = ts.parseJsonConfigFileContent(config.config, ts.sys, root.replaceAll('\\', '/'));
  const errors = parsed.errors.filter(e => e.code !== 18003); // scope is explicit dirs
  if (errors.length) throw Object.assign(new Error(errors.map(e => ts.flattenDiagnosticMessageText(e.messageText, '\n')).join('\n')), { category: 'invalid_configuration' });
  options = { ...parsed.options, noEmit: true };
}
const program = ts.createProgram([...files].sort(), options);
const checker = program.getTypeChecker();
const declarations = new Map();
const nodes = [];
const isFunction = node => ts.isFunctionDeclaration(node) || ts.isMethodDeclaration(node) || ts.isConstructorDeclaration(node) || ts.isGetAccessorDeclaration(node) || ts.isSetAccessorDeclaration(node) || ts.isArrowFunction(node) || ts.isFunctionExpression(node);
function name(node) {
  if (ts.isConstructorDeclaration(node)) return 'constructor';
  if (node.name) return node.name.getText();
  if (ts.isVariableDeclaration(node.parent) || ts.isPropertyDeclaration(node.parent) || ts.isPropertyAssignment(node.parent)) return node.parent.name.getText();
  return '<anonymous>';
}
function register(node, owners, source) {
  let next = owners;
  if (ts.isClassDeclaration(node) || ts.isClassExpression(node)) next = [...owners, node.name?.text || '<class>'];
  if (isFunction(node) && node.body) {
    const position = source.getLineAndCharacterOfPosition(node.getStart(source));
    const symbol = [...owners, name(node)].join('.');
    const file = relative(source.fileName);
    const id = `${file}:${position.line + 1}:${position.character + 1}:${symbol}`;
    declarations.set(node, id);
    nodes.push({ id, location: { file, line: position.line + 1, column: position.character + 1, symbol } });
    next = [...owners, name(node)];
  }
  ts.forEachChild(node, child => register(child, next, source));
}
const sources = program.getSourceFiles().filter(s => files.has(path.resolve(s.fileName))).sort((a, b) => a.fileName < b.fileName ? -1 : a.fileName > b.fileName ? 1 : 0);
const syntaxErrors = program.getSyntacticDiagnostics().filter(d => d.file && files.has(path.resolve(d.file.fileName)));
if (syntaxErrors.length) throw new Error(syntaxErrors.map(d => `${relative(d.file.fileName)}: ${ts.flattenDiagnosticMessageText(d.messageText, '\n')}`).join('\n'));
for (const source of sources) register(source, [], source);
function declaredTarget(declaration) {
  if (!declaration) return undefined;
  if (declarations.has(declaration)) return declarations.get(declaration);
  if (declaration.initializer && declarations.has(declaration.initializer)) return declarations.get(declaration.initializer);
  return undefined;
}
const edges = new Map();
let unresolved = 0;
function calls(node, caller) {
  if (isFunction(node)) caller = declarations.get(node); // never attribute nested bodies to outer caller
  if (caller && (ts.isCallExpression(node) || ts.isNewExpression(node))) {
    const signature = checker.getResolvedSignature(node);
    let target = declaredTarget(signature?.declaration);
    if (!target) {
      let symbol = checker.getSymbolAtLocation(ts.isPropertyAccessExpression(node.expression) ? node.expression.name : node.expression);
      if (symbol?.flags & ts.SymbolFlags.Alias) symbol = checker.getAliasedSymbol(symbol);
      target = declaredTarget(symbol?.valueDeclaration);
      if (!target) for (const declaration of symbol?.declarations || []) {
        target = declaredTarget(declaration);
        if (target) break;
      }
    }
    if (target) edges.set(JSON.stringify([caller, target]), { caller, callee: target });
    else unresolved++;
  }
  ts.forEachChild(node, child => calls(child, caller));
}
for (const source of sources) calls(source, undefined);
nodes.sort((a, b) => a.id < b.id ? -1 : a.id > b.id ? 1 : 0);
const orderedEdges = [...edges.entries()].sort(([a], [b]) => a < b ? -1 : a > b ? 1 : 0).map(([, e]) => e);
process.stdout.write(JSON.stringify({ nodes, edges: orderedEdges, unresolved_calls: unresolved }) + '\n');

}
try { analyze(); } catch (error) {
  const category = error.category || (['MODULE_NOT_FOUND', 'ERR_MODULE_NOT_FOUND'].includes(error.code) ? 'tool_missing' : 'analyzer_failure');
  console.error(error.stack);
  process.stdout.write(JSON.stringify({ error: { category, message: error.message } }) + '\n');
}
