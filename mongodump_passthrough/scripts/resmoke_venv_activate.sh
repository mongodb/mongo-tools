#!/bin/bash
# this file is copied from mongosync/evergreen/scripts/resmoke_venv_activate.sh

function resmoke_venv_activate {
    SCRIPT_DIR=$(dirname "$0")
    # shellcheck source=mongodump_passthrough/scripts/find-recent-python.sh
    . "$SCRIPT_DIR/find-recent-python.sh"

    python3 -m venv "${resmoke_venv_dir:?}"

    # shellcheck disable=SC1091
    source "$resmoke_venv_dir/bin/activate"

    poetry_reqs_path="poetry-requirements.txt"

    legacy_reqs_file="${resmoke_dir:?}/etc/pip/dev-requirements.txt"

    if [ -e "$poetry_reqs_path" ]; then
        echo "Using Poetry-generated requirements file ..."
        reqs_file_path="$poetry_reqs_path"
    elif [ -e "$legacy_reqs_file" ]; then
        echo "Using mongo repo’s requirements file ..."
        reqs_file_path="$legacy_reqs_file"
    else
        echo "Found neither $poetry_reqs_path nor $legacy_reqs_file .. what gives??"
    fi

    echo Installing pip ...
    python -m pip --disable-pip-version-check install "pip==21.0.1" "wheel==0.37.0" || exit 1

    echo Installing pip requirements from "$reqs_file_path" ...

    if ! python -m pip \
        --disable-pip-version-check \
        install \
        --no-index \
        --find-links wheelhouse \
        --log install.log \
        --requirement "$reqs_file_path"; then
        echo "Pip install error"
        cat install.log
        exit 1
    fi

    # The sitecustomize.py file, when it exists, is run when the Python interpreter is started and before
    # any code is run.
    # In this sitecustomize.py file, we attempt to import 'mongosync' before Resmoke is run.
    # The reason we have to import all the modules in mongosync beforehand is Resmoke's fixture and hook
    # builder relies on all fixtures and hooks being registered by name in a registry, so that during
    # Resmoke's execution, after suite YAML files are parsed, Resmoke's builder can look up fixtures
    # and hooks by name and know what / how to construct them.
    # Therefore we need to register the modules before Resmoke starts running, which is what will happen
    # if we put it in a sitecustomize.py file.
    echo "Trying to create a sitecustomize.py file..."
    cat >"$resmoke_venv_dir/lib/python3.10/site-packages/sitecustomize.py" <<END_OF_PRELOADER
import traceback
try:
    import resmoke.mongosync
except ModuleNotFoundError as e:
    print("Error: missing mongosync Python fixtures and hooks.")
    traceback.print_exception(e)
except Exception as e:
    print("Error: mongosync module may have problems.")
    traceback.print_exception(e)
    raise
END_OF_PRELOADER
    echo "Done creating sitecustomize.py."
}
