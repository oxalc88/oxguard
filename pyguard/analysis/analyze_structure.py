"""Build a PyGuard runtime module graph from importable project packages."""

from __future__ import annotations

import ast
import fnmatch
import io
import json
import os
import sys
import tempfile
import tokenize
from pathlib import Path

# Python packages grimp cannot infer for top-level modules are represented as
# single-module FoundPackages. Package files are grouped by their actual import
# root so relative imports resolve exactly as Python does.
TEST_FILE_PATTERNS = ("test_*.py", "*_test.py", "conftest.py")
TEST_DIRECTORY_NAMES = {"test", "tests"}
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


def called_identifier_names(node: ast.AST) -> set[str]:
    """Return identifiers that participate in a call expression in *node*.

    pyan3's use edges include ordinary name reads and a placeholder for every
    function parameter. Only an unresolved callee whose name is actually used
    in a call expression is a call reference for the unresolved-call count.
    For ``helpers.run()`` both ``helpers`` and ``run`` participate in the call.
    """
    names: set[str] = set()
    for child in ast.walk(node):
        if not isinstance(child, ast.Call):
            continue
        pending = [child.func]
        while pending:
            current = pending.pop()
            if isinstance(current, ast.Name):
                names.add(current.id)
            elif isinstance(current, ast.Attribute):
                names.add(current.attr)
                pending.append(current.value)
            elif isinstance(current, ast.Call):
                pending.append(current.func)
    return names


def parameter_names(node: ast.FunctionDef | ast.AsyncFunctionDef) -> set[str]:
    """Return every bound parameter name, including positional-only and variadic."""
    arguments = node.args
    names = {
        argument.arg
        for argument in (
            list(arguments.posonlyargs)
            + list(arguments.args)
            + list(arguments.kwonlyargs)
        )
    }
    if arguments.vararg is not None:
        names.add(arguments.vararg.arg)
    if arguments.kwarg is not None:
        names.add(arguments.kwarg.arg)
    return names


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


def stage_pyan_sources(
    files: list[str], root: Path
) -> tuple[list[str], dict[str, str], tempfile.TemporaryDirectory[str] | None]:
    """Mirror sources as UTF-8 when pyan3 cannot read their valid encoding.

    pyan3 opens every file as UTF-8, so a UTF-8 BOM or a valid PEP 263 source
    declaration makes it drop the file (or fail). The structure parser already
    accepted those files. A temporary mirror keeps line numbers and relative
    module layout, and the original paths remain the reported locations.
    """
    needs_copy = False
    for path in files:
        data = Path(path).read_bytes()
        if data.startswith(b"\xef\xbb\xbf"):
            needs_copy = True
            break
        try:
            data.decode("utf-8")
        except UnicodeDecodeError:
            needs_copy = True
            break
    if not needs_copy:
        return files, {}, None

    temporary = tempfile.TemporaryDirectory(prefix="pyguard-structure-functions-")
    staged_root = Path(temporary.name)
    # A root marker pins pyan3's inferred import root to the mirrored tree even
    # when the original project root marker is outside the copied common path.
    (staged_root / "pyproject.toml").write_bytes(b"")
    base = root
    if any(not Path(path).is_relative_to(base) for path in files):
        base = Path(os.path.commonpath(files))
        if not base.is_dir():
            base = base.parent
    staged: list[str] = []
    mapping: dict[str, str] = {}
    for path in files:
        source = Path(path).read_bytes()
        encoding, _ = tokenize.detect_encoding(io.BytesIO(source).readline)
        text = source.decode(encoding)
        try:
            relative = Path(path).relative_to(base)
        except ValueError:
            relative = Path(path).name
        target = staged_root / relative
        target.parent.mkdir(parents=True, exist_ok=True)
        target.write_bytes(text.encode("utf-8"))
        staged.append(str(target))
        mapping[str(target)] = path
    return staged, mapping, temporary


def build_function_graph(
    files: list[str], root: Path
) -> tuple[dict[str, object], list[str]]:
    """Build the function graph, normalizing only sources pyan3 cannot decode."""
    if not files:
        return empty_function_graph(), []
    staged, source_map, temporary = stage_pyan_sources(files, root)
    try:
        return _build_function_graph(staged, source_map, root)
    finally:
        if temporary is not None:
            temporary.cleanup()


def _build_function_graph(
    files: list[str], source_map: dict[str, str], root: Path
) -> tuple[dict[str, object], list[str]]:
    """Build the in-scope function graph from pyan3's resolved use edges.

    Node and edge selection matches the criticality analyzer: defined
    ``FunctionDef``/``AsyncFunctionDef`` nodes in the requested files, and pyan3
    ``uses_edges`` whose caller and callee are both selected functions. SCCs and
    condensation depth come from NetworkX. A parameter placeholder is only
    counted as unresolved when the function actually calls a parameter; plain
    parameter reads are not call references.
    """
    from pyan.analyzer import CallGraphVisitor

    visitor = CallGraphVisitor(files)
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
    index = {node: position for position, node in enumerate(ordered)}
    calls: set[tuple[int, int]] = set()
    unresolved = 0
    for caller, callees in visitor.uses_edges.items():
        if caller not in index:
            continue
        called = called_identifier_names(caller.ast_node)
        parameters = parameter_names(caller.ast_node)
        for callee in callees:
            if callee in index:
                calls.add((index[caller], index[callee]))
                continue
            if "^^^argument^^^" in (callee.name or ""):
                # pyan3 uses one placeholder for every parameter use, so it can
                # only be a call reference when the function calls a parameter.
                if called & parameters:
                    unresolved += 1
                continue
            if callee.name in called:
                unresolved += 1

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
