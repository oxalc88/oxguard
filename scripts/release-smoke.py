"""Check the exact release archives and npm payloads before publication."""
import hashlib
import json
from pathlib import Path
import platform
import subprocess
import sys
import tarfile
import tempfile
import zipfile


def run(args):
    return subprocess.check_output(args, text=True, timeout=30).strip()


def unpack(archive, destination):
    if archive.suffix == ".zip":
        with zipfile.ZipFile(archive) as package:
            # Release zip payloads only contain regular files/directories.
            for name in package.namelist():
                if not (destination / name).resolve().is_relative_to(destination.resolve()):
                    raise ValueError("Archive path escapes destination")
            package.extractall(destination)
    else:
        with tarfile.open(archive) as package:
            package.extractall(destination, filter="data")


def check_documentation(directory, tag, guide=None):
    if (directory / "VERSION").read_text(encoding="utf-8").strip() != tag:
        raise ValueError("Release documentation version does not match tag")
    if guide and not (directory / "docs" / guide).is_file():
        raise ValueError("Release documentation guide is missing")
    files = [directory / "README.md", *(directory / "docs").rglob("*.md")]
    for file in files:
        text = file.read_text(encoding="utf-8")
        if f"OxGuard version: {tag}" not in text.splitlines():
            raise ValueError(f"Documentation version missing or mismatched: {file.name}")
        if any(f"https://github.com/oxalc88/oxguard/{kind}/main/" in text for kind in ("blob", "tree")):
            raise ValueError(f"Release documentation links to main: {file.name}")


def archive_smoke(tool, tag, target_os, arch, directory):
    name = f"{tool}-{tag}-{target_os}-{arch}"
    extension = "zip" if target_os == "windows" else "tar.gz"
    archive = directory / f"{name}.{extension}"
    expected, filename = Path(str(archive) + ".sha256").read_text().split()
    if filename != archive.name or hashlib.sha256(archive.read_bytes()).hexdigest() != expected:
        raise ValueError("Release archive checksum mismatch")
    with tempfile.TemporaryDirectory(prefix="oxguard-release-") as temporary:
        destination = Path(temporary)
        unpack(archive, destination)
        check_documentation(destination / name, tag, f"guides/{tool}-result-contract.md")
        binary = destination / name / (tool + (".exe" if target_os == "windows" else ""))
        metadata = run(["go", "version", "-m", str(binary)])
        settings = dict(line.strip().split(None, 1)[1].split("=", 1)
                        for line in metadata.splitlines() if line.strip().startswith("build\t"))
        if settings.get("GOOS") != target_os or settings.get("GOARCH") != arch:
            raise ValueError("Release binary target does not match archive")
        if tool == "pyguard":
            owned = {p.name for p in (Path(__file__).resolve().parents[1] / "pyguard/analysis").glob("*.py")}
            if not owned or not all((destination / name / "analysis" / file).is_file() for file in owned):
                raise ValueError("Release archive is missing Python analysis helpers")
        host_arch = {"x86_64": "amd64", "aarch64": "arm64", "arm64": "arm64"}.get(platform.machine().lower())
        if target_os == platform.system().lower() and arch == host_arch:
            if run([str(binary), "--version"]) != tag:
                raise ValueError("Release binary version does not match tag")
            if not run([str(binary), "--help"]):
                raise ValueError("Release binary help is empty")
    print(f"Verified release archive: {archive.name}")


def npm_smoke(tag, directory):
    # Extract exact tarballs into Node's installed layout; do not repeat registry installs.
    version = tag.removeprefix("v")
    with tempfile.TemporaryDirectory(prefix="oxguard-npm-release-") as temporary:
        destination = Path(temporary)
        scope = destination / "node_modules/@oxguard"
        for name in ["tsguard", "tsguard-linux-x64"]:
            tarball = directory / f"oxguard-{name}-{version}.tgz"
            staging = destination / name
            unpack(tarball, staging)
            package = staging / "package"
            manifest = json.loads((package / "package.json").read_text())
            if manifest["name"] != f"@oxguard/{name}" or manifest["version"] != version or manifest.get("scripts"):
                raise ValueError("Release npm manifest does not match tag/name or contains scripts")
            check_documentation(package, tag, "guides/tsguard-result-contract.md" if name == "tsguard" else None)
            scope.mkdir(parents=True, exist_ok=True)
            package.rename(scope / name)
        launcher = scope / "tsguard/bin/tsguard.cjs"
        if run(["node", str(launcher), "--version"]) != tag:
            raise ValueError("Packaged launcher/native version mismatch")
        if not run(["node", str(launcher), "--help"]):
            raise ValueError("Packaged launcher help is empty")
    print("Verified exact npm tarballs: launcher and Linux x64 native payload")


if __name__ == "__main__":
    if len(sys.argv) == 7 and sys.argv[1] == "archive":
        archive_smoke(*sys.argv[2:6], Path(sys.argv[6]))
    elif len(sys.argv) == 4 and sys.argv[1] == "npm":
        npm_smoke(sys.argv[2], Path(sys.argv[3]))
    else:
        raise SystemExit("Usage: release-smoke.py archive <tool> <tag> <os> <arch> <dist> | npm <tag> <npm-dir>")
