#!/bin/bash
set -ex

pushd ..
go build -v
ver=$(./mr-plotter -version)
popd
cp ../mr-plotter .
pushd ../tools/hardcodecert
go build -v
popd
pushd ../tools/setsessionkeys
go build -v
popd
cp ../tools/hardcodecert/hardcodecert .
cp ../tools/setsessionkeys/setsessionkeys .
rsync -PHav ../assets .
cp ../plotter.ini .

docker build -t btrdb/dev-mrplotter:${ver} .
#docker push btrdb/mrplotter:${ver}
docker tag btrdb/dev-mrplotter:${ver} btrdb/dev-mrplotter:latest
docker push btrdb/dev-mrplotter:latest
docker push btrdb/dev-mrplotter:${ver}
