#!/bin/sh
# Install tabb from GitHub releases.
# Usage: curl -fsSL https://raw.githubusercontent.com/joelhelbling/tabb/main/install.sh | sh
# Env:
#   TABB_VERSION  pin a version (e.g. v0.1.0); default: latest
#   INSTALL_DIR   install location; default: ~/.local/bin
set -eu

REPO="joelhelbling/tabb"
INSTALL_DIR="${INSTALL_DIR:-$HOME/.local/bin}"

# --- detect OS ---
os="$(uname -s)"
case "$os" in
  Linux)  os="linux" ;;
  Darwin) os="darwin" ;;
  *) echo "tabb: unsupported OS: $os" >&2; exit 1 ;;
esac

# --- detect arch ---
arch="$(uname -m)"
case "$arch" in
  x86_64|amd64) arch="amd64" ;;
  arm64|aarch64) arch="arm64" ;;
  *) echo "tabb: unsupported architecture: $arch" >&2; exit 1 ;;
esac

# --- resolve version ---
version="${TABB_VERSION:-}"
if [ -z "$version" ]; then
  version="$(curl -fsSL "https://api.github.com/repos/$REPO/releases/latest" \
    | grep '"tag_name":' | head -n1 | sed -E 's/.*"tag_name": *"([^"]+)".*/\1/')"
fi
if [ -z "$version" ]; then
  echo "tabb: could not resolve release version" >&2
  exit 1
fi

# goreleaser strips the leading 'v' from the version in archive names.
ver_no_v="${version#v}"
archive="tabb_${ver_no_v}_${os}_${arch}.tar.gz"
base="https://github.com/$REPO/releases/download/$version"

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

echo "tabb: downloading $archive ($version)"
curl -fsSL "$base/$archive" -o "$tmp/$archive"
curl -fsSL "$base/checksums.txt" -o "$tmp/checksums.txt"

# --- verify checksum ---
echo "tabb: verifying checksum"
expected="$(grep " $archive\$" "$tmp/checksums.txt" | awk '{print $1}')"
if [ -z "$expected" ]; then
  echo "tabb: no checksum found for $archive" >&2
  exit 1
fi
if command -v sha256sum >/dev/null 2>&1; then
  actual="$(sha256sum "$tmp/$archive" | awk '{print $1}')"
else
  actual="$(shasum -a 256 "$tmp/$archive" | awk '{print $1}')"
fi
if [ "$expected" != "$actual" ]; then
  echo "tabb: checksum mismatch (expected $expected, got $actual)" >&2
  exit 1
fi

# --- extract and install ---
tar -xzf "$tmp/$archive" -C "$tmp"
mkdir -p "$INSTALL_DIR"
install -m 0755 "$tmp/tabb" "$INSTALL_DIR/tabb"

echo "tabb: installed $version to $INSTALL_DIR/tabb"
case ":$PATH:" in
  *":$INSTALL_DIR:"*) ;;
  *) echo "tabb: note — $INSTALL_DIR is not on your PATH" >&2 ;;
esac
