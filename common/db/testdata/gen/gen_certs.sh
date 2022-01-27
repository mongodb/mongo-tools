#!/bin/sh
set -x

# CA
openssl genrsa -out ../mongodb-test-ca.key 4096
openssl req -batch -new -x509 -days 3650 -key ../mongodb-test-ca.key -out ../mongodb-test-ca.crt -config openssl-test-ca.cnf

# IA
openssl genrsa -out ../mongodb-test-ia.key 4096
openssl req -batch -new -key ../mongodb-test-ia.key -out ../mongodb-test-ia.csr -config openssl-test-ia.cnf

openssl x509 -sha256 -req -days 3650 -in ../mongodb-test-ia.csr \
    -CA ../mongodb-test-ca.crt \
    -CAkey ../mongodb-test-ca.key \
    -set_serial 01 \
    -out ../mongodb-test-ia.crt \
    -extfile openssl-test-ca.cnf \
    -extensions v3_ca

cat ../mongodb-test-ca.crt  > ../ca.pem
cat ../mongodb-test-ia.crt  > ../ia.pem
cat ../mongodb-test-ia.crt ../mongodb-test-ca.crt > ../ca-ia.pem

# Server
openssl genrsa -out ../mongodb-test-server1.key 4096
openssl req -batch -new -key ../mongodb-test-server1.key -out ../mongodb-test-server1.csr -config openssl-test-server.cnf

openssl x509 -sha256 -req -days 3650 -in ../mongodb-test-server1.csr \
    -CA ../mongodb-test-ia.crt \
    -CAkey ../mongodb-test-ia.key \
    -CAcreateserial \
    -out ../mongodb-test-server1.crt \
    -extfile openssl-test-server.cnf \
    -extensions v3_req

cat ../mongodb-test-server1.crt ../mongodb-test-server1.key > ../test-server.pem

# Client
openssl genrsa -out ../mongodb-test-client.key 4096
openssl req -batch -new -key ../mongodb-test-client.key -out ../mongodb-test-client.csr -config openssl-test-client.cnf

openssl x509 -sha256 -req -days 3650 -in ../mongodb-test-client.csr \
    -CA ../mongodb-test-ia.crt \
    -CAkey ../mongodb-test-ia.key \
    -CAcreateserial \
    -out ../mongodb-test-client.crt \
    -extfile openssl-test-client.cnf \
    -extensions v3_req

cat ../mongodb-test-client.crt ../mongodb-test-client.key > ../test-client.pem

# PKCS8
openssl pkcs8 -v2 des3 -topk8 -passout pass:passwordIsTacoCat -inform PEM -outform PEM -in ../test-client.pem -out ../test-client-pkcs8-encrypted.key
cat ../mongodb-test-client.crt ../test-client-pkcs8-encrypted.key > ../test-client-pkcs8-encrypted.pem

openssl pkcs8 -topk8 -nocrypt -inform PEM -outform PEM -in ../test-client.pem -out ../test-client-pkcs8-unencrypted.key
cat ../mongodb-test-client.crt ../test-client-pkcs8-unencrypted.key > ../test-client-pkcs8-unencrypted.pem