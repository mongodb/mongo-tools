#!/bin/sh

# This is copied from mise.run.

set -o errexit
set -o nounset

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
  # XXX - MongoDB-specific change - this lets us install Mise on RHEL 7.0 systems, which have an
  # older glibc than the one needed by the non-musl mise binary.
  if [ "$(uname -s)" = Linux ]; then
    musl="-musl"
  fi
  if type ldd >/dev/null 2>/dev/null; then
    libc=$(ldd /bin/ls | grep 'musl' | head -1 | cut -d ' ' -f1)
    if [ -n "$libc" ]; then
      musl="-musl"
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
  elif command -v zstd >/dev/null 2>&1; then
    echo "tar.zst"
  else
    echo "tar.gz"
  fi
}

tar_supports_zstd() {
  # tar is bsdtar or version is >= 1.31
  if tar --version | grep -q 'bsdtar' && command -v zstd >/dev/null 2>&1; then
    true
  elif tar --version | grep -q '1\.(3[1-9]|[4-9][0-9]'; then
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
  os="$(get_os)"
  arch="$(get_arch)"
  ext="$(get_ext)"
  url="https://github.com/jdx/mise/releases/download/v${version}/SHASUMS256.txt"

  # For current version use static checksum otherwise
  # use checksum from releases
  if [ "$version" = "v2025.2.7" ]; then
    checksum_linux_x86_64="a29c2600fcefc0488d3e0e16d5cfc9ccfccdb06f6348c4cd4050a5eeaf9dd53d  ./mise-v2025.2.7-linux-x64.tar.gz"
    checksum_linux_x86_64_musl="bed90247c1f637377855bba1c90056d7d1d1a3c36c6f2370054152df45b57f02  ./mise-v2025.2.7-linux-x64-musl.tar.gz"
    checksum_linux_arm64="fe4f89997d8b4c9e7a1d3ac8733117d5463021c8c87f04a40deb27d34752c15f  ./mise-v2025.2.7-linux-arm64.tar.gz"
    checksum_linux_arm64_musl="d0c2519b2db28cd51ac9ffd966bc6a484f8942e7fa19943460a9717ccc68e85e  ./mise-v2025.2.7-linux-arm64-musl.tar.gz"
    checksum_linux_armv7="7a1d9d36135b35a82a636780acc638ca3f9cb240efb4786ce3905c8a187c0d19  ./mise-v2025.2.7-linux-armv7.tar.gz"
    checksum_linux_armv7_musl="abb499fff1b20dc4044206d4a00753c9e69e26d64c05b489285f7730de43075b  ./mise-v2025.2.7-linux-armv7-musl.tar.gz"
    checksum_macos_x86_64="b8c04d4099b9956b48e3942a43e6425e6c3a5c813da8b5c18a9d7eb473cf518f  ./mise-v2025.2.7-macos-x64.tar.gz"
    checksum_macos_arm64="376f463de5266b446da78c76de0030c9d2df8a4b5ca58c06cb608a05c2cb6d0a  ./mise-v2025.2.7-macos-arm64.tar.gz"
    checksum_linux_x86_64_zstd="f1af6e34936224d0a2d2f8dfc123e3cd69863718722c3eef75962ed18db0dfd2  ./mise-v2025.2.7-linux-x64.tar.zst"
    checksum_linux_x86_64_musl_zstd="be4f0ab5945f0c821f0b60c12935e8e6cae9ebc3807a941cccf36af4bee42b48  ./mise-v2025.2.7-linux-x64-musl.tar.zst"
    checksum_linux_arm64_zstd="58ac3e32bea8b931619d02a1fdb9c86d990835b2f125e44caa9f4a689ed5f4e8  ./mise-v2025.2.7-linux-arm64.tar.zst"
    checksum_linux_arm64_musl_zstd="0d22664b59dc930ae58048671de5284122dbd66f409fd573c2160d94e6f9f9b7  ./mise-v2025.2.7-linux-arm64-musl.tar.zst"
    checksum_linux_armv7_zstd="7bf64f9562d505a31c17c164bfb2dde13e92babda5aace6f1de54d594c70f908  ./mise-v2025.2.7-linux-armv7.tar.zst"
    checksum_linux_armv7_musl_zstd="51481e505a74cbed5a66860e54b4e3cfe26c28e7b95cd44f15f9b38075b9c3b6  ./mise-v2025.2.7-linux-armv7-musl.tar.zst"
    checksum_macos_x86_64_zstd="b17fde3a4bbfb98347ae8a93662e4b12285b6e7e8a7b57c0c6db9e1e4a0420e9  ./mise-v2025.2.7-macos-x64.tar.zst"
    checksum_macos_arm64_zstd="181243d3901a31430e2214bc137f915a4960b207cd15bac9ae503996e4e25c1e  ./mise-v2025.2.7-macos-arm64.tar.zst"

    # TODO: refactor this, it's a bit messy
    if [ "$(get_ext)" = "tar.zst" ]; then
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
        stderr=$(mktemp)
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
  filename="$(basename "$url")"
  cache_dir="$(mktemp -d)"
  file="$cache_dir/$filename"

  info "mise: installing mise..."

  if command -v curl >/dev/null 2>&1; then
    debug ">" curl -#fLo "$file" "$url"
    curl -#fLo "$file" "$url"
  else
    if command -v wget >/dev/null 2>&1; then
      debug ">" wget -qO "$file" "$url"
      stderr=$(mktemp)
      wget -O "$file" "$url" >"$stderr" 2>&1 || error "wget failed: $(cat "$stderr")"
    else
      error "mise standalone install requires curl or wget but neither is installed. Aborting."
    fi
  fi

  echo "$file"
}

install_mise() {
  version="${MISE_VERSION:-v2025.2.7}"
  version="${version#v}"
  os="$(get_os)"
  arch="$(get_arch)"
  ext="$(get_ext)"
  install_path="${MISE_INSTALL_PATH:-$HOME/.local/bin/mise}"
  install_dir="$(dirname "$install_path")"
  tarball_url="https://github.com/jdx/mise/releases/download/v${version}/mise-v${version}-${os}-${arch}.${ext}"

  cache_file=$(download_file "$tarball_url")
  debug "mise-setup: tarball=$cache_file"

  debug "validating checksum"
  cd "$(dirname "$cache_file")" && get_checksum "$version" | "$(shasum_bin)" -c >/dev/null

  # extract tarball
  mkdir -p "$install_dir"
  rm -rf "$install_path"
  cd "$(mktemp -d)"
  if [ "$(get_ext)" = "tar.zst" ] && ! tar_supports_zstd; then
    zstd -d -c "$cache_file" | tar -xf -
  else
    tar -xf "$cache_file"
  fi
  mv mise/bin/mise "$install_path"
  info "mise: installed successfully to $install_path"
}

after_finish_help() {
  case "${SHELL:-}" in
  */zsh)
    info "mise: run the following to activate mise in your shell:"
    info "echo \"eval \\\"\\\$($install_path activate zsh)\\\"\" >> \"${ZDOTDIR-$HOME}/.zshrc\""
    info ""
    info "mise: run \`mise doctor\` to verify this is setup correctly"
    ;;
  */bash)
    info "mise: run the following to activate mise in your shell:"
    info "echo \"eval \\\"\\\$($install_path activate bash)\\\"\" >> ~/.bashrc"
    info ""
    info "mise: run \`mise doctor\` to verify this is setup correctly"
    ;;
  */fish)
    info "mise: run the following to activate mise in your shell:"
    info "echo \"$install_path activate fish | source\" >> ~/.config/fish/config.fish"
    info ""
    info "mise: run \`mise doctor\` to verify this is setup correctly"
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
