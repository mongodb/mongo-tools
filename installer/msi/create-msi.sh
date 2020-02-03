#!/bin/bash
# Copyright (c) 2020-Present MongoDB Inc.

MDBTools_VER="$(cat ./VERSION.txt)"

# clear MSI_BUILD_DIR
MSI_BUILD_DIR="$PROJECT_ROOT/msi_build")
rm -rf "$MSI_BUILD_DIR"
mkdir -p "$MSI_BUILD_DIR"

cd "$MSI_BUILD_DIR"

# copy msi sources to current directory
cp -R "$PROJECT_ROOT/installer/msi/"* ./

# copy README.md from the PROJECT_ROOT
cp "$PROJECT_ROOT/README.md" ./

# copy bin dir.
cp "$PROJECT_ROOT"/bin/* ./

# copy openssl dlls.
cp "/cygdrive/c/openssl/bin/*.dll" ./

POWERSHELL='C:/Windows/System32/WindowsPowerShell/v1.0/powershell.exe' # 64-bit powershell
"$POWERSHELL" \
        -NoProfile \
        -NoLogo \
        -NonInteractive \
        -ExecutionPolicy ByPass \
        -File ./build-msi.ps1 \
        -VersionLabel "$MDBTools_VER"
