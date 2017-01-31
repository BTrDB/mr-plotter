#!/bin/bash
set -ex

pushd ..
go build -v
ver=$(./mr-plotter -version)
popd
cp ../mr-plotter .
docker build -t btrdb/mrplotter:${ver} .
docker push btrdb/mrplotter:${ver}
docker tag btrdb/mrplotter:${ver} btrdb/mrplotter:latest
docker push btrdb/mrplotter:latest
