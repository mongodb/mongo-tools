#!/bin/bash

set -o errexit
set -o pipefail
set -o verbose

: "${RUN_KINIT?}" "${KERBEROS_KEYTAB?}"

# export sensitive info before `set -x`
if [ "$RUN_KINIT" = 'true' ]; then
    # BUILD-3830
    mkdir -p "$(pwd)/.evergreen"
    touch "$(pwd)/.evergreen/krb5.conf.empty"
    KRB5_CONFIG="$(pwd)/.evergreen/krb5.conf.empty"
    export KRB5_CONFIG

    echo "Writing keytab"
    echo "$KERBEROS_KEYTAB" | base64 -d >"$(pwd)/.evergreen/drivers.keytab"

    # The KDC is external, so a transient DNS or network blip makes kinit fail outright. Retry a few
    # times rather than failing the whole task before any tests run.
    echo "Running kinit"
    for attempt in 1 2 3; do
        if kinit -k -t "$(pwd)/.evergreen/drivers.keytab" -p drivers@LDAPTEST.10GEN.CC; then
            break
        fi
        if [ "$attempt" -eq 3 ]; then
            echo "kinit failed after ${attempt} attempts" >&2
            exit 1
        fi
        echo "kinit attempt ${attempt} failed; retrying in 5s" >&2
        sleep 5
    done
fi
set -x
set -v
if [ "Windows_NT" = "$OS" ]; then
    cmd /c "REG ADD HKLM\SYSTEM\ControlSet001\Control\Lsa\Kerberos\Domains\LDAPTEST.10GEN.CC /v KdcNames /d ldaptest.10gen.cc /t REG_MULTI_SZ /f"
fi
