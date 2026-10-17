"""Copy documentation into a release payload using its tag, without editing sources."""
from pathlib import Path
import re
import shutil
import sys


def stamp_document(text, tag):
    label = f"\nOxGuard version: {tag}\n"
    for kind in ("blob", "tree"):
        text = text.replace(f"https://github.com/oxalc88/oxguard/{kind}/main/",
                            f"https://github.com/oxalc88/oxguard/{kind}/{tag}/")
    if text.startswith("---\n"):
        end = text.find("\n---\n", 4)
        if end >= 0:
            offset = end + len("\n---\n")
            return text[:offset] + label + text[offset:]
    return label + "\n" + text


def package_docs(tag, destination, source=None):
    if not re.fullmatch(r"v(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)(-[0-9A-Za-z.-]+)?", tag):
        raise ValueError("Invalid release tag")
    source = source or Path(__file__).resolve().parents[1]
    destination.mkdir(parents=True, exist_ok=True)
    shutil.copytree(source / "docs", destination / "docs", dirs_exist_ok=True)
    shutil.copyfile(source / "README.md", destination / "README.md")
    files = [destination / "README.md", *(destination / "docs").rglob("*.md")]
    for file in files:
        file.write_text(stamp_document(file.read_text(encoding="utf-8"), tag), encoding="utf-8")
    (destination / "VERSION").write_text(tag + "\n", encoding="utf-8")


if __name__ == "__main__":
    if len(sys.argv) != 3:
        raise SystemExit("Usage: package-release-docs.py <tag> <output-directory>")
    package_docs(sys.argv[1], Path(sys.argv[2]))
