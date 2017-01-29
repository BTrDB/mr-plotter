#!/bin/bash
set -ex

docker build  -t btrdb/mrplotter:latest .
docker push btrdb/mrplotter:latest
