// Extract compiler facts. Go owns policy and thresholds.
emitAnalysis(input => {
  const { ts, program, sources, relative } = loadProject(input);
  const checker = program.getTypeChecker();
  const crypto = require('node:crypto');
  const declarations = new Map();
  const functions = [];
  const handlers = [];
  const modules = sources.map(s => ({ id: relative(s.fileName), location: { file: relative(s.fileName) } }));
  const moduleIDs = new Set(modules.map(m => m.id));
  const imports = new Map();
  let externalImports = 0, unresolvedImports = 0, typeImports = 0;
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
      const statement = node.block.statements.length === 1 ? node.block.statements[0] : undefined;
      const fallback = statement && ts.isReturnStatement(statement) && (!statement.expression || ts.isLiteralExpression(statement.expression) || [ts.SyntaxKind.NullKeyword, ts.SyntaxKind.FalseKeyword, ts.SyntaxKind.TrueKeyword].includes(statement.expression.kind));
      handlers.push({ location: location(node, source), tokens: tokens.length, fallback: !!fallback,
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
  function dependency(node, source) {
    let specifier;
    let typeOnly = false;
    if (ts.isImportDeclaration(node) || ts.isExportDeclaration(node)) {
      specifier = node.moduleSpecifier;
      const clause = node.importClause || node.exportClause;
      typeOnly = node.isTypeOnly || clause?.isTypeOnly || (!clause?.name && clause?.namedBindings && ts.isNamedImports(clause.namedBindings) && clause.namedBindings.elements.length > 0 && clause.namedBindings.elements.every(e => e.isTypeOnly)) || (clause && ts.isNamedExports(clause) && clause.elements.length > 0 && clause.elements.every(e => e.isTypeOnly));
    } else if (ts.isImportEqualsDeclaration(node) && ts.isExternalModuleReference(node.moduleReference)) {
      specifier = node.moduleReference.expression;
      typeOnly = node.isTypeOnly;
    } else if (ts.isCallExpression(node) && node.arguments.length > 0) {
      const expr = node.expression;
      if (expr.kind === ts.SyntaxKind.ImportKeyword) specifier = node.arguments[0];
      if (ts.isIdentifier(expr) && expr.text === 'require') {
        const decl = checker.getSymbolAtLocation(expr)?.valueDeclaration;
        if (!decl || decl.getSourceFile().isDeclarationFile) specifier = node.arguments[0];
      }
    }
    if (specifier) {
      if (typeOnly) typeImports++;
      else if (!ts.isStringLiteralLike(specifier)) unresolvedImports++;
      else {
        const resolved = ts.resolveModuleName(specifier.text, source.fileName, program.getCompilerOptions(), ts.sys).resolvedModule;
        const callee = resolved ? relative(resolved.resolvedFileName) : undefined;
        const caller = relative(source.fileName);
        if (moduleIDs.has(callee)) imports.set(JSON.stringify([caller,callee]), { id: JSON.stringify([caller,callee]), caller, callee });
        else if (!resolved && specifier.text.startsWith('.')) unresolvedImports++;
        else externalImports++;
      }
    }
    ts.forEachChild(node, child => dependency(child, source));
  }
  for (const source of sources) dependency(source, source);
  // Branch counts are compiler facts for comparison, not a new complexity score.
  function count(node, owner) {
    if (isFunction(node)) owner = declarations.get(node);
    if (owner && (ts.isIfStatement(node) || ts.isSwitchStatement(node) || ts.isConditionalExpression(node) || ts.isForStatement(node) || ts.isForOfStatement(node) || ts.isForInStatement(node) || ts.isWhileStatement(node) || ts.isDoStatement(node) || ts.isCatchClause(node))) owner.nested_branches++;
    ts.forEachChild(node, child => count(child, owner));
  }
  for (const source of sources) count(source, undefined);
  const ordered = values => values.sort((a,b) => JSON.stringify(a) < JSON.stringify(b) ? -1 : JSON.stringify(a) > JSON.stringify(b) ? 1 : 0);
  return { findings: [], measurements: [], partial: unresolvedImports > 0,
    limitation: unresolvedImports > 0 ? `${unresolvedImports} local or dynamic module references could not be resolved; the graph is partial.` : '',
    snapshot: { functions: ordered(functions), calls: ordered([...edges.values()]), handlers: ordered(handlers), unresolved_calls: unresolved,
      modules: ordered(modules), imports: ordered([...imports.values()]), external_imports: externalImports, unresolved_imports: unresolvedImports, type_imports: typeImports } };
});
