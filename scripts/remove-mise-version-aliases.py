#!/usr/bin/env python3
"""Remove mise's per-tool version aliases from a mise installs tree.

For each installed version mise writes convenience aliases under installs/<tool>/: the version
prefixes (e.g. `1`, `1.27`) and `latest`, each pointing at a concrete version directory. On Unix
they are relative symlinks; from mise 2026.10 on Windows they are NTFS junctions (older Windows mise
wrote a small file holding the target path instead).

Evergreen's cache.save cannot archive a junction: it tries to read it as a file and aborts the whole
task with "Incorrect function". Removing the aliases before the cache is saved avoids that. They are
not needed here: mise.toml pins full versions, which mise resolves to the concrete install directory
directly, and mise recreates the aliases on demand.

Some CI images (e.g. SUSE 12) still run Python 2 as python3, so keep this 2/3 compatible.

Usage: remove-mise-version-aliases.py <installs_dir>
"""

import os
import stat
import sys

FILE_ATTRIBUTE_REPARSE_POINT = getattr(stat, "FILE_ATTRIBUTE_REPARSE_POINT", 0x400)


def is_version_alias(path):
    try:
        st = os.lstat(path)
    except OSError:
        return False
    if stat.S_ISLNK(st.st_mode):
        return True
    if getattr(st, "st_file_attributes", 0) & FILE_ATTRIBUTE_REPARSE_POINT:
        return True
    # Pre-2026.10 Windows mise stored an alias as a small file containing "./<version>".
    if stat.S_ISREG(st.st_mode) and st.st_size < 1024:
        try:
            with open(path) as f:
                return f.read(2) == "./"
        except (IOError, OSError):
            return False
    return False


def remove_version_alias(path):
    try:
        os.unlink(path)
    except (IOError, OSError):
        # A Windows junction is a directory: rmdir removes the link itself, not its target.
        os.rmdir(path)


def main(installs_dir):
    if not os.path.isdir(installs_dir):
        return
    for tool in os.listdir(installs_dir):
        tool_path = os.path.join(installs_dir, tool)
        if not os.path.isdir(tool_path):
            continue
        for name in os.listdir(tool_path):
            path = os.path.join(tool_path, name)
            if is_version_alias(path):
                remove_version_alias(path)


if __name__ == "__main__":
    main(sys.argv[1])
