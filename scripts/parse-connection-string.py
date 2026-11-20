#!/usr/bin/env python3
"""Print the connectionString from mongodb-runner's `start --json` output and save the rest.

The runner prints a single-line JSON object to stdout, but callers capture it alongside the runner's
stderr diagnostics, so scan the input for the first JSON object that carries a connectionString and
print its value. The same JSON object is also written to the file named by the argument, without the
connectionString/connectionStringWithOidc fields. Those are the only fields that can carry
credentials: the runner itself redacts passwords, keyfile contents, and certificate passwords from
the serialized cluster data, so the saved file is safe to archive. Prints a message to stderr and
exits non-zero when no such object is found.

This is a Python program rather than a jq pipeline for three reasons. The input interleaves the
runner's debug diagnostics with the JSON result (start-test-cluster.sh captures stdout and stderr
in one file), and jq cannot skip non-JSON lines, so a text filter would be needed in front of it
anyway. The connection string must print with no trailing newline (see below), while jq always
appends one, so on Windows the captured value would depend on the jq build's line endings and need
defensive \r stripping at every call site. And python3 is already required by the scripts that call
this on every host they run on, Windows included, while jq is nowhere guaranteed: it is not in the
mise toolchain, nothing in this repo installs it, the Evergreen image the DSC tasks run on carries
no jq package, and stock macOS does not ship it.

Usage: parse-connection-string.py <redacted-json-path> < <runner output>
"""

import json
import sys

if len(sys.argv) != 2:
    print(
        "usage: parse-connection-string.py <redacted-json-path> < <runner output>",
        file=sys.stderr,
    )
    sys.exit(2)

save_path = sys.argv[1]

for line in sys.stdin:
    line = line.strip()
    if not line.startswith("{"):
        continue
    try:
        obj = json.loads(line)
    except ValueError:
        continue
    if isinstance(obj, dict) and obj.get("connectionString"):
        # The connection-string fields are the only credential-bearing ones; everything else is
        # the runner's already-redacted serialized cluster data.
        saved = {
            key: value
            for key, value in obj.items()
            if key not in ("connectionString", "connectionStringWithOidc")
        }
        # The saved file is a post-mortem aid, so a failure to write it must not turn a
        # successful cluster start into a reported failure: the connection string still prints.
        try:
            with open(save_path, "w", encoding="utf-8") as f:
                json.dump(saved, f, indent=2)
                f.write("\n")
        except OSError as error:
            print(
                f"parse-connection-string.py: could not write {save_path}: {error}",
                file=sys.stderr,
            )
        # Write the value with no trailing newline. Python's text-mode stdout translates "\n" to
        # "\r\n" on Windows, and callers capture our output with command substitution, which strips
        # the "\n" but leaves the "\r" (a stray carriage return that corrupts the connection
        # string). With no newline there is nothing to translate, so the captured value is clean
        # everywhere.
        sys.stdout.write(obj["connectionString"])
        sys.exit(0)

print(
    "parse-connection-string.py: no JSON object with a connectionString in the input",
    file=sys.stderr,
)
sys.exit(1)
