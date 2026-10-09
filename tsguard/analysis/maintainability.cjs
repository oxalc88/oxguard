// Extract compiler facts. Go owns policy and thresholds.
emitAnalysis(input => {
  const { ts, program, sources, relative } = loadProject(input);
  const checker = program.getTypeChecker();
  const crypto = require('node:crypto');
  const declarations = new Map();
  const functions = [];
  const handlers = [];
  const isFunction = node => ts.isFunctionDeclaration(node) || ts.isMethodDeclaration(node) || ts.isConstructorDeclaration(node) || ts.isGetAccessorDeclaration(node) || ts.isSetAccessorDeclaration(node) || ts.isArrowFunction(node) || ts.isFunctionExpression(node);
  const location = (node, source, symbol) => {
    const pos = source.getLineAndCharacterOfPosition(node.getStart(source));
    return { file: relative(source.fileName), line: pos.line + 1, column: pos.character + 1, ...(symbol ? { symbol } : {}) };
  };
  const name = node => node.name?.getText() || (ts.isVariableDeclaration(node.parent) ? node.parent.name.getText() : '<anonymous>');
  function register(node, owners, source) {
    let next = owners;
    if (ts.isClassDeclaration(node) || ts.isClassExpression(node)) next = [...owners, node.name?.text || '<class>'];
    if (isFunction(node) && node.body) {
      const loc = location(node, source, [...owners, name(node)].join('.'));
      const id = `${loc.file}:${loc.line}:${loc.column}:${loc.symbol}`;
      const fact = { id, location: loc, forward_target: '', nested_branches: 0 };
      functions.push(fact);
      declarations.set(node, fact);
      next = [...owners, name(node)];
    }
    ts.forEachChild(node, child => register(child, next, source));
  }
  for (const source of sources) register(source, [], source);
  function target(call) {
    const signature = checker.getResolvedSignature(call);
    const declaration = signature?.declaration;
    if (declarations.has(declaration)) return declarations.get(declaration);
    let symbol = checker.getSymbolAtLocation(ts.isPropertyAccessExpression(call.expression) ? call.expression.name : call.expression);
    if (symbol?.flags & ts.SymbolFlags.Alias) symbol = checker.getAliasedSymbol(symbol);
    for (const decl of symbol?.declarations || []) {
      const fact = declarations.get(decl) || declarations.get(decl.initializer);
      if (fact) return fact;
    }
  }
  const edges = new Map();
  let unresolved = 0;
  function visit(node, owner, source) {
    if (isFunction(node)) owner = declarations.get(node);
    if (owner && (ts.isCallExpression(node) || ts.isNewExpression(node))) {
      const callee = target(node);
      if (callee) edges.set(JSON.stringify([owner.id, callee.id]), { caller: owner.id, callee: callee.id });
      else unresolved++;
    }
    if (ts.isCatchClause(node)) {
      // Exact token identity, preserving identifiers/literals and failure policy.
      // Short handlers are below jscpd's normal minimum token threshold.
      const scanner = ts.createScanner(ts.ScriptTarget.Latest, true, source.languageVariant, node.block.getText(source));
      const tokens = [];
      while (scanner.scan() !== ts.SyntaxKind.EndOfFileToken) tokens.push(scanner.getTokenText());
      handlers.push({ location: location(node, source), tokens: tokens.length,
        fingerprint: crypto.createHash('sha256').update(JSON.stringify(tokens)).digest('hex') });
    }
    ts.forEachChild(node, child => visit(child, owner, source));
  }
  for (const [node, fact] of declarations) {
    // Only unchanged, synchronous positional forwarding to a resolved function.
    // Exclude DI methods, async boundaries, rest/default/optional/destructured
    // parameters and return/type transformations.
    const expression = ts.isBlock(node.body) ? (node.body.statements.length === 1 && ts.isReturnStatement(node.body.statements[0]) ? node.body.statements[0].expression : undefined) : node.body;
    if (!(ts.isFunctionDeclaration(node) || ts.isArrowFunction(node) || ts.isFunctionExpression(node)) || node.typeParameters?.length || node.modifiers?.some(m => m.kind === ts.SyntaxKind.AsyncKeyword)) continue;
    if (!expression || !ts.isCallExpression(expression) || !ts.isIdentifier(expression.expression)) continue;
    if (node.parameters.some(p => !ts.isIdentifier(p.name) || p.initializer || p.dotDotDotToken || p.questionToken)) continue;
    if (expression.arguments.length !== node.parameters.length || expression.arguments.some((a, i) => !ts.isIdentifier(a) || checker.getSymbolAtLocation(a) !== checker.getSymbolAtLocation(node.parameters[i].name))) continue;
    const callee = target(expression);
    if (!callee || callee.id === fact.id) continue;
    const callerType = checker.getSignatureFromDeclaration(node);
    const calleeType = checker.getResolvedSignature(expression);
    if (!callerType || !calleeType || checker.typeToString(checker.getReturnTypeOfSignature(callerType)) !== checker.typeToString(checker.getReturnTypeOfSignature(calleeType))) continue;
    fact.forward_target = callee.id;
  }
  for (const source of sources) visit(source, undefined, source);
  // Branch counts are compiler facts for comparison, not a new complexity score.
  function count(node, owner) {
    if (isFunction(node)) owner = declarations.get(node);
    if (owner && (ts.isIfStatement(node) || ts.isSwitchStatement(node) || ts.isConditionalExpression(node) || ts.isForStatement(node) || ts.isForOfStatement(node) || ts.isForInStatement(node) || ts.isWhileStatement(node) || ts.isDoStatement(node) || ts.isCatchClause(node))) owner.nested_branches++;
    ts.forEachChild(node, child => count(child, owner));
  }
  for (const source of sources) count(source, undefined);
  const ordered = values => values.sort((a,b) => JSON.stringify(a) < JSON.stringify(b) ? -1 : JSON.stringify(a) > JSON.stringify(b) ? 1 : 0);
  return { findings: [], measurements: [], snapshot: { functions: ordered(functions), calls: ordered([...edges.values()]), handlers: ordered(handlers), unresolved_calls: unresolved } };
});
