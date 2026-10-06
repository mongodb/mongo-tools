#!/usr/bin/env python3
"""Print the connectionString from mongodb-runner's `start --json` output.

The runner prints a single-line JSON object to stdout, but callers capture it alongside the runner's
stderr diagnostics, so scan the input for the first JSON object that carries a connectionString and
print its value. Prints a message to stderr and exits non-zero when none is found.

Usage: parse-connection-string.py < <runner output>
"""

import json
import sys

for line in sys.stdin:
    line = line.strip()
    if not line.startswith("{"):
        continue
    try:
        obj = json.loads(line)
    except ValueError:
        continue
    if isinstance(obj, dict) and obj.get("connectionString"):
        # Write the value with no trailing newline. Python's text-mode stdout translates "\n" to
        # "\r\n" on Windows, and callers capture our output with command substitution, which strips
        # the "\n" but leaves the "\r" (a stray carriage return that corrupts the connection string).
        # With no newline there is nothing to translate, so the captured value is clean everywhere.
        sys.stdout.write(obj["connectionString"])
        sys.exit(0)

print(
    "parse-connection-string.py: no JSON object with a connectionString in the input",
    file=sys.stderr,
)
sys.exit(1)
