#!/bin/bash

# this file is used in case you want to do complex environment variable
# substitutions or configuration at startup

# generate a new self signed cert

# Use version 3 API
export ETCDCTL_API=3

set -ex

# Install a self-signed certificate
openssl genrsa -des3 -passout pass:x -out server.pass.key 2048
openssl rsa -passin pass:x -in server.pass.key -out server.key
rm server.pass.key
openssl req -new -key server.key -out server.csr \
  -subj "/C=US/ST=CA/L=Berkeley/O=UCBerkeley/OU=EECS/CN=default.autocert.smartgrid.store"
openssl x509 -req -days 365 -in server.csr -signkey server.key -out server.crt
hardcodecert server.crt server.key

if [[ $1 = "init" ]]
then
  head -c 16 /dev/urandom > encrypt_key
  head -c 16 /dev/urandom > mac_key
  setsessionkeys encrypt_key mac_key
  exit 0
fi

# Update the Javascript code so that the latest version is used
MR_PLOTTER_REPO=$GOPATH/src/github.com/SoftwareDefinedBuildings/mr-plotter
cd $MR_PLOTTER_REPO
git pull origin v4

# Run mr-plotter
cd $GOPATH/bin
mr-plotter $MR_PLOTTER_REPO/plotter.ini |& pp
