#!/usr/bin/env python3

# Stops a cluster started by scripts/start-test-cluster.sh, including a DSC cluster started with
# --dsc. This is a no-op when the cluster was not started, so it is safe to run unconditionally: it
# runs in Evergreen's project-wide post block for every task.

import argparse
import os
import re
import subprocess
import sys
import tarfile
from pathlib import Path

# abspath rather than resolve(): on Windows, resolve() follows links, and on Evergreen hosts it turns
# C:\data paths into Z:\data ones. The tests build their paths from the C:\ form, and the tools
# reject a URI whose tlsCAFile differs from the --sslCAFile passed on the command line.
REPO_ROOT = Path(os.path.abspath(__file__)).parent.parent

DSC_CLUSTER_ID = "tools-dsc"

USAGE = """\
Stops a cluster started by scripts/start-test-cluster.sh, or a DSC cluster started with --dsc.

Usage:
  ./scripts/stop-test-cluster.sh [--id=<id>] [--dsc]
"""


def main():
    args = parse_args(sys.argv[1:])
    if args.dsc:
        stop_dsc_cluster()
    else:
        stop_runner_cluster(args.id)


def stop_runner_cluster(cluster_id):
    cluster_dir = REPO_ROOT / "test-cluster" / cluster_id
    if not cluster_exists(cluster_dir):
        print("no %s to stop" % cluster_dir, file=sys.stderr)
        return

    status = runner_stop(cluster_id)
    if status != 0:
        sys.exit(status)
    remove_connection_files(cluster_dir)


def stop_dsc_cluster():
    # A DSC cluster uses the same layout as a runner-managed one, so it is just cluster
    # "tools-dsc" plus log archiving.
    cluster_dir = REPO_ROOT / "test-cluster" / DSC_CLUSTER_ID
    if not cluster_exists(cluster_dir):
        print("no %s to stop" % cluster_dir, file=sys.stderr)
        return

    # The teardown's exit status is held rather than allowed to end the script, so that a failed
    # teardown still produces an archive. The teardown is skipped when there is no connection
    # string, because the runner tears its own compose project down when start fails.
    status = 0
    if (cluster_dir / "connection-string").is_file():
        status = runner_stop(DSC_CLUSTER_ID)

    # Archiving happens after the teardown, so that anything the runner writes into logDir during
    # teardown is included.
    archive_logs(cluster_dir)
    remove_connection_files(cluster_dir)

    if status != 0:
        sys.exit(status)


def cluster_exists(cluster_dir):
    # This runs in the Evergreen project-wide post block, so it has to be a no-op for the tasks that
    # never started a cluster. We check for logs as well as the connection string because a start
    # that failed partway through leaves logs but no connection string.
    return (cluster_dir / "connection-string").is_file() or (cluster_dir / "logs").is_dir()


def runner_stop(cluster_id):
    runner = ["mise", "exec", "node", "npm:@mongodb-js/mongodb-runner", "--", "mongodb-runner"]
    return subprocess.run(runner + ["stop", "--id=%s" % cluster_id]).returncode


def archive_logs(cluster_dir):
    # Archived here rather than with an `s3.put` because this runs in Evergreen's project-wide post
    # block. A filter that matches nothing would require `s3.put` to allow `optional` for multi-file
    # puts, which it does not.
    log_dir = cluster_dir / "logs"
    if not log_dir.is_dir():
        return

    try:
        with tarfile.open(cluster_dir / "logs.tgz", "w:gz") as archive:
            archive.add(log_dir, arcname="logs")
    except OSError:
        print("could not archive the cluster logs", file=sys.stderr)


def remove_connection_files(cluster_dir):
    # Not unlink(missing_ok=True), which needs Python 3.8.
    for name in ("connection-string", "expansions.yml"):
        try:
            (cluster_dir / name).unlink()
        except FileNotFoundError:
            pass


def parse_args(argv):
    if "-h" in argv or "--help" in argv:
        print(USAGE, end="")
        sys.exit(0)

    parser = argparse.ArgumentParser(prog="stop-test-cluster.sh", add_help=False, allow_abbrev=False)
    parser.add_argument("--id", default=os.environ.get("CLUSTER_ID") or "tools-test")
    parser.add_argument("--dsc", action="store_true")
    args = parser.parse_args(argv)

    # The id is used in filesystem paths and mongodb-runner rejects ids outside [A-Za-z0-9_-], so
    # validate it before we stop or remove anything.
    if not re.fullmatch(r"[A-Za-z0-9_-]+", args.id):
        print(
            "invalid cluster id '%s'; use only letters, digits, underscores, and hyphens" % args.id,
            file=sys.stderr,
        )
        sys.exit(1)

    return args


if __name__ == "__main__":
    main()
