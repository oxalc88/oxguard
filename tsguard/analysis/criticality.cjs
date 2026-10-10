'use strict';
// Compiler-backed analyzer only: emits nodes/edges. Go owns ranking, policy,
// reporting and artifacts. This is embedded in the native binary, not a launcher.
function analyze() {
const input = JSON.parse(process.argv[1]);
const { ts, program, sources, relative } = loadProject(input);
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
