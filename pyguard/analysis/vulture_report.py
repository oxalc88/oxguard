"""Expose Vulture's native items without parsing its human report."""
import contextlib
import json
import sys


def main() -> int:
    data = {"schema_version": "1", "findings": [], "measurements": [], "artifacts": []}
    try:
        from vulture import Vulture
        from vulture.config import make_config
        # Keep configuration precedence, exclusions and confidence identical to
        # the CLI. Analyzer verbosity is diagnostic output, never JSON stdout.
        with contextlib.redirect_stdout(sys.stderr):
            config = make_config(sys.argv[1:])
            analyzer = Vulture(verbose=config["verbose"], ignore_names=config["ignore_names"], ignore_decorators=config["ignore_decorators"])
            analyzer.scavenge(config["paths"], exclude=config["exclude"])
            items = analyzer.get_unused_code(min_confidence=config["min_confidence"], sort_by_size=config["sort_by_size"])
        if analyzer.exit_code:
            data["error"] = {"category": "analyzer_failure", "message": "Vulture could not evaluate all configured paths."}
        else:
            for item in items:
                data["findings"].append({"rule": "vulture.unused." + item.typ, "severity": "warning", "evidence": item.message, "observed": item.confidence, "location": {"file": str(item.filename), "line": item.first_lineno, "symbol": item.name}})
    except Exception as error:
        data["error"] = {"category": "tool_missing" if isinstance(error, ImportError) else "invalid_configuration", "message": "Vulture could not load its analyzer or configuration."}
    print(json.dumps(data, sort_keys=True))
    return 1 if data.get("error") or data["findings"] else 0


if __name__ == "__main__":
    sys.exit(main())
