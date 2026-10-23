#!/usr/bin/env python3
"""Recreate bin/ symlinks for a single npm package inside a mise install directory.

npm global installs create bin/ symlinks pointing to lib/node_modules/{pkg}/bin/{file}. If a cache
restore ever leaves those as plain copies of the files, Node.js ESM relative imports break (the
import base URL is bin/ instead of lib/node_modules/{pkg}/bin/), so this recreates the symlinks.

Some CI images (e.g. SUSE 12) still run Python 2 as python3, so keep this 2/3 compatible.

Usage: recreate-npm-bin-symlinks.py <install_dir> <pkg_name> <pkg_json_path>
"""

import json
import os
import sys

install_dir, pkg_name, pkg_json_path = sys.argv[1], sys.argv[2], sys.argv[3]

with open(pkg_json_path) as f:
    pkg = json.load(f)

bins = pkg.get("bin", {})
if not isinstance(bins, dict):
    bins = {pkg_name: bins}

bin_dir = os.path.join(install_dir, "bin")
if not os.path.isdir(bin_dir):
    os.makedirs(bin_dir)

for name, rel_path in bins.items():
    link = os.path.join(bin_dir, name)
    rel_path_clean = rel_path[2:] if rel_path.startswith("./") else rel_path
    target = os.path.join("..", "lib", "node_modules", pkg_name, rel_path_clean)
    try:
        os.unlink(link)
    except (IOError, OSError):
        pass
    os.symlink(target, link)
    # Make the link target executable.
    real_target = os.path.join(install_dir, "lib", "node_modules", pkg_name, rel_path_clean)
    if os.path.exists(real_target):
        os.chmod(real_target, 0o755)
