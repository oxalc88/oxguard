"""Build a PyGuard runtime module graph from importable project packages."""

from __future__ import annotations

import ast
import builtins
import fnmatch
import io
import json
import os
import sys
import tempfile
import tokenize
import hashlib
from pathlib import Path

# Python packages grimp cannot infer for top-level modules are represented as
# single-module FoundPackages. Package files are grouped by their actual import
# root so relative imports resolve exactly as Python does.
TEST_FILE_PATTERNS = ("test_*.py", "*_test.py", "conftest.py")
TEST_DIRECTORY_NAMES = {"test", "tests"}
BUILTIN_NAMES = frozenset(dir(builtins))
ENVIRONMENT_DIRECTORY_NAMES = {
    "__pycache__",
    ".git",
    ".hg",
    ".nox",
    ".svn",
    ".tox",
    ".venv",
    "node_modules",
    "site-packages",
    "venv",
}


class StructureError(Exception):
    def __init__(self, category: str, message: str) -> None:
        super().__init__(message)
        self.category = category
        self.message = message


def is_type_checking(node: ast.AST) -> bool:
    """Match grimp: TYPE_CHECKING or <anything>.TYPE_CHECKING."""
    if isinstance(node, ast.Name):
        return node.id == "TYPE_CHECKING"
    if isinstance(node, ast.Attribute):
        return node.attr == "TYPE_CHECKING"
    return False


def exclusion_patterns() -> list[str]:
    patterns: list[str] = []
    if os.environ.get("PYGUARD_EXCLUDE_TESTS") == "1":
        patterns.extend(TEST_FILE_PATTERNS)
    raw = os.environ.get("PYGUARD_EXCLUDE_GLOBS", "")
    patterns.extend(item.strip() for item in raw.split(",") if item.strip())
    return patterns


def relative_path(path: Path, root: Path) -> str:
    try:
        return path.resolve().relative_to(root).as_posix()
    except (OSError, ValueError):
        return path.as_posix()


def excluded(path: Path, root: Path, patterns: list[str], exclude_tests: bool) -> bool:
    rel = relative_path(path, root)
    name = path.name
    for pattern in patterns:
        normalized = pattern.replace("\\", "/").strip("/")
        if not normalized:
            continue
        if fnmatch.fnmatchcase(name, normalized):
            return True
        if fnmatch.fnmatchcase(rel, normalized):
            return True
        if rel == normalized or rel.startswith(normalized + "/"):
            return True
    if exclude_tests:
        parts = set(rel.split("/")[:-1])
        if parts & TEST_DIRECTORY_NAMES:
            return True
    return False


def skipped_directory(path: Path) -> bool:
    """Dependency, hidden and build trees are never part of local resolution."""
    name = path.name
    if name.startswith("."):
        return True
    return name in ENVIRONMENT_DIRECTORY_NAMES or name in {"build", "dist"}


def walk_python_files(root: Path) -> list[Path]:
    files: list[Path] = []
    for directory, names, filenames in os.walk(root):
        names[:] = sorted(name for name in names if not skipped_directory(Path(name)))
        for filename in sorted(filenames):
            if filename.endswith(".py"):
                files.append((Path(directory) / filename).resolve())
    return files


def project_import_roots(project_root: Path) -> list[Path]:
    """Return the fixed import roots defined by the project layout.

    A conventional ``<project>/src`` directory is its own import root, so
    ``src/pkg/a.py`` is ``pkg.a`` in every scope, including the default ``.``
    scope and a narrowed ``--dirs`` path inside ``src``. Every other file is
    imported relative to the project root. Roots never depend on which graph
    nodes are requested or filtered out.
    """
    roots = [project_root]
    source_root = project_root / "src"
    if source_root.is_dir():
        roots.append(source_root)
    return roots


def collect_candidates(targets: list[str]) -> set[Path]:
    """Return every Python file in the explicitly configured scopes."""
    candidates: set[Path] = set()
    for target in targets or ["."]:
        path = Path(target)
        if not path.exists():
            raise StructureError(
                "invalid_configuration", f"Source path not found: {path}"
            )
        if path.is_file():
            if path.suffix == ".py" and "__pycache__" not in path.resolve().parts:
                candidates.add(path.resolve())
            continue
        candidates.update(walk_python_files(path.resolve()))
    return candidates


def module_name(path: Path, import_root: Path) -> str:
    parts = list(path.relative_to(import_root).with_suffix("").parts)
    if parts and parts[-1] == "__init__":
        parts = parts[:-1]
    return ".".join(parts)


def nearest_import_root(path: Path, roots: list[Path]) -> Path:
    containing = [root for root in roots if path == root or root in path.parents]
    if not containing:
        return path.parent
    return max(containing, key=lambda root: (len(root.parts), str(root)))


def build_inventory(
    roots: list[Path],
) -> tuple[
    dict[Path, tuple[str, Path, bool]],
    dict[str, tuple[Path | None, Path, bool]],
    set[str],
    list[tuple[str, Path, Path]],
]:
    """Map every file under the import roots to one fixed module identity.

    Scope and exclusion filters deliberately do not participate here: an
    out-of-scope file still makes its module resolvable, so it can be recognized
    as a scope boundary rather than a missing module. Directory prefixes of
    files are implicit namespace packages even without an ``__init__.py``.
    """
    identities: dict[Path, tuple[str, Path, bool]] = {}
    inventory: dict[str, tuple[Path | None, Path, bool]] = {}
    collisions: list[tuple[str, Path, Path]] = []
    files = sorted({path for root in roots for path in walk_python_files(root)})
    for path in files:
        import_root = nearest_import_root(path, roots)
        try:
            name = module_name(path, import_root)
        except ValueError:
            import_root = path.parent
            name = path.stem
        name = name or path.stem
        is_package = path.name == "__init__.py"
        identities[path] = (name, import_root, is_package)
        if name in inventory:
            retained_path = inventory[name][0]
            assert retained_path is not None
            collisions.append((name, retained_path, path))
            continue
        inventory[name] = (path, import_root, is_package)

    namespace_packages: set[str] = set()
    for path, (_, import_root, _) in identities.items():
        try:
            parents = path.parent.relative_to(import_root).parts
        except ValueError:
            continue
        for index in range(1, len(parents) + 1):
            name = ".".join(parents[:index])
            if name and name not in inventory:
                namespace_packages.add(name)
    for name in sorted(namespace_packages):
        inventory[name] = (None, roots[0], True)
    return identities, inventory, namespace_packages, collisions


def read_tree(path: Path) -> ast.Module:
    # Parse the on-disk bytes so Python's own detection honors a UTF-8 BOM and
    # PEP 263 encoding declarations. Decoding as UTF-8 first rejects valid
    # sources and hides their imports from the graph.
    try:
        source = path.read_bytes()
        return ast.parse(source, filename=str(path))
    except (OSError, UnicodeError, SyntaxError) as error:
        raise StructureError(
            "adapter_failure", f"Unparsable source: {path}: {error}"
        ) from error


def failure_handlers(path: Path, tree: ast.Module) -> list[dict[str, object]]:
    """Return syntactic exception-handler policy facts for Go normalization."""
    try:
        with tokenize.open(path) as source_file:
            source = source_file.read()
    except (OSError, UnicodeError, SyntaxError) as error:
        raise StructureError(
            "adapter_failure", f"Unparsable source: {path}: {error}"
        ) from error
    # StringIO.readline follows Python's physical newline rules (LF, CRLF and
    # CR); unlike str.splitlines(), it does not treat Unicode separators as
    # source line boundaries.
    try:
        source_tokens = list(tokenize.generate_tokens(io.StringIO(source).readline))
    except tokenize.TokenError:
        source_tokens = []
    token_lines = {token.start[0]: token.line for token in source_tokens if token.line}

    def character_column(row: int, byte_column: int) -> int:
        line = token_lines.get(row, "")
        return len(line.encode("utf-8")[:byte_column].decode("utf-8"))

    facts: list[dict[str, object]] = []
    for node in ast.walk(tree):
        if not isinstance(node, (ast.Try, getattr(ast, "TryStar", ast.Try))):
            continue
        for handler in node.handlers:
            body = handler.body
            fallback = False
            exit_policy = False
            if len(body) == 1 and not isinstance(body[0], ast.Pass):
                statement = body[0]
                if isinstance(statement, ast.Return):
                    value = statement.value
                    if value is None:
                        fallback = True
                    elif isinstance(value, ast.Constant):
                        fallback = True
                    elif isinstance(value, ast.Name):
                        fallback = value.id != handler.name
                    elif isinstance(value, (ast.List, ast.Tuple, ast.Dict)) and not getattr(value, "elts", []) and not getattr(value, "keys", []):
                        fallback = True
                elif isinstance(statement, (ast.Continue, ast.Break)):
                    exit_policy = True
            def broad_type(item: ast.AST) -> bool:
                return ((isinstance(item, ast.Name) and item.id in {"Exception", "BaseException"}) or
                        (isinstance(item, ast.Attribute) and isinstance(item.value, ast.Name)
                         and item.value.id == "builtins" and item.attr in {"Exception", "BaseException"}) or
                        (isinstance(item, ast.Tuple) and any(broad_type(child) for child in item.elts)))
            broad = handler.type is None or broad_type(handler.type)
            tokens: list[str] = []
            if body and source_tokens:
                # Locate the terminating colon of this except header while
                # respecting bracketed/multiline exception expressions.
                depth = 0
                colon_end: tuple[int, int] | None = None
                for token in source_tokens:
                    if token.start < (handler.lineno, 0):
                        continue
                    if token.type == tokenize.OP:
                        if token.string in "([{":
                            depth += 1
                        elif token.string in ")]}":
                            depth = max(0, depth - 1)
                        elif token.string == ":" and depth == 0:
                            colon_end = token.end
                            break
                end_line = handler.end_lineno or body[-1].end_lineno or body[-1].lineno
                end_byte_col = handler.end_col_offset
                end_col = (
                    character_column(end_line, end_byte_col)
                    if end_byte_col is not None
                    else (body[-1].end_col_offset or 0)
                )
                end_position = (end_line, end_col)
                dropped = {
                    tokenize.COMMENT,
                    tokenize.NL,
                    tokenize.NEWLINE,
                    tokenize.INDENT,
                    tokenize.DEDENT,
                    tokenize.ENCODING,
                    tokenize.ENDMARKER,
                }
                if colon_end is not None:
                    tokens = [
                        token.string
                        for token in source_tokens
                        if token.start >= colon_end
                        and token.end <= end_position
                        and token.type not in dropped
                    ]
            facts.append(
                {
                    "file": path.as_posix(),
                    "line": handler.lineno,
                    "fallback": fallback and broad,
                    "exit": exit_policy and broad,
                    "token_count": len(tokens),
                    "fingerprint": hashlib.sha256(
                        "\0".join(tokens).encode("utf-8")
                    ).hexdigest(),
                }
            )
    return facts


def absolute_import_base(
    node: ast.ImportFrom, module_name: str, is_package: bool
) -> str:
    if node.level == 0:
        return node.module or ""
    parts = module_name.split(".")
    keep = len(parts) - node.level + (1 if is_package else 0)
    if keep < 1:
        return ""
    base = ".".join(parts[:keep])
    if node.module:
        return f"{base}.{node.module}" if base else node.module
    return base


# Attributes Python supplies for imported modules and packages. They are valid
# ``from package import name`` targets even though no source statement binds
# them.
IMPLICIT_MODULE_ATTRIBUTES = frozenset(
    {
        "__all__",
        "__builtins__",
        "__cached__",
        "__dict__",
        "__doc__",
        "__file__",
        "__loader__",
        "__name__",
        "__package__",
        "__path__",
        "__spec__",
    }
)


def match_pattern_names(pattern: ast.AST) -> set[str]:
    """Return names bound by a structural pattern match."""
    names: set[str] = set()
    for node in ast.walk(pattern):
        if isinstance(node, ast.MatchAs) and node.name:
            names.add(node.name)
        elif isinstance(node, ast.MatchStar) and node.name:
            names.add(node.name)
        elif isinstance(node, ast.MatchMapping) and node.rest:
            names.add(node.rest)
    return names


def contains_dynamic_export(node: ast.AST) -> bool:
    """Detect globals()/setattr() calls that can create arbitrary names."""
    for child in ast.walk(node):
        if not isinstance(child, ast.Call):
            continue
        func = child.func
        if isinstance(func, ast.Name) and func.id in {"globals", "setattr"}:
            return True
        if isinstance(func, ast.Attribute) and func.attr == "setattr":
            return True
    return False


def module_exports(tree: ast.Module) -> tuple[set[str], bool]:
    """Return a deliberate over-approximation of a package's exports.

    A name counts as exported when any statement in the module could bind it at
    runtime, including ordinary conditional branches and any nesting level.
    This keeps a valid conditional import from becoming incomplete evidence;
    only a name that is clearly absent makes the result incomplete. TYPE_CHECKING
    bodies and annotation-only assignments are excluded because neither creates
    a runtime module attribute.
    """
    exports: set[str] = set(IMPLICIT_MODULE_ATTRIBUTES)
    dynamic = False

    def visit(node: ast.AST) -> None:
        nonlocal dynamic
        if isinstance(node, ast.If) and is_type_checking(node.test):
            # The test still runs; only the guarded body is unreachable at
            # runtime. The else branch, including an elif chain, remains.
            visit(node.test)
            for statement in node.orelse:
                visit(statement)
            return
        if isinstance(node, ast.AnnAssign) and node.value is None:
            # `name: Type` does not bind name at runtime. Keep looking at the
            # annotation for walrus/dynamic expressions, but do not export the
            # target.
            visit(node.annotation)
            return
        if isinstance(node, ast.Name) and isinstance(node.ctx, ast.Store):
            exports.add(node.id)
        elif isinstance(node, (ast.FunctionDef, ast.AsyncFunctionDef, ast.ClassDef)):
            exports.add(node.name)
        elif isinstance(node, ast.Import):
            for alias in node.names:
                exports.add(alias.asname or alias.name.split(".", 1)[0])
        elif isinstance(node, ast.ImportFrom):
            if any(alias.name == "*" for alias in node.names):
                dynamic = True
            for alias in node.names:
                if alias.name != "*":
                    exports.add(alias.asname or alias.name)
        elif isinstance(node, ast.ExceptHandler) and node.name:
            exports.add(node.name)
        elif isinstance(node, ast.Match):
            for case in node.cases:
                exports.update(match_pattern_names(case.pattern))
        elif isinstance(node, ast.Call) and contains_dynamic_export(node):
            dynamic = True
        for child in ast.iter_child_nodes(node):
            visit(child)

    visit(tree)
    # A module-level __getattr__ supplies arbitrary attributes even when it is
    # imported or assigned rather than defined by a function statement.
    if "__getattr__" in exports:
        dynamic = True
    return exports, dynamic


def scan_ast(
    tree: ast.Module,
    module_name: str,
    is_package: bool,
    inventory_names: set[str],
    included_names: set[str],
    package_flags: dict[str, bool],
    namespace_packages: set[str],
    package_exports: dict[str, set[str]],
    dynamic_exports: dict[str, bool],
) -> tuple[
    int,
    int,
    set[int],
    list[tuple[int, str]],
    dict[int, set[str]],
    dict[int, set[str]],
]:
    """Count type-only/deferred imports and resolve local import statements.

    Deferred means an import inside a function or method body. Class bodies run
    at import time and stay runtime imports. The traversal mirrors grimp's
    TYPE_CHECKING handling, including elif/else branches. ``from package import
    name`` validates ``name`` as either a submodule or a real package export.
    """
    type_imports = 0
    deferred_imports = 0
    deferred_lines: set[int] = set()
    unresolved: list[tuple[int, str]] = []
    missing_by_line: dict[int, set[str]] = {}
    valid_by_line: dict[int, set[str]] = {}
    local_roots = {name.split(".", 1)[0] for name in inventory_names}

    def local(target: str) -> bool:
        return bool(target) and target.split(".", 1)[0] in local_roots

    def missing(line: int, target: str) -> None:
        unresolved.append((line, target))
        missing_by_line.setdefault(line, set()).add(target)

    def exact_import(target: str, line: int) -> None:
        if target in inventory_names:
            valid_by_line.setdefault(line, set()).add(target)
            return
        if local(target):
            missing(line, target)

    def from_import(base: str, node: ast.ImportFrom, line: int) -> None:
        if not base:
            missing(line, base)
            return
        if base not in inventory_names:
            if local(base):
                missing(line, base)
            return

        if not package_flags.get(base, False):
            valid_by_line.setdefault(line, set()).add(base)
            return

        for alias in node.names:
            if alias.name == "*":
                valid_by_line.setdefault(line, set()).add(base)
                continue
            if alias.name in IMPLICIT_MODULE_ATTRIBUTES:
                # Python supplies these attributes on every imported module or
                # package. They are valid ``from package import name`` targets
                # even though no source statement binds them.
                valid_by_line.setdefault(line, set()).add(base)
                continue
            target = f"{base}.{alias.name}"
            if target in inventory_names:
                # Validate against the full inventory before the graph-scope
                # boundary is applied. An out-of-scope child is valid, while a
                # nonexistent child remains unresolved even when its package
                # parent exists or is outside the selected graph nodes.
                valid_by_line.setdefault(line, set()).add(target)
            elif alias.name in package_exports.get(
                base, set()
            ) or dynamic_exports.get(base, False):
                valid_by_line.setdefault(line, set()).add(base)
            else:
                # Implicit namespace packages have no __init__ attributes, so
                # every imported name must be a real child module or package.
                missing(line, target)

    def visit_body(body: list[ast.stmt], in_tc: bool, in_function: bool) -> None:
        for statement in body:
            visit(statement, in_tc, in_function)

    def visit(node: ast.AST, in_tc: bool, in_function: bool) -> None:
        nonlocal type_imports, deferred_imports
        if isinstance(node, (ast.FunctionDef, ast.AsyncFunctionDef)):
            visit_body(node.body, in_tc, True)
            return
        if isinstance(node, ast.If):
            visit_body(node.body, in_tc or is_type_checking(node.test), in_function)
            if len(node.orelse) == 1 and isinstance(node.orelse[0], ast.If):
                visit(node.orelse[0], in_tc, in_function)
            else:
                visit_body(node.orelse, in_tc, in_function)
            return
        if isinstance(node, ast.Import):
            if in_tc:
                type_imports += 1
            else:
                if in_function:
                    deferred_imports += 1
                    deferred_lines.add(node.lineno)
                for alias in node.names:
                    exact_import(alias.name, node.lineno)
            return
        if isinstance(node, ast.ImportFrom):
            if in_tc:
                type_imports += 1
            else:
                if in_function:
                    deferred_imports += 1
                    deferred_lines.add(node.lineno)
                from_import(
                    absolute_import_base(node, module_name, is_package),
                    node,
                    node.lineno,
                )
            return
        for child in ast.iter_child_nodes(node):
            visit(child, in_tc, in_function)

    visit_body(tree.body, False, False)
    return (
        type_imports,
        deferred_imports,
        deferred_lines,
        unresolved,
        missing_by_line,
        valid_by_line,
    )


def build_graph(
    module_files: dict[str, Path],
    inventory: dict[str, tuple[Path | None, Path, bool]],
    deferred_by_module: dict[str, set[int]],
    missing_by_module: dict[str, dict[int, set[str]]],
    valid_by_module: dict[str, dict[int, set[str]]],
) -> tuple[list[tuple[str, str]], list[list[str]], dict[str, int], int, str]:
    from grimp.application.ports.modulefinder import FoundPackage, ModuleFile
    from grimp.application.scanning import scan_imports
    from grimp.domain.valueobjects import Module

    groups: dict[tuple[str, str], dict[str, object]] = {}
    bare: dict[str, Path] = {}
    for name, (path, import_root, is_package) in inventory.items():
        if path is None:
            # Implicit namespace packages participate in resolution but have
            # no module file to scan or emit as a graph node.
            continue
        if is_package or "." in name:
            top = name.split(".", 1)[0]
            key = (str(import_root), top)
            group = groups.setdefault(
                key, {"directory": import_root / top, "files": []}
            )
            files = group["files"]
            assert isinstance(files, list)
            files.append((name, path))
        else:
            bare[name] = path

    found_packages: set[FoundPackage] = set()
    module_files_by_name: dict[str, ModuleFile] = {}
    for key, group in groups.items():
        directory = group["directory"]
        assert isinstance(directory, Path)
        entries = group["files"]
        assert isinstance(entries, list)
        files = frozenset(
            ModuleFile(Module(name), path.stat().st_mtime)
            for name, path in entries
        )
        found_packages.add(
            FoundPackage(
                name=key[1],
                directory=str(directory),
                module_files=files,
            )
        )
        for module_file in files:
            module_files_by_name[module_file.module.name] = module_file
    for name, path in bare.items():
        module_file = ModuleFile(Module(name), path.stat().st_mtime)
        found_packages.add(
            FoundPackage(
                name=name,
                directory=str(path.with_suffix("")),
                module_files=frozenset({module_file}),
            )
        )
        module_files_by_name[name] = module_file

    # Inventory includes excluded modules so grimp resolves an excluded target
    # exactly instead of falling back to its package. Only retained modules are
    # emitted as graph nodes. A module with an unresolved local import is
    # withheld from grimp entirely: the import is already known to be invalid
    # (including relative imports beyond the top-level package), and passing it
    # to grimp 3.18 can raise a PanicException that is not an Exception.
    unresolved_modules = {
        name for name, missing_lines in missing_by_module.items() if missing_lines
    }
    scan_files = [
        module_files_by_name[name]
        for name in sorted(module_files)
        if name not in unresolved_modules
    ]
    included_names = set(module_files)
    edges: set[tuple[str, str]] = set()
    resolver_reason = ""
    try:
        scanned = scan_imports(
            scan_files,
            found_packages=found_packages,
            include_external_packages=False,
            exclude_type_checking_imports=True,
        )
        for module_file, imports in scanned.items():
            name = module_file.module.name
            deferred = deferred_by_module.get(name, set())
            missing_lines = missing_by_module.get(name, {})
            valid_lines = valid_by_module.get(name, {})
            for direct_import in imports:
                if direct_import.line_number in deferred:
                    continue
                imported = direct_import.imported.name
                if imported not in included_names:
                    continue
                line = direct_import.line_number
                missing = missing_lines.get(line, set())
                valid = valid_lines.get(line, set())
                # A missing ``from . import name`` can make grimp fall back to
                # the package. Drop that invented parent edge unless the same
                # statement also resolved a legitimate package attribute.
                if imported not in valid and any(
                    imported == target or target.startswith(imported + ".")
                    for target in missing
                ):
                    continue
                key = (direct_import.importer.name, imported)
                if key in edges:
                    continue
                edges.add(key)
    except BaseException as error:
        # grimp's Rust boundary raises PanicException (a BaseException) for
        # source shapes it cannot resolve. That is incomplete source evidence,
        # not an analyzer execution failure: retain module nodes with no edges
        # and report the resolver limitation as not_evaluated.
        edges.clear()
        resolver_reason = (
            "Structure resolver could not evaluate imports "
            f"({type(error).__name__}); graph edges are omitted."
        )

    import networkx as nx

    module_names = sorted(module_files)
    dag = nx.DiGraph()
    dag.add_nodes_from(module_names)
    dag.add_edges_from(sorted(edges))
    components = [
        set(component) for component in nx.strongly_connected_components(dag)
    ]
    condensation = nx.condensation(dag, scc=components)
    depths = {node: 0 for node in condensation.nodes}
    for component in reversed(list(nx.topological_sort(condensation))):
        depths[component] = max(
            (1 + depths[target] for target in condensation.successors(component)),
            default=0,
        )
    mapping = condensation.graph["mapping"]
    module_depth = {module: depths[mapping[module]] for module in module_names}
    ordered_components = sorted(
        (sorted(component) for component in components),
        key=lambda component: tuple(module_files[name] for name in component),
    )
    cycles = sum(
        1
        for component in ordered_components
        if len(component) > 1
        or (component[0], component[0]) in edges
    )
    return sorted(edges), ordered_components, module_depth, cycles, resolver_reason


def import_bound_names(tree: ast.Module) -> set[str]:
    """Return every name bound by an import statement anywhere in the module.

    This is a syntactic boundary, not scope resolution: a name imported by any
    statement in the module marks a call chain as an imported/external API.
    Star imports bind no specific name and therefore contribute nothing.
    """
    names: set[str] = set()
    for node in ast.walk(tree):
        if isinstance(node, ast.Import):
            for alias in node.names:
                names.add(alias.asname or alias.name.split(".", 1)[0])
        elif isinstance(node, ast.ImportFrom):
            for alias in node.names:
                if alias.name != "*":
                    names.add(alias.asname or alias.name)
    return names


def call_target_names(func: ast.expr) -> tuple[str | None, str | None]:
    """Return the terminal and root names of a Name/Attribute call target.

    A subscript, call result or other expression has no terminal name and is
    therefore never resolvable by this syntactic rule.
    """
    if isinstance(func, ast.Name):
        return func.id, func.id
    if isinstance(func, ast.Attribute):
        current = func.value
        while isinstance(current, ast.Attribute):
            current = current.value
        root = current.id if isinstance(current, ast.Name) else None
        return func.attr, root
    return None, None


def call_is_resolved(
    func: ast.expr, callee_names: set[str], imported_names: set[str]
) -> bool:
    """Decide one call site from names alone; there is no alias/dataflow tracking."""
    terminal, root = call_target_names(func)
    if terminal is None:
        return False
    if terminal in callee_names:
        return True
    if isinstance(func, ast.Name) and terminal in BUILTIN_NAMES:
        return True
    return root is not None and root in imported_names


def own_scope_calls(
    node: ast.FunctionDef | ast.AsyncFunctionDef,
) -> list[ast.Call]:
    """Visit calls in one function's own scope, never its nested definitions.

    Nested functions and classes are their own callers, so their bodies are not
    descended into. A nested definition's header (decorators, defaults and
    annotations) executes in the enclosing scope and is visited. Lambda and
    comprehension bodies have no caller of their own and stay in this scope.
    """
    calls: list[ast.Call] = []

    def visit(current: ast.AST) -> None:
        if isinstance(current, (ast.FunctionDef, ast.AsyncFunctionDef, ast.ClassDef)):
            body_ids = {id(statement) for statement in current.body}
            for child in ast.iter_child_nodes(current):
                if id(child) not in body_ids:
                    visit(child)
            return
        if isinstance(current, ast.Call):
            calls.append(current)
        for child in ast.iter_child_nodes(current):
            visit(child)

    for statement in node.body:
        visit(statement)
    return calls


def module_level_function_ids(tree: ast.Module) -> set[int]:
    """Collect function definitions whose enclosing scope is the module.

    Definitions under module-level ``if``/``try``/``with``/loop statements are
    still module-level. Class and function bodies belong to another scope and
    are never descended into.
    """
    identifiers: set[int] = set()

    def visit(node: ast.AST) -> None:
        if isinstance(node, (ast.FunctionDef, ast.AsyncFunctionDef)):
            identifiers.add(id(node))
            return
        if isinstance(node, ast.ClassDef):
            return
        for child in ast.iter_child_nodes(node):
            visit(child)

    for statement in tree.body:
        visit(statement)
    return identifiers


def forwarding_fact(
    node: object,
    module_scope: object,
    function_scope: object,
    module_level: set[int],
    in_scope: set[object],
) -> tuple[str, str] | None:
    """Return (mode, target symbol) for one unchanged forwarding wrapper.

    Only a module-level, undecorated ``def``/``async def`` with plain
    positional-or-keyword parameters and exactly one ``return <Call>`` can
    forward. A leading docstring is allowed. The call target is the pyan3
    module binding for the returned name, unless that name is shadowed in the
    wrapper parameter or local. It must be an in-scope function other than the wrapper
    itself, and its return annotation must be textually identical. ``async def`` forwards only
    as ``return await b(...)`` to another ``async def``.
    """
    definition = getattr(node, "ast_node", None)
    if not isinstance(definition, (ast.FunctionDef, ast.AsyncFunctionDef)):
        return None
    if id(definition) not in module_level or definition.decorator_list:
        return None
    arguments = definition.args
    if (
        arguments.posonlyargs
        or arguments.vararg
        or arguments.kwarg
        or arguments.kwonlyargs
        or arguments.defaults
        or arguments.kw_defaults
    ):
        return None
    body = list(definition.body)
    if (
        body
        and isinstance(body[0], ast.Expr)
        and isinstance(body[0].value, ast.Constant)
        and isinstance(body[0].value.value, str)
    ):
        body = body[1:]
    if len(body) != 1 or not isinstance(body[0], ast.Return):
        return None
    returned = body[0].value
    if returned is None:
        return None
    is_async = isinstance(definition, ast.AsyncFunctionDef)
    if is_async:
        if not isinstance(returned, ast.Await) or not isinstance(
            returned.value, ast.Call
        ):
            return None
        call = returned.value
    else:
        if not isinstance(returned, ast.Call):
            return None
        call = returned
    if not isinstance(call.func, ast.Name) or call.keywords:
        return None
    parameters = [argument.arg for argument in arguments.args]
    if len(call.args) != len(parameters):
        return None
    if any(
        not isinstance(argument, ast.Name) or argument.id != parameter
        for argument, parameter in zip(call.args, parameters)
    ):
        return None
    # Use the returned Name's binding rather than use_edges, which also
    # contains annotation/default references and loses import alias names.
    name = call.func.id
    parameter_names = {
        argument.arg
        for argument in (
            *arguments.posonlyargs,
            *arguments.args,
            *arguments.kwonlyargs,
        )
    }
    if arguments.vararg is not None:
        parameter_names.add(arguments.vararg.arg)
    if arguments.kwarg is not None:
        parameter_names.add(arguments.kwarg.arg)
    if name in parameter_names or name in getattr(function_scope, "locals", set()):
        return None
    target = getattr(module_scope, "defs", {}).get(name)
    if target not in in_scope or target is node:
        return None
    if not isinstance(getattr(target, "ast_node", None), (ast.FunctionDef, ast.AsyncFunctionDef)):
        return None
    if is_async and not isinstance(target.ast_node, ast.AsyncFunctionDef):
        return None
    if (definition.returns is None) != (target.ast_node.returns is None):
        return None
    if definition.returns is not None and ast.unparse(
        definition.returns
    ) != ast.unparse(target.ast_node.returns):
        return None
    return ("async-return-await" if is_async else "sync", target.get_name())


def empty_function_graph() -> dict[str, object]:
    return {
        "functions": [],
        "calls": [],
        "function_components": [],
        "function_counts": {
            "functions": 0,
            "calls": 0,
            "recursive_components": 0,
            "unresolved_calls": 0,
        },
    }


def needs_utf8_mirror(data: bytes) -> bool:
    """Decide from Python's own detection whether pyan3 would misread bytes.

    A source the structure parser accepted has a valid PEP 263 encoding, and
    that encoding is not necessarily UTF-8 just because the bytes also happen
    to decode as UTF-8. pyan3 always opens files as UTF-8, so a Latin-1 file
    that is coincidentally valid UTF-8 would expose different identifiers and
    could collapse distinct functions after Python's NFKC normalization.
    """
    try:
        encoding, _ = tokenize.detect_encoding(io.BytesIO(data).readline)
    except (SyntaxError, UnicodeDecodeError):
        # The structure parser already accepted the same bytes, so this is
        # unexpected. Mirroring cannot lose information and is the safe answer.
        return True
    return encoding != "utf-8"


def stage_pyan_sources(
    files: list[str],
) -> tuple[
    list[str],
    dict[str, str],
    tempfile.TemporaryDirectory[str] | None,
    str | None,
]:
    """Mirror sources as UTF-8 when pyan3 would not read their declared text.

    pyan3 opens every file as UTF-8, so a UTF-8 BOM or a valid PEP 263 source
    declaration makes it drop the file (or fail). Successful UTF-8 decoding is
    not sufficient evidence: a Latin-1 source can also decode as UTF-8 while
    pyan3 would see different characters and Python would then normalize two
    distinct identifiers together. ``tokenize.detect_encoding`` mirrors every
    source whose declared or BOM encoding is not plain UTF-8, decoding with
    that detected encoding. The structure parser already accepted those files.
    A temporary mirror keeps line numbers and the module layout relative to
    pyan3's original inferred import root, and the original paths remain the
    reported locations. The mirror root is returned so the visitor uses that
    same root even when ``__init__.py`` ancestors were not in scope and
    therefore were not copied.
    """
    needs_copy = any(
        needs_utf8_mirror(Path(path).read_bytes()) for path in files
    )
    if not needs_copy:
        return files, {}, None, None

    from pyan.anutils import infer_root

    try:
        # This is exactly the root pyan3 would infer for the original files.
        import_root = Path(infer_root(files))
    except (OSError, ValueError):
        import_root = Path(os.path.abspath(os.path.commonpath(files)))
        if not import_root.is_dir():
            import_root = import_root.parent
    temporary = tempfile.TemporaryDirectory(prefix="pyguard-structure-functions-")
    staged_root = Path(temporary.name)
    staged: list[str] = []
    mapping: dict[str, str] = {}
    for path in files:
        source = Path(path).read_bytes()
        encoding, _ = tokenize.detect_encoding(io.BytesIO(source).readline)
        text = source.decode(encoding)
        try:
            relative = Path(os.path.abspath(path)).relative_to(import_root)
        except ValueError:
            relative = Path(path).name
        target = staged_root / relative
        target.parent.mkdir(parents=True, exist_ok=True)
        target.write_bytes(text.encode("utf-8"))
        staged.append(str(target))
        mapping[str(target)] = path
    return staged, mapping, temporary, str(staged_root)


def build_function_graph(
    files: list[str], root: Path
) -> tuple[dict[str, object], list[str]]:
    """Build the function graph, normalizing only sources pyan3 would misread."""
    if not files:
        return empty_function_graph(), []
    staged, source_map, temporary, visitor_root = stage_pyan_sources(files)
    try:
        return _build_function_graph(staged, source_map, root, visitor_root)
    finally:
        if temporary is not None:
            temporary.cleanup()


def _build_function_graph(
    files: list[str],
    source_map: dict[str, str],
    root: Path,
    visitor_root: str | None = None,
) -> tuple[dict[str, object], list[str]]:
    """Build the in-scope function graph from pyan3's resolved use edges.

    Node and edge selection matches the criticality analyzer: defined
    ``FunctionDef``/``AsyncFunctionDef`` nodes in the requested files, and pyan3
    ``uses_edges`` whose caller and callee are both selected functions. SCCs and
    condensation depth come from NetworkX. ``calls.unresolved`` is measured
    separately by visiting the caller's own-scope ``ast.Call`` sites, not from
    pyan3 placeholder or wildcard edges.
    """
    from pyan.analyzer import CallGraphVisitor

    visitor = CallGraphVisitor(files, root=visitor_root)
    skipped = sorted(
        source_map.get(path, path) for path in set(files) - set(visitor.filenames)
    )
    functions = {
        node
        for group in visitor.nodes.values()
        for node in group
        if node.defined
        and node.filename in files
        and isinstance(node.ast_node, (ast.FunctionDef, ast.AsyncFunctionDef))
    }
    ordered = sorted(
        functions,
        key=lambda node: (
            relative_path(Path(source_map.get(node.filename, node.filename)), root),
            node.ast_node.lineno,
            node.get_name(),
        ),
    )
    module_trees: dict[str, ast.Module] = {}
    module_nodes: dict[str, object] = {}
    for group in visitor.nodes.values():
        for node in group:
            if isinstance(node.ast_node, ast.Module):
                module_trees.setdefault(node.filename, node.ast_node)
                module_nodes.setdefault(node.filename, node)
    module_level = {
        identifier
        for tree in module_trees.values()
        for identifier in module_level_function_ids(tree)
    }
    imported_by_module = {
        filename: import_bound_names(tree) for filename, tree in module_trees.items()
    }
    index = {node: position for position, node in enumerate(ordered)}
    calls: set[tuple[int, int]] = set()
    resolved_by_caller: dict[object, list[object]] = {}
    unresolved = 0
    for caller in ordered:
        resolved = [
            callee for callee in visitor.uses_edges.get(caller, ()) if callee in index
        ]
        resolved_by_caller[caller] = resolved
        callee_names: set[str] = set()
        for callee in resolved:
            calls.add((index[caller], index[callee]))
            callee_names.add(callee.name)
        imported_names = imported_by_module.get(caller.filename, set())
        unresolved += sum(
            1
            for call in own_scope_calls(caller.ast_node)
            if not call_is_resolved(call.func, callee_names, imported_names)
        )

    forwarding = {
        function: fact
        for function in ordered
        if (
            fact := forwarding_fact(
                function,
                visitor.scopes.get(
                    module_nodes[function.filename].get_name()
                    if function.filename in module_nodes
                    else ""
                ),
                visitor.scopes.get(function.get_name()),
                module_level,
                set(index),
            )
        )
        is not None
    }

    import networkx as nx

    dag = nx.DiGraph()
    dag.add_nodes_from(range(len(ordered)))
    dag.add_edges_from(sorted(calls))
    components = [set(component) for component in nx.strongly_connected_components(dag)]
    condensation = nx.condensation(dag, scc=components)
    depths = {node: 0 for node in condensation.nodes}
    for component in reversed(list(nx.topological_sort(condensation))):
        depths[component] = max(
            (1 + depths[target] for target in condensation.successors(component)),
            default=0,
        )
    mapping = condensation.graph["mapping"]
    ordered_components = sorted(
        (sorted(component) for component in components), key=lambda part: tuple(part)
    )
    recursive_components = sum(
        1
        for component in ordered_components
        if len(component) > 1 or (component[0], component[0]) in calls
    )
    return (
        {
            "functions": [
                {
                    "file": relative_path(
                        Path(source_map.get(node.filename, node.filename)), root
                    ),
                    "line": node.ast_node.lineno,
                    "symbol": node.get_name(),
                    "depth": depths[mapping[position]],
                    "module_level": id(node.ast_node) in module_level,
                    "forwarding_mode": forwarding.get(node, ("", ""))[0],
                    "forward_target": forwarding.get(node, ("", ""))[1],
                }
                for position, node in enumerate(ordered)
            ],
            "calls": [
                {"caller": caller, "callee": callee}
                for caller, callee in sorted(calls)
            ],
            "function_components": [
                {
                    "functions": component,
                    "cyclic": len(component) > 1
                    or (component[0], component[0]) in calls,
                }
                for component in ordered_components
            ],
            "function_counts": {
                "functions": len(ordered),
                "calls": len(calls),
                "recursive_components": recursive_components,
                "unresolved_calls": unresolved,
            },
        },
        skipped,
    )


def build_report(targets: list[str]) -> dict[str, object]:
    root = Path.cwd().resolve()
    target_paths = [Path(target) for target in targets or ["."]]
    for path in target_paths:
        if not path.exists():
            raise StructureError(
                "invalid_configuration", f"Source path not found: {path}"
            )
    import_roots = project_import_roots(root)
    (
        identity_by_path,
        inventory,
        namespace_packages,
        collisions,
    ) = build_inventory(import_roots)

    patterns = exclusion_patterns()
    exclude_tests = os.environ.get("PYGUARD_EXCLUDE_TESTS") == "1"
    included_paths = {
        path
        for path in collect_candidates(targets)
        if not excluded(path, root, patterns, exclude_tests)
    }

    parsed: dict[Path, ast.Module] = {}
    unparsable: list[Path] = []
    for path in sorted(included_paths):
        try:
            parsed[path] = read_tree(path)
        except StructureError:
            unparsable.append(path)

    handler_facts: list[dict[str, object]] = []
    for path in sorted(parsed):
        handler_facts.extend(failure_handlers(path, parsed[path]))

    retained: dict[str, tuple[Path, Path, bool]] = {}
    retained_by_path: dict[Path, tuple[str, Path, bool]] = {}
    for path in sorted(parsed):
        identity = identity_by_path.get(path)
        if identity is None:
            # A candidate outside every configured import root is invalid
            # configuration rather than a reason to guess an identity.
            unparsable.append(path)
            continue
        name, import_root, is_package = identity
        canonical = inventory.get(name)
        if canonical is not None and canonical[0] is not None and canonical[0] != path:
            # The global inventory keeps the first file for a duplicate
            # identity. Never scan a different file under the same name.
            collisions.append((name, canonical[0], path))
            continue
        if name in retained:
            collisions.append((name, retained[name][0], path))
            continue
        retained[name] = (path, import_root, is_package)
        retained_by_path[path] = (name, import_root, is_package)

    package_exports: dict[str, set[str]] = {}
    dynamic_exports: dict[str, bool] = {}
    # Export validation uses the full inventory, not only retained graph nodes:
    # ``from X import name`` must be checked before an out-of-scope X or child
    # becomes a silent boundary.
    for name, (path, _, is_package) in inventory.items():
        if not is_package or path is None:
            continue
        tree = parsed.get(path)
        if tree is None:
            try:
                tree = read_tree(path)
            except StructureError:
                # An unreadable out-of-scope package cannot prove a negative.
                # Treat its attributes as dynamic rather than reporting a
                # source-independent missing name.
                dynamic_exports[name] = True
                continue
        exports, dynamic = module_exports(tree)
        package_exports[name] = exports
        dynamic_exports[name] = dynamic

    inventory_names = set(inventory)
    included_names = set(retained)
    package_flags = {name: info[2] for name, info in inventory.items()}

    type_imports = 0
    deferred_imports = 0
    deferred_by_module: dict[str, set[int]] = {}
    unresolved: list[tuple[str, int, str]] = []
    missing_by_module: dict[str, dict[int, set[str]]] = {}
    valid_by_module: dict[str, dict[int, set[str]]] = {}
    for path in sorted(retained_by_path):
        tree = parsed[path]
        name, _, is_package = retained_by_path[path]
        # Package detection is looked up from inventory so alias validation
        # also treats an excluded package base as a scope boundary.
        is_package = package_flags.get(name, False)
        (
            typed,
            deferred,
            lines,
            missing,
            missing_lines,
            valid_lines,
        ) = scan_ast(
            tree,
            name,
            is_package,
            inventory_names,
            included_names,
            package_flags,
            namespace_packages,
            package_exports,
            dynamic_exports,
        )
        type_imports += typed
        deferred_imports += deferred
        deferred_by_module[name] = lines
        missing_by_module[name] = missing_lines
        valid_by_module[name] = valid_lines
        unresolved.extend(
            (relative_path(path, root), line, target) for line, target in missing
        )

    module_files = {name: info[0] for name, info in retained.items()}
    edges: list[tuple[str, str]] = []
    components: list[list[str]] = []
    depths: dict[str, int] = {}
    cycles = 0
    resolver_reason = ""
    if module_files:
        edges, components, depths, cycles, resolver_reason = build_graph(
            module_files,
            inventory,
            deferred_by_module,
            missing_by_module,
            valid_by_module,
        )

    # Files that the structure parser could not read are already incomplete
    # evidence. pyan3 only sees parseable sources, so its own skipped set adds
    # evidence only for a source it drops after Python accepted it.
    function_graph, function_skipped = build_function_graph(
        sorted(str(path) for path in parsed), root
    )

    reasons: list[str] = []
    if unparsable:
        reasons.append(
            "Unparsable source: "
            + ", ".join(relative_path(path, root) for path in sorted(set(unparsable)))
        )
    # pyan3 drops a file it cannot parse from its filename set instead of
    # failing. Structure already treats those files as unparsable, so only a
    # skip that the module parser did not see adds another limitation.
    unparsable_paths = {str(path.resolve()) for path in unparsable}
    extra_skipped = [
        path for path in function_skipped if str(Path(path).resolve()) not in unparsable_paths
    ]
    if extra_skipped:
        reasons.append(
            "Unparsable source skipped by pyan3: "
            + ", ".join(
                relative_path(Path(path), root) for path in sorted(set(extra_skipped))
            )
        )
    imported_targets = {
        target
        for targets_by_line in valid_by_module.values()
        for targets in targets_by_line.values()
        for target in targets
    }
    relevant_collisions = [
        collision
        for collision in collisions
        if collision[0] in module_files or collision[0] in imported_targets
    ]
    if relevant_collisions:
        unique_collisions = sorted(
            {
                (name, kept, skipped)
                for name, kept, skipped in relevant_collisions
            }
        )
        reasons.append(
            "Duplicate module identity"
            + ("ies: " if len(unique_collisions) > 1 else ": ")
            + ", ".join(
                f"{name} ({relative_path(kept, root)}, {relative_path(skipped, root)})"
                for name, kept, skipped in unique_collisions
            )
        )
    if unresolved:
        reasons.append(
            "Unresolved local import"
            + ("s: " if len(unresolved) > 1 else ": ")
            + ", ".join(
                f"{target or '<relative>'} ({path}:{line})"
                for path, line, target in sorted(unresolved)
            )
        )
    if resolver_reason:
        reasons.append(resolver_reason)
    if not module_files:
        reasons.append("No importable Python modules found in the requested scope.")

    return {
        "schema_version": "1",
        "modules": [
            {
                "name": name,
                "file": relative_path(module_files[name], root),
                "depth": depths.get(name, 0),
            }
            for name in sorted(module_files)
        ],
        "imports": [
            {"importer": importer, "imported": imported}
            for importer, imported in edges
        ],
        "components": [
            {
                "modules": component,
                "cyclic": len(component) > 1
                or (component[0], component[0]) in edges,
            }
            for component in components
        ],
        "counts": {
            "modules": len(module_files),
            "edges": len(edges),
            "cycles": cycles,
            "type_imports": type_imports,
            "deferred_imports": deferred_imports,
        },
        **function_graph,
        "failure_handlers": handler_facts,
        "partial": bool(reasons),
        "limitation": " ".join(reasons),
    }


def machine_failure(category: str, message: str) -> int:
    print(
        json.dumps(
            {
                "schema_version": "1",
                "modules": [],
                "imports": [],
                "components": [],
                "counts": {},
                **empty_function_graph(),
                "partial": False,
                "limitation": "",
                "error": {"category": category, "message": message},
            },
            sort_keys=True,
        )
    )
    return 1


def main() -> int:
    machine = "--json" in sys.argv[1:]
    targets = [arg for arg in sys.argv[1:] if arg != "--json"] or ["."]
    try:
        try:
            from grimp.application.graph import ImportGraph  # noqa: F401
            from grimp.application.scanning import scan_imports  # noqa: F401
            from grimp.domain.valueobjects import Module  # noqa: F401
        except ImportError as error:
            raise StructureError(
                "tool_missing",
                f"grimp is not installed. Run: uv add --group dev grimp==3.18 ({error})",
            ) from error
        try:
            import networkx  # noqa: F401
        except ImportError as error:
            raise StructureError(
                "tool_missing",
                f"networkx is not installed. Run: uv add --group dev networkx ({error})",
            ) from error
        try:
            from pyan.analyzer import CallGraphVisitor  # noqa: F401
        except ImportError as error:
            raise StructureError(
                "tool_missing",
                f"pyan3 is not installed. Run: uv add --group dev pyan3 ({error})",
            ) from error
        report = build_report(targets)
    except StructureError as error:
        if machine:
            return machine_failure(error.category, error.message)
        print(f"WARNING: {error.message}", file=sys.stderr)
        return 1
    except (
        AttributeError,
        IndexError,
        KeyError,
        NotImplementedError,
        OSError,
        RecursionError,
        RuntimeError,
        TypeError,
        ValueError,
    ) as error:
        if machine:
            return machine_failure(
                "adapter_failure", f"Structure analysis failed: {error}"
            )
        print(f"WARNING: Structure analysis failed: {error}", file=sys.stderr)
        return 1

    if machine:
        print(json.dumps(report, sort_keys=True))
        return 0

    counts = report["counts"]
    assert isinstance(counts, dict)
    print(
        "  structure: "
        f"{counts['modules']} modules, {counts['edges']} runtime edges, "
        f"{counts['cycles']} cycles, {counts['type_imports']} type-only imports, "
        f"{counts['deferred_imports']} deferred imports"
    )
    for component in report["components"]:
        assert isinstance(component, dict)
        if component["cyclic"]:
            print("  [ADVISORY] circular dependency: " + ", ".join(component["modules"]))
    if report["partial"]:
        print(f"  [ADVISORY] structure — {report['limitation']}")
    return 0


if __name__ == "__main__":
    sys.exit(main())
