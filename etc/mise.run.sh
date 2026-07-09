#!/bin/sh
set -eu

#region logging setup
if [ "${MISE_DEBUG-}" = "true" ] || [ "${MISE_DEBUG-}" = "1" ]; then
  debug() {
    echo "$@" >&2
  }
else
  debug() {
    :
  }
fi

if [ "${MISE_QUIET-}" = "1" ] || [ "${MISE_QUIET-}" = "true" ]; then
  info() {
    :
  }
else
  info() {
    echo "$@" >&2
  }
fi

error() {
  echo "$@" >&2
  exit 1
}
#endregion

#region environment setup
get_os() {
  os="$(uname -s)"
  if [ "$os" = Darwin ]; then
    echo "macos"
  elif [ "$os" = Linux ]; then
    echo "linux"
  else
    error "unsupported OS: $os"
  fi
}

get_arch() {
  musl=""
  if type ldd >/dev/null 2>/dev/null; then
    if [ "${MISE_INSTALL_MUSL-}" = "1" ] || [ "${MISE_INSTALL_MUSL-}" = "true" ]; then
      musl="-musl"
    elif [ "$(uname -o)" = "Android" ]; then
      # Android (Termux) always uses musl
      musl="-musl"
    else
      libc=$(ldd /bin/ls | grep 'musl' | head -1 | cut -d ' ' -f1)
      if [ -n "$libc" ]; then
        musl="-musl"
      fi
    fi
  fi
  arch="$(uname -m)"
  if [ "$arch" = x86_64 ]; then
    echo "x64$musl"
  elif [ "$arch" = aarch64 ] || [ "$arch" = arm64 ]; then
    echo "arm64$musl"
  elif [ "$arch" = armv7l ]; then
    echo "armv7$musl"
  else
    error "unsupported architecture: $arch"
  fi
}

get_ext() {
  if [ -n "${MISE_INSTALL_EXT:-}" ]; then
    echo "$MISE_INSTALL_EXT"
  elif [ -n "${MISE_VERSION:-}" ] && echo "$MISE_VERSION" | grep -q '^v2024'; then
    # 2024 versions don't have zstd tarballs
    echo "tar.gz"
  elif tar_supports_zstd; then
    echo "tar.zst"
  else
    echo "tar.gz"
  fi
}

tar_supports_zstd() {
  if ! command -v zstd >/dev/null 2>&1; then
    false
  # tar is bsdtar
  elif tar --version | grep -q 'bsdtar'; then
    true
  # tar version is >= 1.31
  elif tar --version | grep -q '1\.\(3[1-9]\|[4-9][0-9]\)'; then
    true
  else
    false
  fi
}

shasum_bin() {
  if command -v shasum >/dev/null 2>&1; then
    echo "shasum"
  elif command -v sha256sum >/dev/null 2>&1; then
    echo "sha256sum"
  else
    error "mise install requires shasum or sha256sum but neither is installed. Aborting."
  fi
}

get_checksum() {
  version=$1
  os=$2
  arch=$3
  ext=$4
  url="https://github.com/jdx/mise/releases/download/v${version}/SHASUMS256.txt"
  current_version="v2026.5.12"
  current_version="${current_version#v}"

  # For current version use static checksum otherwise
  # use checksum from releases
  if [ "$version" = "$current_version" ]; then
    checksum_linux_x86_64="bd0930c0b619f51ddb60e32e5cce18a5533567b2f1ba9fc4875b9f39a2bb3ed8  ./mise-v2026.5.12-linux-x64.tar.gz"
    checksum_linux_x86_64_musl="656ba82cf7f3c2224d9c9721bee9e73d47cee0029dbe1caf421287f89ecc95d9  ./mise-v2026.5.12-linux-x64-musl.tar.gz"
    checksum_linux_arm64="67c2bd96da9c6da030db4174b2dd0f8e6636c25519b23a15f0b734556e6e5ee0  ./mise-v2026.5.12-linux-arm64.tar.gz"
    checksum_linux_arm64_musl="b261c3cfaf3152c85b6abdef6be143c4b5c4cd36e8f192471d607498586d3879  ./mise-v2026.5.12-linux-arm64-musl.tar.gz"
    checksum_linux_armv7="a70649fc516e4281ec815a294150e227471ef16c797d7cc55ee9d0a37ed1d27d  ./mise-v2026.5.12-linux-armv7.tar.gz"
    checksum_linux_armv7_musl="f4f9e4f4556884e1b86849c65d5159d63761acc3ad1df88be09ffc664fd4d8be  ./mise-v2026.5.12-linux-armv7-musl.tar.gz"
    checksum_macos_x86_64="dcab53de40bbd42c10607d64081e9df328c4885db30b41c4421f27e18b8f7efa  ./mise-v2026.5.12-macos-x64.tar.gz"
    checksum_macos_arm64="5b883c868a0748dd0c595d30fd000ec5138dfabdeef2c30222866ebf34af1ae3  ./mise-v2026.5.12-macos-arm64.tar.gz"
    checksum_linux_x86_64_zstd="28ef8096e83f224d11be6aa2cf6468192ec07d6a2dd93af35bff040cfe525062  ./mise-v2026.5.12-linux-x64.tar.zst"
    checksum_linux_x86_64_musl_zstd="bf8d469e097e018f7ad182ace9ba5ccaad97fd2cdab4a253d79dc06303b85cc7  ./mise-v2026.5.12-linux-x64-musl.tar.zst"
    checksum_linux_arm64_zstd="9eb91643cd8c423ee8275e028c9915dba43b90ba26d801940008f6d7dab49aab  ./mise-v2026.5.12-linux-arm64.tar.zst"
    checksum_linux_arm64_musl_zstd="08c2fef0d5692f88459ad341a9a98e3a470097923364776e736a7ff2c6ca8a6d  ./mise-v2026.5.12-linux-arm64-musl.tar.zst"
    checksum_linux_armv7_zstd="a61252a488a9ab5339283da4017df704b941dd7a29bddec50e51ebae5b5a18ca  ./mise-v2026.5.12-linux-armv7.tar.zst"
    checksum_linux_armv7_musl_zstd="0d3051f5060da38ee823d394f45383310efec1ad3f30bc44ffb88a0d4fc63286  ./mise-v2026.5.12-linux-armv7-musl.tar.zst"
    checksum_macos_x86_64_zstd="40b4ab7c8327e7a96f926a20ee865813866563c43f7e8eee6917032dbd5ea013  ./mise-v2026.5.12-macos-x64.tar.zst"
    checksum_macos_arm64_zstd="6d6aca15552da7108a166b5aa260d10e7399b22468f7d85071f3ccd262c68ea0  ./mise-v2026.5.12-macos-arm64.tar.zst"

    # TODO: refactor this, it's a bit messy
    if [ "$ext" = "tar.zst" ]; then
      if [ "$os" = "linux" ]; then
        if [ "$arch" = "x64" ]; then
          echo "$checksum_linux_x86_64_zstd"
        elif [ "$arch" = "x64-musl" ]; then
          echo "$checksum_linux_x86_64_musl_zstd"
        elif [ "$arch" = "arm64" ]; then
          echo "$checksum_linux_arm64_zstd"
        elif [ "$arch" = "arm64-musl" ]; then
          echo "$checksum_linux_arm64_musl_zstd"
        elif [ "$arch" = "armv7" ]; then
          echo "$checksum_linux_armv7_zstd"
        elif [ "$arch" = "armv7-musl" ]; then
          echo "$checksum_linux_armv7_musl_zstd"
        else
          warn "no checksum for $os-$arch"
        fi
      elif [ "$os" = "macos" ]; then
        if [ "$arch" = "x64" ]; then
          echo "$checksum_macos_x86_64_zstd"
        elif [ "$arch" = "arm64" ]; then
          echo "$checksum_macos_arm64_zstd"
        else
          warn "no checksum for $os-$arch"
        fi
      else
        warn "no checksum for $os-$arch"
      fi
    else
      if [ "$os" = "linux" ]; then
        if [ "$arch" = "x64" ]; then
          echo "$checksum_linux_x86_64"
        elif [ "$arch" = "x64-musl" ]; then
          echo "$checksum_linux_x86_64_musl"
        elif [ "$arch" = "arm64" ]; then
          echo "$checksum_linux_arm64"
        elif [ "$arch" = "arm64-musl" ]; then
          echo "$checksum_linux_arm64_musl"
        elif [ "$arch" = "armv7" ]; then
          echo "$checksum_linux_armv7"
        elif [ "$arch" = "armv7-musl" ]; then
          echo "$checksum_linux_armv7_musl"
        else
          warn "no checksum for $os-$arch"
        fi
      elif [ "$os" = "macos" ]; then
        if [ "$arch" = "x64" ]; then
          echo "$checksum_macos_x86_64"
        elif [ "$arch" = "arm64" ]; then
          echo "$checksum_macos_arm64"
        else
          warn "no checksum for $os-$arch"
        fi
      else
        warn "no checksum for $os-$arch"
      fi
    fi
  else
    if command -v curl >/dev/null 2>&1; then
      debug ">" curl -fsSL "$url"
      checksums="$(curl --compressed -fsSL "$url")"
    else
      if command -v wget >/dev/null 2>&1; then
        debug ">" wget -qO - "$url"
        checksums="$(wget -qO - "$url")"
      else
        error "mise standalone install specific version requires curl or wget but neither is installed. Aborting."
      fi
    fi
    # TODO: verify with minisign or gpg if available

    checksum="$(echo "$checksums" | grep "$os-$arch.$ext")"
    if ! echo "$checksum" | grep -Eq "^([0-9a-f]{32}|[0-9a-f]{64})"; then
      warn "no checksum for mise $version and $os-$arch"
    else
      echo "$checksum"
    fi
  fi
}

#endregion

download_file() {
  url="$1"
  download_dir="$2"
  filename="$(basename "$url")"
  file="$download_dir/$filename"

  info "mise: installing mise..."

  if command -v curl >/dev/null 2>&1; then
    debug ">" curl -#fLo "$file" "$url"
    curl -#fLo "$file" "$url"
  else
    if command -v wget >/dev/null 2>&1; then
      debug ">" wget -qO "$file" "$url"
      stderr=$(mktemp)
      wget -O "$file" "$url" >"$stderr" 2>&1 || error "wget failed: $(cat "$stderr")"
      rm "$stderr"
    else
      error "mise standalone install requires curl or wget but neither is installed. Aborting."
    fi
  fi

  echo "$file"
}

install_mise() {
  version="${MISE_VERSION:-v2026.5.12}"
  version="${version#v}"
  current_version="v2026.5.12"
  current_version="${current_version#v}"
  os="${MISE_INSTALL_OS:-$(get_os)}"
  arch="${MISE_INSTALL_ARCH:-$(get_arch)}"
  ext="${MISE_INSTALL_EXT:-$(get_ext)}"
  install_path="${MISE_INSTALL_PATH:-$HOME/.local/bin/mise}"
  install_dir="$(dirname "$install_path")"
  install_from_github="${MISE_INSTALL_FROM_GITHUB:-}"
  if [ "$version" != "$current_version" ] || [ "$install_from_github" = "1" ] || [ "$install_from_github" = "true" ]; then
    tarball_url="https://github.com/jdx/mise/releases/download/v${version}/mise-v${version}-${os}-${arch}.${ext}"
  elif [ -n "${MISE_TARBALL_URL-}" ]; then
    tarball_url="$MISE_TARBALL_URL"
  else
    tarball_url="https://mise.en.dev/v${version}/mise-v${version}-${os}-${arch}.${ext}"
  fi

  download_dir="$(mktemp -d)"
  cache_file=$(download_file "$tarball_url" "$download_dir")
  debug "mise-setup: tarball=$cache_file"

  debug "validating checksum"
  cd "$(dirname "$cache_file")" && get_checksum "$version" "$os" "$arch" "$ext" | "$(shasum_bin)" -c >/dev/null

  # extract tarball
  if [ -d "$install_path" ]; then
    error "MISE_INSTALL_PATH '$install_path' is a directory. Please set it to a file path, e.g. '$install_path/mise'."
  fi
  mkdir -p "$install_dir"
  rm -f "$install_path"
  extract_dir="$(mktemp -d)"
  cd "$extract_dir"
  if [ "$ext" = "tar.zst" ] && ! tar_supports_zstd; then
    zstd -d -c "$cache_file" | tar -xf -
  else
    tar -xf "$cache_file"
  fi
  mv mise/bin/mise "$install_path"

  # cleanup
  cd / # Move out of $extract_dir before removing it
  rm -rf "$download_dir"
  rm -rf "$extract_dir"

  info "mise: installed successfully to $install_path"
}

after_finish_help() {
  case "${SHELL:-}" in
  */zsh)
    info "mise: run the following to activate mise in your shell:"
    info "echo \"eval \\\"\\\$($install_path activate zsh)\\\"\" >> \"${ZDOTDIR-$HOME}/.zshrc\""
    info ""
    info "mise: run \`mise doctor\` to verify this is set up correctly"
    ;;
  */bash)
    info "mise: run the following to activate mise in your shell:"
    info "echo \"eval \\\"\\\$($install_path activate bash)\\\"\" >> ~/.bashrc"
    info ""
    info "mise: run \`mise doctor\` to verify this is set up correctly"
    ;;
  */fish)
    info "mise: run the following to activate mise in your shell:"
    info "echo \"$install_path activate fish | source\" >> ~/.config/fish/config.fish"
    info ""
    info "mise: run \`mise doctor\` to verify this is set up correctly"
    ;;
  *)
    info "mise: run \`$install_path --help\` to get started"
    ;;
  esac
}

install_mise
if [ "${MISE_INSTALL_HELP-}" != 0 ]; then
  after_finish_help
fi
