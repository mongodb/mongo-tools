#!/bin/bash

set -x
set -v
set -e

mongotarget=$(if [ "${mongo_target}" ]; then echo "${mongo_target}"; else echo "${mongo_os}"; fi)
mongoversion=$(if [ "${mongo_version_always_use_latest}" ]; then echo "latest"; else echo "${mongo_version}"; fi)
PATH=/opt/mongodbtoolchain/v3/bin/:$PATH

python="python3"
if [ "Windows_NT" = "$OS" ]; then
  python="py.exe -3"
fi

dlurl=$($python binaryurl.py --edition=${mongo_edition} --target=$mongotarget --version=$mongoversion --arch=${mongo_arch|x86_64})
filename=$(echo $dlurl | sed -e "s_.*/__")
mkdir -p bin
curl -s $dlurl --output $filename
${decompress} $filename
rm $filename

if [ "${only_shell}" ]; then
  mv -f ./mongodb-*/bin/mongo${extension} ./bin/
else
  mv -f ./mongodb-*/bin/mongo${extension} ./bin/
  mv -f ./mongodb-*/bin/mongos${extension} ./bin/
  mv -f ./mongodb-*/bin/mongod${extension} ./bin/
fi
if [ "Windows_NT" = "$OS" ]; then
  mv -f ./mongodb-*/bin/netsnmp.dll ./bin/
fi

chmod +x ./bin/*
rm -rf ./mongodb-*
