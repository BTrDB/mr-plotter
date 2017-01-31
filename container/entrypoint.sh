#!/bin/bash

# this file is used in case you want to do complex environment variable
# substitutions or configuration at startup

# generate a new self signed cert

# Use version 3 API
export ETCDCTL_API=3

set -ex

# run mr-plotter
cd $GOPATH/bin
mr-plotter $GOPATH/src/github.com/SoftwareDefinedBuildings/mr-plotter/plotter.ini |& pp
