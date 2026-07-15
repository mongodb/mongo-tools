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
    echo "Running kinit"
    kinit -k -t "$(pwd)/.evergreen/drivers.keytab" -p drivers@LDAPTEST.10GEN.CC
fi
set -x
set -v
if [ "Windows_NT" = "$OS" ]; then
    cmd /c "REG ADD HKLM\SYSTEM\ControlSet001\Control\Lsa\Kerberos\Domains\LDAPTEST.10GEN.CC /v KdcNames /d ldaptest.10gen.cc /t REG_MULTI_SZ /f"
fi
