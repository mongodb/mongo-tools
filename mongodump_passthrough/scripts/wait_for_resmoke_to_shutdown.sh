#!/bin/bash

proc="resmoke.py"
get_pids() { proc_pids=$(pgrep -f "$1"); }
while true; do
    get_pids $proc
    if [ -z "$proc_pids" ]; then
        break
    fi
    sleep 5
done
