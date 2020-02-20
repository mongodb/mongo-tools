#!/bin/sh
set -e

if [ "$1" = "configure" ]; then
  if [ -d /usr/doc -a ! -e /usr/doc/mongodb-database-tools -a -d /usr/share/doc/mongodb-database-tools ]; then
    ln -sf ../share/doc/mongodb-database-tools /usr/doc/mongodb-database-tools
  fi
fi
