#!/usr/bin/env sh
# oxguard install script — installs pyguard or tsguard from GitHub Releases.
# Usage: curl -fsSL https://github.com/oxalc88/oxguard/releases/latest/download/install.sh | sh -s -- pyguard
#        curl -fsSL https://github.com/oxalc88/oxguard/releases/latest/download/install.sh | sh -s -- tsguard
set -eu

REPO="oxalc88/oxguard"
INSTALL_DIR="${OXGUARD_INSTALL_DIR:-$HOME/.local/bin}"

# ── helpers ─────────────────────────────────────────────────────────────────

die() { printf '\033[31merror:\033[0m %s\n' "$*" >&2; exit 1; }
info() { printf '  %s\n' "$*"; }
ok()   { printf '  \033[32m[OK]\033[0m   %s\n' "$*"; }

need_cmd() { command -v "$1" >/dev/null 2>&1 || die "required command not found: $1"; }

# ── argument parsing ─────────────────────────────────────────────────────────

TOOL="${1:-}"
case "$TOOL" in
  pyguard|tsguard) ;;
  "")
    printf 'Usage: install.sh [pyguard|tsguard]\n\n'
    printf '  pyguard  — Python quality gate (ruff, mypy, radon, bandit, ...)\n'
    printf '  tsguard  — TypeScript quality gate (biome, vitest, fta-cli, ...)\n'
    exit 1
    ;;
  *) die "unknown tool: $TOOL (choose pyguard or tsguard)" ;;
esac

# ── platform detection ───────────────────────────────────────────────────────

OS="$(uname -s | tr '[:upper:]' '[:lower:]')"
ARCH="$(uname -m)"

case "$OS" in
  linux)  ;;
  darwin) ;;
  *)      die "unsupported OS: $OS" ;;
esac

case "$ARCH" in
  x86_64|amd64)   ARCH=amd64 ;;
  aarch64|arm64)  ARCH=arm64 ;;
  *)              die "unsupported architecture: $ARCH" ;;
esac

# ── version resolution ───────────────────────────────────────────────────────

VERSION="${OXGUARD_VERSION:-}"
if [ -z "$VERSION" ]; then
  need_cmd curl
  API_URL="https://api.github.com/repos/${REPO}/releases/latest"
  VERSION="$(curl -fsSL "$API_URL" | grep '"tag_name"' | sed 's/.*"tag_name": *"\([^"]*\)".*/\1/')"
  [ -n "$VERSION" ] || die "could not resolve latest version from GitHub API"
fi

info "installing $TOOL $VERSION for $OS/$ARCH"

# ── download ─────────────────────────────────────────────────────────────────

TARBALL="${TOOL}-${VERSION}-${OS}-${ARCH}.tar.gz"
BASE_URL="https://github.com/${REPO}/releases/download/${VERSION}"
TMPDIR="$(mktemp -d)"
trap 'rm -rf "$TMPDIR"' EXIT

need_cmd curl
curl -fsSL --progress-bar -o "$TMPDIR/$TARBALL" "$BASE_URL/$TARBALL"
curl -fsSL -o "$TMPDIR/$TARBALL.sha256" "$BASE_URL/$TARBALL.sha256"

# ── verify ───────────────────────────────────────────────────────────────────

info "verifying checksum..."
cd "$TMPDIR"
if command -v sha256sum >/dev/null 2>&1; then
  sha256sum -c "$TARBALL.sha256" >/dev/null || die "checksum mismatch"
elif command -v shasum >/dev/null 2>&1; then
  shasum -a 256 -c "$TARBALL.sha256" >/dev/null || die "checksum mismatch"
else
  info "[WARN] no sha256sum or shasum found — skipping checksum verification"
fi
cd - >/dev/null

# ── extract ──────────────────────────────────────────────────────────────────

tar -xzf "$TMPDIR/$TARBALL" -C "$TMPDIR"
EXTRACTED="$TMPDIR/${TARBALL%.tar.gz}"

# ── install ──────────────────────────────────────────────────────────────────

mkdir -p "$INSTALL_DIR"

install -m 755 "$EXTRACTED/$TOOL" "$INSTALL_DIR/$TOOL"

ok "$TOOL installed to $INSTALL_DIR/$TOOL"

# ── verify binary runs ───────────────────────────────────────────────────────

if INSTALLED_VER="$("$INSTALL_DIR/$TOOL" --version 2>&1)"; then
  ok "$TOOL $INSTALLED_VER is working"
else
  die "$TOOL installed but --version failed — check your system libraries"
fi

# ── PATH guidance ────────────────────────────────────────────────────────────

case ":$PATH:" in
  *":$INSTALL_DIR:"*) ;;
  *)
    printf '\n  \033[33m[!]\033[0m %s is not on your PATH.\n' "$INSTALL_DIR"
    case "${SHELL:-}" in
      */fish)
        printf '      Run: fish_add_path %s\n' "$INSTALL_DIR"
        ;;
      */zsh)
        printf '      Add to ~/.zshrc:\n'
        printf '        export PATH="%s:$PATH"\n' "$INSTALL_DIR"
        ;;
      *)
        printf '      Add to ~/.bashrc (or equivalent):\n'
        printf '        export PATH="%s:$PATH"\n' "$INSTALL_DIR"
        ;;
    esac
    printf '\n'
    ;;
esac

printf '\n'
printf '  Next steps:\n'
printf '    cd your-project\n'
printf '    %s setup    # wire dev dependencies + AI tool hooks\n' "$TOOL"
printf '    %s doctor   # verify environment\n' "$TOOL"
printf '\n'
