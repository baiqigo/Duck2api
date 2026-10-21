#!/usr/bin/env sh
set -eu

REPO="${DUCK2API_REPO:-baiqigo/Duck2api}"
VERSION="${1:-latest}"
PREFIX="${PREFIX:-$HOME/.local/bin}"

os="$(uname -s | tr '[:upper:]' '[:lower:]')"
case "$os" in
  linux|darwin) ;;
  *) echo "unsupported OS: $os (use install.ps1 on Windows)" >&2; exit 1 ;;
esac

arch="$(uname -m)"
case "$arch" in
  x86_64|amd64) arch=amd64 ;;
  aarch64|arm64) arch=arm64 ;;
  *) echo "unsupported architecture: $arch" >&2; exit 1 ;;
esac

asset="duck2api-${os}-${arch}.tar.gz"
if [ "$VERSION" = "latest" ]; then
  base="https://github.com/${REPO}/releases/latest/download"
else
  base="https://github.com/${REPO}/releases/download/${VERSION}"
fi

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

download() {
  if command -v curl >/dev/null 2>&1; then
    curl -fL --retry 3 -o "$2" "$1"
  elif command -v wget >/dev/null 2>&1; then
    wget -O "$2" "$1"
  else
    echo "curl or wget is required" >&2
    exit 1
  fi
}

echo "Downloading ${base}/${asset}"
download "${base}/${asset}" "$tmp/$asset"

if command -v sha256sum >/dev/null 2>&1; then
  download "${base}/checksums.txt" "$tmp/checksums.txt"
  (cd "$tmp" && grep "  ${asset}$" checksums.txt | sha256sum -c -)
elif command -v shasum >/dev/null 2>&1; then
  download "${base}/checksums.txt" "$tmp/checksums.txt"
  (cd "$tmp" && grep "  ${asset}$" checksums.txt | shasum -a 256 -c -)
fi

tar -xzf "$tmp/$asset" -C "$tmp"
mkdir -p "$PREFIX"
install -m 0755 "$tmp/duck2api" "$PREFIX/duck2api"
echo "Installed duck2api to $PREFIX/duck2api"

case ":$PATH:" in
  *":$PREFIX:"*) ;;
  *) echo "Add $PREFIX to PATH, then run: duck2api" ;;
esac