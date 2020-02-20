#!/bin/sh

set -e
if [ \( "$1" = "upgrade" -o "$1" = "remove" \) -a -L /usr/doc/mongodb-database-tools ]; then
  rm -f /usr/doc/mongodb-database-tools
fi
