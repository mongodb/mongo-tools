#!/usr/bin/env python3
"""Recreate bin/ symlinks for a single npm package inside a mise install directory.

Evergreen's cache.save command dereferences symlinks, converting them to regular files. npm global
installs create bin/ symlinks pointing to lib/node_modules/{pkg}/bin/{file}. After a cache restore
these become plain copies of those files, which breaks Node.js ESM relative imports (the import
base URL is now bin/ instead of lib/node_modules/{pkg}/bin/). This script recreates those symlinks
so the cache-hit path behaves the same as a fresh install.

Usage: recreate-npm-bin-symlinks.py <install_dir> <pkg_name> <pkg_json_path>
"""

import json
import os
import sys

install_dir, pkg_name, pkg_json_path = sys.argv[1], sys.argv[2], sys.argv[3]

with open(pkg_json_path) as f:
    pkg = json.load(f)

bins = pkg.get("bin", {})
if isinstance(bins, str):
    bins = {pkg_name: bins}

bin_dir = os.path.join(install_dir, "bin")
os.makedirs(bin_dir, exist_ok=True)

for name, rel_path in bins.items():
    link = os.path.join(bin_dir, name)
    rel_path_clean = rel_path[2:] if rel_path.startswith("./") else rel_path
    target = os.path.join("..", "lib", "node_modules", pkg_name, rel_path_clean)
    try:
        os.unlink(link)
    except FileNotFoundError:
        pass
    os.symlink(target, link)
    # Make the link target executable.
    real_target = os.path.join(install_dir, "lib", "node_modules", pkg_name, rel_path_clean)
    if os.path.exists(real_target):
        os.chmod(real_target, 0o755)
