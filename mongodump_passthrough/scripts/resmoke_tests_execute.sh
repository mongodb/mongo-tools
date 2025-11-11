#!/bin/bash

SCRIPT_DIR=$(dirname "$0")

# shellcheck source=evergreen/scripts/resmoke_venv_activate.sh
. "$SCRIPT_DIR/resmoke_venv_activate.sh"

# shellcheck source=etc/functions.sh
. "$SCRIPT_DIR/../../etc/functions.sh"

# shellcheck source=etc/find-recent-python.sh
. "$SCRIPT_DIR/../../etc/find-recent-python.sh"

set -o errexit
set -o verbose

# If the test is running a coverage-instrumented binary, the coverage data
# files will be written to this directory. Otherwise this variable is ignored.
GOCOVERDIR="$(pwd)/src/mongosync/coverage"
export GOCOVERDIR
mkdir -p "$GOCOVERDIR"

resmoke_venv_activate

# Location of this script.
script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"

# Capture the location of the mongosync Python libraries, to be used in the PYTHONPATH
# variable. The PYTHONPATH variable tells the Python interpreter where to search for
# various libraries and modules. We must capture the location of the mongosync Python
# libraries so that if we try to import something from the mongosync modules, the Python
# interpreter will know where to search for those modules.
#
# shellcheck disable=SC2154
pushd "$resmoke_dir" || exit 1
full_resmoke_dir=$(pwd)
cd ..
cd mongosync
mongosync_libs_dir=$(pwd)
modified_python_path="$mongosync_libs_dir:$full_resmoke_dir"
echo "Modified path including mongosync Python libs: $modified_python_path"
popd

# Copy the expansions file to folder above the Resmoke invocation point since that is
# where Resmoke wants it to be.
cp expansions.yml "$resmoke_dir/.."

# Store the root directory.
pushd "$resmoke_dir" || exit 1

# Set the TMPDIR environment variable to be a directory in the task's working
# directory so that temporary files created by processes spawned by resmoke.py get
# cleaned up after the task completes. This also ensures the spawned processes
# aren't impacted by limited space in the mount point for the /tmp directory.
TMPDIR="$(pwd)/tmp"
export TMPDIR
mkdir -p "$TMPDIR"

if [ -f /proc/self/coredump_filter ]; then
    # Set the shell process (and its children processes) to dump ELF headers (bit 4),
    # anonymous shared mappings (bit 1), and anonymous private mappings (bit 0).
    echo 0x13 >/proc/self/coredump_filter

    if [ -f /sbin/sysctl ]; then
        # Check that the core pattern is set explicitly on our distro image instead
        # of being the OS's default value. This ensures that coredump names are consistent
        # across distros and can be picked up by Evergreen.
        core_pattern=$(/sbin/sysctl -n "kernel.core_pattern")
        if [ "$core_pattern" = "dump_%e.%p.core" ]; then
            echo "Enabling coredumps"
            ulimit -c unlimited
        fi
    fi
fi

# shellcheck disable=SC2154
extra_args="$extra_args --jobs=${resmoke_jobs} --storageEngineCacheSizeGB=4"

# shellcheck disable=SC2154
if [ "${should_shuffle}" = true ]; then
    extra_args="$extra_args --shuffle"
fi

# shellcheck disable=SC2154
echo "AWS Profile for archiving: ${aws_profile_remote}"

# Set the suite name to be the task name by default; unless overridden with the `suite` expansion.
#
# shellcheck disable=SC2154
suite_name=${task_name}
if [[ -n ${suite} ]]; then
    suite_name=${suite}
fi

resmoke_version=$(print_resmoke_version_for_source_server_version "${src_mongo_version:?}")

case "$resmoke_version" in
8.0 | 7.0)
    install_dir="$(pwd)/${src_mongo_version:?}/bin"
    ;;
6.0)
    install_dir="$(pwd)"
    ;;
*)
    echo "Unknown resmoke_version: [$resmoke_version]"
    exit 1
    ;;
esac

resmokeconfig_dir="../mongosync/resmoke/suite-config/$resmoke_version"
echo "resmokeconfig_dir=../mongosync/resmoke/suite-config/$resmoke_version"

mongo_executable="$(pwd)/mongo-for-shell/bin/mongo"

set +o errexit

# TODO (REP-5544): Remove once we no longer need this hack:
sed -i -E 's/self._log_local_resmoke_invocation()/### Commented out call to _log_local_resmoke_invocation/' buildscripts/resmokelib/run/__init__.py

# This seems to happen with PRs that are not against `main`.
if [ -z "${project}" ]; then
    echo "No 'project' found in the environment. Setting this to 'mongosync'."
    project=mongosync
fi

eval \
    PYTHONPATH="$modified_python_path" \
    AWS_PROFILE="${aws_profile_remote}" \
    python buildscripts/resmoke.py \
    --configDir="$resmokeconfig_dir" run \
    "${resmoke_args:?}" \
    "$extra_args" \
    --suites="${suite_name}" \
    --log=evg \
    --staggerJobs=on \
    --installDir="$install_dir" \
    --mongo="$mongo_executable" \
    --buildId="${build_id:?}" \
    --distroId="${distro_id:?}" \
    --executionNumber="${execution:?}" \
    --projectName="${project:?}" \
    --gitRevision="${revision:?}" \
    --revisionOrderId="${revision_order_id:?}" \
    --taskId="${task_id:?}" \
    --taskName="$task_name" \
    --taskWorkDir='${workdir}' \
    --variantName="${build_variant:?}" \
    --versionId="${version_id:?}" \
    --reportFile=report.json
resmoke_exit_code=$?
set -o errexit

# Group test results by job logs and jobs lobs to be displayed in Evergreen.
# Disable shellcheck as python is defined in the functions.sh script.
# shellcheck disable=SC2154
python3 "$script_dir/post_process_resmoke_tests_to_link_to_job_logs.py"

# 74 is exit code for IOError on POSIX systems, which is raised when the machine is
# shutting down.
#
# 75 is exit code resmoke.py uses when the log output would be incomplete due to failing
# to communicate with logkeeper.
if [[ $resmoke_exit_code == 74 || $resmoke_exit_code == 75 ]]; then
    echo $resmoke_exit_code >infrastructure_failure
    exit 0
# TODO (REP-235): Hide failing tests when a hook hasn't failed.
elif [ $resmoke_exit_code != 0 ]; then
    # Exit with the resmoke error code.
    exit $resmoke_exit_code

# elif [ $resmoke_exit_code != 0 ]; then
#   # 3 is the exit code returned by hooks. Ignore non-hook exit codes.
#   echo "Ignoring non-hook failure with error code: $resmoke_exit_code"

#   # Head back to the root directory.
#   popd
#   # Remove the failing tests from the test report.
#   eval \
#   ${gcov_environment} \
#   ${lang_environment} \
#   ${san_options} \
#   ${snmp_config_path} \
#   python3 "$script_dir/remove_failing_tests_from_resmoke_test_report.py" \
#   --sourceReportFile="$resmoke_dir/report.json" \
#   --destinationReportFile="$resmoke_dir/report.json"
elif [ $resmoke_exit_code = 0 ]; then
    # On success delete core files.
    find -H .. \( -name "*.core" -o -name "*.mdmp" \) -exec rm -rf {} \;
fi

exit $resmoke_exit_code
