#!/usr/bin/env bash
# llmaker installer.
#
#   curl -fsSL https://raw.githubusercontent.com/raiyanyahya/llmaker/master/scripts/install.sh | sh
#
# Downloads the latest release binary for your OS/arch into a bin directory on
# your PATH. Falls back to `go install` when no prebuilt asset is available.
# Note: POSIX sh (dash on Debian/Ubuntu) is the interpreter for `curl … | sh`,
# and `pipefail` is unsupported there before dash 0.5.12 — so keep to `-eu`.
set -eu

REPO="${LLMAKER_REPO:-raiyanyahya/llmaker}"
BIN="llmaker"
INSTALL_DIR="${LLMAKER_INSTALL_DIR:-}"

info() { printf '\033[36m==>\033[0m %s\n' "$1"; }
err()  { printf '\033[31merror:\033[0m %s\n' "$1" >&2; }

detect_platform() {
  local os arch
  os="$(uname -s | tr '[:upper:]' '[:lower:]')"
  arch="$(uname -m)"
  case "$arch" in
    x86_64|amd64) arch="amd64" ;;
    arm64|aarch64) arch="arm64" ;;
    *) err "unsupported architecture: $arch"; exit 1 ;;
  esac
  case "$os" in
    linux|darwin) ;;
    *) err "unsupported OS: $os (try: go install github.com/$REPO/cmd/llmaker@latest)"; exit 1 ;;
  esac
  echo "${os}_${arch}"
}

choose_dir() {
  if [ -n "$INSTALL_DIR" ]; then echo "$INSTALL_DIR"; return; fi
  if [ -w "/usr/local/bin" ]; then echo "/usr/local/bin"; return; fi
  echo "$HOME/.local/bin"
}

main() {
  local platform tag url tmp dir
  platform="$(detect_platform)"

  tag="$(curl -fsSL "https://api.github.com/repos/$REPO/releases/latest" 2>/dev/null \
        | grep -m1 '"tag_name"' | cut -d '"' -f4 || true)"

  if [ -z "$tag" ]; then
    info "No release found; installing from source via 'go install'."
    if ! command -v go >/dev/null 2>&1; then
      err "Go is not installed and no release binary is available."
      exit 1
    fi
    go install "github.com/$REPO/cmd/llmaker@latest"
    info "Installed via go install (ensure \$GOBIN or \$GOPATH/bin is on PATH)."
    return
  fi

  url="https://github.com/$REPO/releases/download/${tag}/${BIN}_${tag#v}_${platform}.tar.gz"
  dir="$(choose_dir)"
  mkdir -p "$dir"
  tmp="$(mktemp -d)"
  trap 'rm -rf "$tmp"' EXIT

  info "Downloading $BIN $tag ($platform)…"
  curl -fsSL "$url" -o "$tmp/$BIN.tar.gz"
  tar -xzf "$tmp/$BIN.tar.gz" -C "$tmp"
  install -m 0755 "$tmp/$BIN" "$dir/$BIN"

  info "Installed to $dir/$BIN"
  case ":$PATH:" in
    *":$dir:"*) ;;
    *) info "Add $dir to your PATH to use '$BIN'." ;;
  esac
  "$dir/$BIN" version || true
}

main "$@"
