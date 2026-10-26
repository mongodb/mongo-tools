#!/usr/bin/env python3

# Starts a MongoDB cluster with mongodb-runner for local development and prints the connection
# string that you should export as `TOOLS_TESTING_MONGOD`. Stop it with scripts/stop-test-cluster.sh.

import argparse
import json
import os
import re
import shutil
import subprocess
import sys
import time
from pathlib import Path

# abspath rather than resolve(): on Windows, resolve() follows links, and on Evergreen hosts it turns
# C:\data paths into Z:\data ones. The tests build their paths from the C:\ form, and the tools
# reject a URI whose tlsCAFile differs from the --sslCAFile passed on the command line.
REPO_ROOT = Path(os.path.abspath(__file__)).parent.parent
SCRIPT_DIR = REPO_ROOT / "scripts"

RUNNER = ["mise", "exec", "node", "npm:@mongodb-js/mongodb-runner", "--", "mongodb-runner"]

DSC_CLUSTER_ID = "tools-dsc"

# The feature flags a DSC cluster runs with in production, so the integration suite exercises the
# same filtered metrics a real customer would see. The mongosync tool enables the same four flags.
DSC_MONGOD_ARGS = [
    "--setParameter",
    "featureFlagCollStatsMetricsFiltering=true",
    "--setParameter",
    "featureFlagMetricsFiltering=true",
    "--setParameter",
    "featureFlagReplSetGetStatusMetricsFiltering=true",
    "--setParameter",
    "featureFlagServerStatusMetricsFiltering=true",
]

USAGE = """\
Starts a MongoDB cluster with mongodb-runner and prints the connection string that you should export
as `TOOLS_TESTING_MONGOD`.

The runner picks free ports randomly.

Usage:
  ./scripts/start-test-cluster.sh [options] [-- <extra mongod args>]

An option with a value can be written as either "--opt value" or "--opt=value".

Options:
  --topology standalone|replset|sharded   Cluster topology (default standalone)
  --version <ver>                         Server version for mongodb-runner to download (default 8.0)
  --bin-dir <dir>                         Use existing mongod/mongos binaries instead of downloading
  --id <id>                               Cluster id, also used by stop (default tools-test)
  --shards <n>                            Number of shards for the sharded topology (default 2)
  --secondaries <n>                       Number of secondaries per replica set (default 2)
  --tls                                   Require TLS using the certificates under common/db/testdata
  --create-user                           Create an admin user for auth runs (needs AUTH_USERNAME and
                                          AUTH_PASSWORD)
  --evg-expansion-var=<name>              Write an Evergreen expansions file setting name to the URI
  --dsc                                   Start a disaggregated-storage (DSC) cluster instead of a
                                          runner-managed one. Combines with --topology (default
                                          replset) and needs docker compose plus registry access.

Every option can also be set through the matching environment variable (TOPOLOGY, VERSION, BIN_DIR,
CLUSTER_ID, SHARDS, SECONDARIES, USE_TLS, CREATE_USER, AUTH_USERNAME, AUTH_PASSWORD,
EVG_EXPANSION_VAR, ADDITIONAL_ARGS, DSC). CI uses the environment form so that the auth password is
not passed on the command line.

Stop the cluster with scripts/stop-test-cluster.sh.
"""


def main():
    args = parse_args(sys.argv[1:])
    if args.dsc:
        start_dsc_cluster(args)
    else:
        start_asc_cluster(args)


def parse_args(argv):
    # Split off the extra mongod args at the first `--`, so that everything before it has to be a
    # recognised option and everything after it is passed through untouched.
    if "--" in argv:
        index = argv.index("--")
        head, extra = argv[:index], argv[index + 1 :]
    else:
        head, extra = argv, []

    if "-h" in head or "--help" in head:
        print(USAGE, end="")
        sys.exit(0)

    # AWS auth has no static user, but the runner authenticates as an admin to verify the server came
    # up, so it needs one. Create a bootstrap SCRAM user; the AWS auth test ignores it and
    # authenticates with its own MONGODB-AWS URI. AWS auth also requires an Enterprise server, so
    # this path only runs in CI.
    aws_auth = env_true("AWS_AUTH")
    create_user = env_true("CREATE_USER")
    auth_username = env("AUTH_USERNAME")
    auth_password = env("AUTH_PASSWORD")
    if aws_auth:
        # aws_e2e_assume_role.js authenticates as this user to create the $external AWS user, and the
        # runner needs an admin user to verify the server came up, so use the credentials that script
        # expects.
        create_user = True
        auth_username = "bob"
        auth_password = "pwd123"

    parser = argparse.ArgumentParser(prog="start-test-cluster.sh", add_help=False)
    parser.add_argument("--topology", default=env("TOPOLOGY", "standalone"))
    parser.add_argument("--version", default=env("VERSION", "8.0"))
    parser.add_argument("--bin-dir", default=env("BIN_DIR"))
    parser.add_argument("--id", default=env("CLUSTER_ID", "tools-test"))
    parser.add_argument("--shards", type=int, default=env("SHARDS", "2"))
    parser.add_argument("--secondaries", type=int, default=env("SECONDARIES", "2"))
    parser.add_argument("--tls", action="store_true", default=env_true("USE_TLS"))
    parser.add_argument("--create-user", action="store_true", default=create_user)
    parser.add_argument("--evg-expansion-var", default=env("EVG_EXPANSION_VAR"))
    parser.add_argument("--dsc", action="store_true", default=env_true("DSC"))
    args = parser.parse_args(head)

    if args.topology not in ("standalone", "replset", "sharded"):
        fail("invalid topology '%s'; expected standalone, replset, or sharded" % args.topology)

    # DSC is a storage backend, not a topology: a DSC cluster is itself a replica set (or sharded
    # cluster). It is always at least a replica set, so treat the standalone default as replset.
    if args.dsc and args.topology == "standalone":
        args.topology = "replset"

    # mongodb-runner rejects ids outside [A-Za-z0-9_-], but we build filesystem paths from the id
    # before it gets that far, so validate it up front.
    if not re.fullmatch(r"[A-Za-z0-9_-]+", args.id):
        fail("invalid cluster id '%s'; use only letters, digits, underscores, and hyphens" % args.id)

    if args.create_user and not (auth_username and auth_password):
        fail("CREATE_USER=true requires both AUTH_USERNAME and AUTH_PASSWORD")

    args.aws_auth = aws_auth
    args.auth_username = auth_username
    args.auth_password = auth_password
    args.extra_mongod_args = extra

    return args


def start_asc_cluster(args):
    ca_file = native_path(REPO_ROOT / "common" / "db" / "testdata" / "ca-ia.pem")
    cert_file = native_path(REPO_ROOT / "common" / "db" / "testdata" / "test-server.pem")

    # We use the binaries in `./bin` if they already exist. This lets us test arbitrary Server
    # binaries, not just the ones downloaded by mongodb-downloader.
    bin_dir = args.bin_dir
    if not bin_dir:
        for name in ("mongod.exe", "mongod"):
            if (REPO_ROOT / "bin" / name).is_file():
                bin_dir = str(REPO_ROOT / "bin")
                break

    cluster_dir, log_dir = cluster_dirs(args.id)
    runner_args = runner_start_args(args, args.id, log_dir)

    if bin_dir:
        runner_args.append("--binDir=%s" % native_path(bin_dir))
    else:
        runner_args.append("--version=%s" % args.version)

    config = build_runner_config(args, ca_file, cert_file)
    if config:
        config_file = cluster_dir / "runner-config.json"
        config_file.write_text(json.dumps(config, indent=2) + "\n")
        runner_args += ["--config", native_path(config_file)]

    mongod_args = build_mongod_args(args, ca_file, cert_file)
    if mongod_args:
        runner_args += ["--"] + mongod_args

    connection_string = launch_runner(runner_args, cluster_dir, log_dir)
    finish_start(args, cluster_dir, connection_string)


def start_dsc_cluster(args):
    cluster_dir, log_dir = cluster_dirs(DSC_CLUSTER_ID)
    install_dir = cluster_dir / "server"

    if not command_succeeds(["docker", "compose", "version"], None):
        fail("docker compose v2 is required; see the DSC prerequisites in mongodb-runner's docs")

    # The SLS images live in a private registry. Evergreen does its own login in an earlier step,
    # under an assumed role it does not pass to this script.
    if not os.environ.get("EVG_WORKDIR"):
        run_checked(["bash", str(SCRIPT_DIR / "authenticate-sls-ecr.sh")], None)

    # The DSC binaries and SLS files are downloaded by the "download mongod and shell" Evergreen
    # function before this runs.

    print(
        "Starting a DSC replica set (SLS image tag from the build's manifest). The first run pulls"
        " all SLS images and can take several minutes.",
        file=sys.stderr,
    )

    runner_args = runner_start_args(args, DSC_CLUSTER_ID, log_dir)
    runner_args += [
        "--slsDir=%s" % native_path(install_dir),
        "--binDir=%s" % native_path(install_dir / "bin"),
        "--debug",
        "--",
    ] + DSC_MONGOD_ARGS

    # stop-test-cluster.sh checks for this marker before it tears anything down, so create it before
    # starting: a failed `mongodb-runner start` still tears the cluster down.
    (cluster_dir / "connection-string").touch()
    connection_string = launch_runner(
        runner_args,
        cluster_dir,
        log_dir,
        stderr_to_log=False,
        teardown=teardown_dsc_cluster,
    )

    if not args.evg_expansion_var:
        args.evg_expansion_var = "TOOLS_TESTING_MONGOD"
    finish_start(args, cluster_dir, connection_string)


def runner_start_args(args, cluster_id, log_dir):
    runner_args = [
        "start",
        "--json",
        "--topology=%s" % args.topology,
        "--id=%s" % cluster_id,
        "--logDir=%s" % native_path(log_dir),
    ]
    if args.topology == "sharded":
        runner_args.append("--shards=%d" % args.shards)
    return runner_args


def cluster_dirs(cluster_id):
    cluster_dir = REPO_ROOT / "test-cluster" / cluster_id
    log_dir = cluster_dir / "logs"
    log_dir.mkdir(parents=True, exist_ok=True)
    return cluster_dir, log_dir


def launch_runner(runner_args, cluster_dir, log_dir, env=None, stderr_to_log=True, teardown=None):
    # The runner prints a single-line JSON object once the cluster is up, but on Windows it does not
    # exit: the detached mongod keeps its Node event loop alive. Capturing stdout with a pipe would
    # then wait forever for the pipe to close, so the runner writes to a file and we poll it for the
    # JSON line, stopping early if the process dies.
    runner_output = cluster_dir / "runner-start.log"
    with open(runner_output, "wb") as output:
        process = subprocess.Popen(
            RUNNER + runner_args,
            env=env,
            stdin=subprocess.DEVNULL,
            stdout=output,
            # ASC merges stderr into the log so a failed start can print its tail. DSC leaves stderr
            # on the terminal so the --debug compose output stays visible during the slow startup.
            stderr=subprocess.STDOUT if stderr_to_log else None,
        )

    started = False
    try:
        wait_for_connection_string(runner_output, process)
        connection_string = parse_connection_string(read_text(runner_output))

        # Best-effort. The runner never stops the cluster on a signal, so this can't take the
        # cluster down, and we don't wait because on Windows the kill may not land.
        try:
            process.terminate()
        except OSError:
            pass

        if not connection_string:
            print(
                "the cluster did not start; check %s and %s" % (runner_output, log_dir),
                file=sys.stderr,
            )
            tail = read_text(runner_output).splitlines()[-50:]
            if tail:
                print("\n".join(tail), file=sys.stderr)
            sys.exit(1)
        started = True
    finally:
        if not started and teardown:
            teardown()

    return connection_string


def teardown_dsc_cluster():
    print("start failed after the cluster came up; tearing it down", file=sys.stderr)
    result = subprocess.run(["bash", str(SCRIPT_DIR / "stop-test-cluster.sh"), "--dsc"])
    if result.returncode != 0:
        print(
            "teardown also failed; run scripts/stop-test-cluster.sh --dsc by hand",
            file=sys.stderr,
        )


def finish_start(args, cluster_dir, connection_string):
    # The tools fall back to the operational database as the auth source when the URI does not name
    # one, which fails authentication for a user created in admin, so make the source explicit.
    if args.create_user and "authSource=" not in connection_string:
        separator = "&" if "?" in connection_string else "?"
        connection_string += separator + "authSource=admin"

    (cluster_dir / "connection-string").write_text(connection_string + "\n")

    # The CI "start test cluster" function runs expansions.update against this file so later commands
    # see TOOLS_TESTING_MONGOD.
    if args.evg_expansion_var:
        (cluster_dir / "expansions.yml").write_text(
            "%s: '%s'\n" % (args.evg_expansion_var, connection_string)
        )

    print()
    print("export TOOLS_TESTING_MONGOD='%s'" % connection_string)



def build_runner_config(args, ca_file, cert_file):
    # `internalClientOptions` and users are not CLI flags, so they have to go through `--config`. We
    # pass the repo's own test certificates so that the runner's URI has the same TLS options the
    # tests build from `TOOLS_TESTING_SSL`. Setting `tlsAddClientKey=false` stops the runner from
    # replacing them with a generated cert. The server certificate is valid for 127.0.0.1, so we do
    # not need tlsAllowInvalidCertificates; adding it would put an unsupported parameter in the URI,
    # which the tools log as a warning.
    config = {}

    if args.tls:
        config["tlsAddClientKey"] = False
        config["internalClientOptions"] = {
            "tls": True,
            "tlsCAFile": ca_file,
            "tlsCertificateKeyFile": cert_file,
        }

    if args.create_user:
        config["users"] = [
            {
                "username": args.auth_username,
                "password": args.auth_password,
                "roles": [{"role": "__system", "db": "admin"}],
            }
        ]

    # mongodb-runner gives every member but the first a priority of 0, which makes it passive: the
    # server reports passives separately from hosts in isMaster, and a passive can never be elected
    # primary. The tests want an ordinary replica set, in which any node can be primary, so the
    # membership is passed explicitly with the same priority on every member. This replaces the
    # runner's --secondaries flag, which cannot set a priority.
    if args.topology == "replset":
        config["rsMembers"] = [{"priority": 1} for _ in range(args.secondaries + 1)]

    return config


def build_mongod_args(args, ca_file, cert_file):
    mongod_args = list(args.extra_mongod_args)

    additional_args = env("ADDITIONAL_ARGS")
    if additional_args:
        mongod_args += additional_args.split()

    if args.tls:
        mongod_args += [
            "--tlsMode",
            "requireTLS",
            "--tlsCAFile",
            ca_file,
            "--tlsCertificateKeyFile",
            cert_file,
        ]

    if args.create_user:
        mongod_args.append("--auth")

    if args.aws_auth:
        mongod_args += ["--setParameter", "authenticationMechanisms=MONGODB-AWS,SCRAM-SHA-256"]

    return mongod_args


def wait_for_connection_string(runner_output, process):
    # The runner prints a single-line JSON object once the cluster is up, but on Windows it does not
    # exit: the detached mongod keeps its Node event loop alive. Capturing stdout with a pipe would
    # then wait forever for the pipe to close. So the runner writes to a file and we poll it for the
    # JSON line, stopping early if the process dies.
    for _ in range(900):
        if '"connectionString"' in read_text(runner_output):
            break
        if process.poll() is not None:
            break
        time.sleep(1)


def native_path(path):
    # mongodb-runner and mongod are native binaries, so under Cygwin they cannot open the POSIX
    # paths we use everywhere else. Convert just the paths they see; a path that is already native
    # (a Windows path starts with a drive letter) is returned unchanged.
    text = str(path)
    if re.match(r"[A-Za-z]:[\\/]", text):
        return text

    if shutil.which("cygpath"):
        # stdout=PIPE and universal_newlines rather than capture_output and text, which need Python
        # 3.7.
        result = subprocess.run(
            ["cygpath", "-w", text],
            stdout=subprocess.PIPE,
            universal_newlines=True,
            check=True,
        )
        return result.stdout.strip()

    return text


def read_text(path):
    try:
        return path.read_text(errors="replace")
    except FileNotFoundError:
        return ""


def parse_connection_string(text):
    # mongodb-runner prints a single-line JSON object carrying connectionString, but callers may
    # capture it alongside the runner's other output, so scan for the first such object.
    for line in text.splitlines():
        line = line.strip()
        if not line.startswith("{"):
            continue
        try:
            obj = json.loads(line)
        except ValueError:
            continue
        if isinstance(obj, dict) and obj.get("connectionString"):
            return obj["connectionString"]
    return ""


def command_succeeds(argv, env):
    try:
        result = subprocess.run(
            argv, env=env, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL
        )
    except FileNotFoundError:
        return False
    return result.returncode == 0


def run_checked(argv, env):
    result = subprocess.run(argv, env=env)
    if result.returncode != 0:
        fail("command failed with status %d: %s" % (result.returncode, " ".join(argv)))


def env(name, default=""):
    # Treat an empty variable the same as an unset one, as the shell's ${VAR:-default} does.
    # Evergreen expands a missing expansion to an empty string, so e.g. SHARDS is set but empty for
    # the topologies that do not pass it.
    return os.environ.get(name) or default


def env_true(name):
    return os.environ.get(name, "false") == "true"


def fail(message):
    print(message, file=sys.stderr)
    sys.exit(1)


if __name__ == "__main__":
    main()
