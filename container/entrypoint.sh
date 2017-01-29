#!/bin/bash

# this file is used in case you want to do complex environment variable
# substitutions or configuration at startup

# generate a new self signed cert

openssl genrsa -des3 -passout pass:x -out server.pass.key 2048
openssl rsa -passin pass:x -in server.pass.key -out server.key
rm server.pass.key
openssl req -new -key server.key -out server.csr \
  -subj "/C=US/ST=CA/L=Berkeley/O=UCBerkeley/OU=EECS/CN=default.autocert.smartgrid.store"
openssl x509 -req -days 365 -in server.csr -signkey server.key -out server.crt
mv server.key $GOPATH/src/github.com/SoftwareDefinedBuildings/mr-plotter/defaultcert/key.pem
mv server.crt $GOPATH/src/github.com/SoftwareDefinedBuildings/mr-plotter/defaultcert/cert.pem

set -e

# run mr-plotter
mr-plotter |& pp
