#!/bin/sh

df -P | awk '/^\// {print $1"\t"$2"\t"$4}' | python -c 'import json, fileinput; print json.dumps({"diskarray":[dict(zip(("mount", "spacetotal", "spaceavail"), l.split())) for l in fileinput.input()]}, indent=2)' > /home/manager/go/src/github.com/SoftwareDefinedBuildings/mr-plotter/assets/disk_usage.json